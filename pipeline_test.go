package gocloc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
	for _, workers := range []int{1, 8} {
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
				if strings.Contains(diagnostics.String(), "same md5") {
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

func TestGetAllFilesParallelMD5PreservesFirstFile(t *testing.T) {
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

func TestGetAllFilesParallelMD5ContinuesAfterWalkError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "valid.go")
	if err := os.WriteFile(file, []byte("package valid\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := NewClocOptions()
	opts.Workers = 4
	opts.SkipDuplicated = false
	files, err := getAllFiles([]string{filepath.Join(dir, "missing"), dir}, NewDefinedLanguages(), opts)
	if err != nil {
		t.Fatalf("getAllFiles() error = %v, want nil", err)
	}
	if got, want := files["Go"].Files, []string{file}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained files = %q, want %q", got, want)
	}
}

func TestGetAllFilesSkipDuplicatedKeepsAllFiles(t *testing.T) {
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
	files, err := getAllFiles([]string{dir}, NewDefinedLanguages(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := files["Go"].Files, []string{first, duplicate}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained files = %q, want %q", got, want)
	}
}
