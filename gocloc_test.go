package gocloc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveWorkerCount(t *testing.T) {
	t.Run("zero-value options remain serial", func(t *testing.T) {
		if got := resolveWorkerCount(&ClocOptions{}); got != 1 {
			t.Fatalf("resolveWorkerCount() = %d, want 1", got)
		}
	})

	t.Run("configured worker count is used", func(t *testing.T) {
		if got := resolveWorkerCount(&ClocOptions{Workers: 3}); got != 3 {
			t.Fatalf("resolveWorkerCount() = %d, want 3", got)
		}
	})

	t.Run("configured worker count is capped", func(t *testing.T) {
		if got := resolveWorkerCount(&ClocOptions{Workers: MaxWorkers + 1}); got != MaxWorkers {
			t.Fatalf("resolveWorkerCount() = %d, want %d", got, MaxWorkers)
		}
	})

	t.Run("callbacks require serial analysis", func(t *testing.T) {
		if got := resolveWorkerCount(&ClocOptions{Workers: 2, OnCode: func(string) {}}); got != 1 {
			t.Fatalf("resolveWorkerCount() = %d, want 1", got)
		}
	})

	t.Run("debug output requires serial analysis", func(t *testing.T) {
		if got := resolveWorkerCount(&ClocOptions{Workers: 2, Debug: true}); got != 1 {
			t.Fatalf("resolveWorkerCount() = %d, want 1", got)
		}
	})
}

func TestProcessorAnalyzeWorkersProducesEquivalentResults(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	pythonFile := filepath.Join(dir, "script.py")
	if err := os.WriteFile(goFile, []byte("package sample\n// comment\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pythonFile, []byte("# comment\nprint(\"x\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	serial := analyzeDirectory(t, dir, &ClocOptions{Workers: 1})
	parallel := analyzeDirectory(t, dir, &ClocOptions{Workers: 2})
	serialLanguages, serialFiles := serial.Languages, serial.Files
	parallelLanguages, parallelFiles := parallel.Languages, parallel.Files

	assertAnalysisCounts(t, serialLanguages, serialFiles)
	assertAnalysisCounts(t, parallelLanguages, parallelFiles)
	if serialFiles[goFile].Code != parallelFiles[goFile].Code || serialFiles[pythonFile].Comments != parallelFiles[pythonFile].Comments {
		t.Fatal("serial and parallel analysis produced different file counts")
	}
}

func TestProcessorAnalyzeWorkersPreservesDuplicateBehavior(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, skipDuplicated := range []bool{false, true} {
		t.Run("skip duplicated="+map[bool]string{false: "false", true: "true"}[skipDuplicated], func(t *testing.T) {
			serial := analyzeDirectory(t, dir, &ClocOptions{Workers: 1, SkipDuplicated: skipDuplicated})
			parallel := analyzeDirectory(t, dir, &ClocOptions{Workers: 2, SkipDuplicated: skipDuplicated})
			if serial.Total.Total != parallel.Total.Total || serial.Total.Code != parallel.Total.Code {
				t.Fatalf("serial totals = (%d, %d), parallel totals = (%d, %d)", serial.Total.Total, serial.Total.Code, parallel.Total.Total, parallel.Total.Code)
			}
			if !reflect.DeepEqual(serial.Files, parallel.Files) {
				t.Fatalf("serial files = %v, parallel files = %v", serial.Files, parallel.Files)
			}
			wantFiles := 1
			if skipDuplicated {
				wantFiles = 2
			}
			if len(parallel.Files) != wantFiles {
				t.Fatalf("file count = %d, want %d", len(parallel.Files), wantFiles)
			}
		})
	}
}

func TestProcessorCallbacksPreserveOrderAndDeduplication(t *testing.T) {
	dir := t.TempDir()
	first := "package first\n// first\n\n"
	second := "package second\n// second\n\n"
	for name, content := range map[string]string{"a.go": first, "b.go": first, "c.go": second} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, skip := range []bool{false, true} {
		for _, debug := range []bool{false, true} {
			t.Run(fmt.Sprintf("skip=%t/debug=%t", skip, debug), func(t *testing.T) {
				lines := []string{}
				opts := &ClocOptions{
					Workers: 8, SkipDuplicated: skip, Debug: debug, Diagnostics: io.Discard,
					OnCode:    func(line string) { lines = append(lines, "code:"+line) },
					OnComment: func(line string) { lines = append(lines, "comment:"+line) },
					OnBlank:   func(line string) { lines = append(lines, "blank:"+line) },
				}
				result := analyzeDirectory(t, dir, opts)
				want := []string{"code:package first", "comment:// first", "blank:"}
				files := int32(2)
				if skip {
					want = append(want, want...)
					files = 3
				}
				want = append(want, "code:package second", "comment:// second", "blank:")
				if !reflect.DeepEqual(lines, want) || result.Total.Total != files {
					t.Fatalf("callbacks=%q total=%+v, want %q (%d files)", lines, result.Total, want, files)
				}
			})
		}
	}
}

func assertAnalysisCounts(t *testing.T, languages map[string]*Language, files map[string]*ClocFile) {
	t.Helper()
	if len(files) != 2 {
		t.Fatalf("analyzed file count = %d, want 2", len(files))
	}
	if got := languages["Go"]; got.Code != 2 || got.Comments != 1 || got.Blanks != 1 {
		t.Fatalf("Go counts = (%d, %d, %d), want (2, 1, 1)", got.Code, got.Comments, got.Blanks)
	}
	if got := languages["Python"]; got.Code != 1 || got.Comments != 1 || got.Blanks != 0 {
		t.Fatalf("Python counts = (%d, %d, %d), want (1, 1, 0)", got.Code, got.Comments, got.Blanks)
	}
}

func analyzeDirectory(t *testing.T, dir string, opts *ClocOptions) *Result {
	t.Helper()
	result, err := NewProcessor(NewDefinedLanguages(), opts).Analyze([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
