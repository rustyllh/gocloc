package gocloc

import (
	"os"
	"path/filepath"
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

func TestAnalyzeFilesWorkersProducesEquivalentResults(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	pythonFile := filepath.Join(dir, "script.py")
	if err := os.WriteFile(goFile, []byte("package sample\n// comment\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pythonFile, []byte("# comment\nprint(\"x\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	serialLanguages := testLanguages(goFile, pythonFile)
	serialFiles := analyzeFiles(serialLanguages, &ClocOptions{Workers: 1})
	parallelLanguages := testLanguages(goFile, pythonFile)
	parallelFiles := analyzeFiles(parallelLanguages, &ClocOptions{Workers: 2})

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
		})
	}
}

func TestAnalyzeFilesCallbacksUseSerialOrder(t *testing.T) {
	dir := t.TempDir()
	firstFile := filepath.Join(dir, "first.go")
	secondFile := filepath.Join(dir, "second.go")
	for file, content := range map[string]string{firstFile: "package first\n", secondFile: "package second\n"} {
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	language := NewLanguage("Go", []string{"//"}, [][]string{{"/*", "*/"}})
	language.Files = []string{firstFile, secondFile}
	var callbackLines []string
	analyzeFiles(map[string]*Language{"Go": language}, &ClocOptions{
		Workers: 2,
		OnCode: func(line string) {
			callbackLines = append(callbackLines, line)
		},
	})
	if len(callbackLines) != 2 || callbackLines[0] != "package first" || callbackLines[1] != "package second" {
		t.Fatalf("callback lines = %q, want [package first package second]", callbackLines)
	}
}

func testLanguages(goFile, pythonFile string) map[string]*Language {
	goLanguage := NewLanguage("Go", []string{"//"}, [][]string{{"/*", "*/"}})
	goLanguage.Files = []string{goFile}
	pythonLanguage := NewLanguage("Python", []string{"#"}, [][]string{{"\"\"\"", "\"\"\""}})
	pythonLanguage.Files = []string{pythonFile}
	return map[string]*Language{"Go": goLanguage, "Python": pythonLanguage}
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
