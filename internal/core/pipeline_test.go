package gocloc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestProcessorDebugLogsAnalysisBeforeDuplicateExclusion(t *testing.T) {
	dir := t.TempDir()
	paths := []string{}
	for i := range 24 {
		path := filepath.Join(dir, fmt.Sprintf("file %02d.go", i))
		if err := os.WriteFile(path, []byte("package sample\n// comment\n\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	for _, workers := range []int{1, 2, 8} {
		for _, skip := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%d/skip=%t", workers, skip), func(t *testing.T) {
				t.Parallel()
				var diagnostics bytes.Buffer
				opts := &ClocOptions{
					Workers: workers, Debug: true, SkipDuplicated: skip, Diagnostics: &diagnostics,
				}
				result := analyzeDirectory(t, dir, opts)
				wantFiles := paths[:1]
				if skip {
					wantFiles = paths
				}
				if !reflect.DeepEqual(result.Languages["Go"].Files, wantFiles) {
					t.Fatalf("retained files=%q, want %q", result.Languages["Go"].Files, wantFiles)
				}
				if result.Total.Total != int32(len(wantFiles)) || result.Total.Code != int32(len(wantFiles)) {
					t.Fatalf("debug changed counts: %+v", result.Total)
				}
				type position struct {
					path  string
					index int
				}
				expected := make(map[string]position)
				for i, path := range paths {
					file := "file=" + strconv.Quote(path)
					records := []string{
						"[FILE] " + file,
						"[CODE] " + file + ` line=1 code=1 comment=0 blank=0 in_comment=false text="package sample\n"`,
						"[COMM] " + file + ` line=2 code=1 comment=1 blank=0 in_comment=false text="// comment\n"`,
						"[BLNK] " + file + ` line=3 code=1 comment=1 blank=1 in_comment=false text="\n"`,
					}
					if !skip && i > 0 {
						records = append(records, "[SKIP] "+file+` reason="duplicate content"`)
					}
					for index, record := range records {
						expected[record] = position{path: path, index: index}
					}
				}
				// Require complete records and per-file order, not any cross-file order.
				next := make(map[string]int)
				for _, record := range strings.Split(strings.TrimSuffix(diagnostics.String(), "\n"), "\n") {
					pos, ok := expected[record]
					if !ok || next[pos.path] != pos.index {
						t.Fatalf("unexpected, interleaved or out-of-order record: %q", record)
					}
					next[pos.path]++
					delete(expected, record)
				}
				if len(expected) != 0 {
					t.Fatalf("missing debug records: %v", expected)
				}
			})
		}
	}
}

func TestProcessorReportsFileFailuresAndContinues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "good.go"), []byte("package good\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// More failures than the result window must still return tokens and finish.
	for i := range parallelResultWindowSize(8) + 1 {
		name := filepath.Join(dir, fmt.Sprintf("broken%03d.go", i))
		if err := os.Symlink(filepath.Join(dir, "missing"), name); err != nil {
			t.Fatal(err)
		}
	}
	for _, workers := range []int{1, 2, 8} {
		for _, skip := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%d/skip=%t", workers, skip), func(t *testing.T) {
				var diagnostics bytes.Buffer
				opts := &ClocOptions{Workers: workers, SkipDuplicated: skip, Diagnostics: &diagnostics}
				result := analyzeDirectory(t, dir, opts)
				if result.Total.Total != 1 || result.Total.Code != 1 || len(result.Files) != 1 {
					t.Fatalf("failed files were counted: %+v", result.Total)
				}
				if got, want := strings.Count(diagnostics.String(), "warning:"), parallelResultWindowSize(8)+1; got != want {
					t.Fatalf("warnings=%d, want %d", got, want)
				}
				if strings.Contains(diagnostics.String(), "duplicate content") {
					t.Fatal("read failures were reported as duplicates")
				}
			})
		}
	}
}

func TestParallelResultWindowSize(t *testing.T) {
	for workers, want := range map[int]int{
		1:  256,
		8:  256,
		16: 512,
	} {
		if got := parallelResultWindowSize(workers); got != want {
			t.Fatalf("parallelResultWindowSize(%d) = %d, want %d", workers, got, want)
		}
	}
}

func TestPipelineDeduplicationPreservesFirstFile(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.go")
	duplicate := filepath.Join(dir, "b.go")
	different := filepath.Join(dir, "c.go")
	for path, content := range map[string]string{
		first:     "package first\n",
		duplicate: "package first\n",
		different: "package different\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	opts := NewClocOptions()
	opts.Workers = 4
	opts.SkipDuplicated = false
	files, clocFiles, err := scanAndAnalyzeFiles([]string{dir}, NewDefinedLanguages(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := files["Go"].Files, []string{first, different}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained files = %q, want %q", got, want)
	}
	if len(clocFiles) != 2 || clocFiles[duplicate] != nil {
		t.Fatalf("analyzed files = %v, want only retained files", clocFiles)
	}
}

func TestParallelDetectionPreservesOrderAcrossIgnoredFiles(t *testing.T) {
	dir := t.TempDir()
	// More ignored candidates than the window must still release all tokens.
	for i := range parallelResultWindowSize(4) + 1 {
		path := filepath.Join(dir, fmt.Sprintf("a%04d.unknown", i))
		if err := os.WriteFile(path, []byte("unknown\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	content := []byte("#!/usr/bin/env python\n# comment\nprint(1)\n")
	for _, name := range []string{"b.go", "c.py"} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	serial := analyzeDirectory(t, dir, &ClocOptions{Workers: 1})
	parallel := analyzeDirectory(t, dir, &ClocOptions{Workers: 4})
	if !reflect.DeepEqual(serial.Files, parallel.Files) || !reflect.DeepEqual(serial.Total, parallel.Total) {
		t.Fatalf("serial and parallel results differ: %v / %v", serial.Files, parallel.Files)
	}
	first := parallel.Files[filepath.Join(dir, "b.go")]
	if len(parallel.Files) != 1 || first == nil || first.Lang != "Python" {
		t.Fatalf("expected first copy with shebang language, got %v", parallel.Files)
	}
}

func TestPipelineContinuesAfterWalkError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "valid.go")
	if err := os.WriteFile(file, []byte("package valid\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := NewClocOptions()
	opts.Workers = 4
	opts.SkipDuplicated = false
	files, _, err := scanAndAnalyzeFiles([]string{filepath.Join(dir, "missing"), dir}, NewDefinedLanguages(), opts)
	if err != nil {
		t.Fatalf("scanAndAnalyzeFiles() error = %v, want nil", err)
	}
	if got, want := files["Go"].Files, []string{file}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained files = %q, want %q", got, want)
	}
}

func TestPipelineSkipDuplicatedKeepsAllFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.go")
	duplicate := filepath.Join(dir, "b.go")
	for _, path := range []string{first, duplicate} {
		if err := os.WriteFile(path, []byte("package same\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	opts := NewClocOptions()
	opts.Workers = 4
	opts.SkipDuplicated = true
	files, _, err := scanAndAnalyzeFiles([]string{dir}, NewDefinedLanguages(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := files["Go"].Files, []string{first, duplicate}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained files = %q, want %q", got, want)
	}
}
