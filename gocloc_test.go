package gocloc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveWorkerCount(t *testing.T) {
	tests := []struct {
		name string
		opts *ClocOptions
		want int
	}{
		{name: "nil options", want: 1},
		{name: "zero value", opts: &ClocOptions{}, want: 1},
		{name: "negative count", opts: &ClocOptions{Workers: -1}, want: 1},
		{name: "one worker", opts: &ClocOptions{Workers: 1}, want: 1},
		{name: "configured count", opts: &ClocOptions{Workers: 3}, want: 3},
		{name: "maximum count", opts: &ClocOptions{Workers: MaxWorkers}, want: MaxWorkers},
		{name: "capped count", opts: &ClocOptions{Workers: MaxWorkers + 1}, want: MaxWorkers},
		{name: "debug does not change count", opts: &ClocOptions{Workers: 2, Debug: true}, want: 2},
		{
			name: "code callback does not change count",
			opts: &ClocOptions{Workers: 2, OnCode: func(string) {}}, want: 2,
		},
		{
			name: "blank callback does not change count",
			opts: &ClocOptions{Workers: 2, OnBlank: func(string) {}}, want: 2,
		},
		{
			name: "comment callback does not change count",
			opts: &ClocOptions{Workers: 2, OnComment: func(string) {}}, want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveWorkerCount(tt.opts); got != tt.want {
				t.Fatalf("resolveWorkerCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRequiresSynchronousCallbacks(t *testing.T) {
	tests := []struct {
		name string
		opts *ClocOptions
		want bool
	}{
		{name: "nil options"},
		{name: "zero value", opts: &ClocOptions{}},
		{name: "one worker", opts: &ClocOptions{Workers: 1}},
		{name: "multiple workers", opts: &ClocOptions{Workers: 8}},
		{name: "warnings only", opts: &ClocOptions{Diagnostics: &bytes.Buffer{}}},
		{name: "debug", opts: &ClocOptions{Debug: true}},
		{
			name: "debug with callback",
			opts: &ClocOptions{Debug: true, OnCode: func(string) {}}, want: true,
		},
		{name: "code callback", opts: &ClocOptions{OnCode: func(string) {}}, want: true},
		{name: "blank callback", opts: &ClocOptions{OnBlank: func(string) {}}, want: true},
		{name: "comment callback", opts: &ClocOptions{OnComment: func(string) {}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := requiresSynchronousCallbacks(tt.opts); got != tt.want {
				t.Fatalf("requiresSynchronousCallbacks() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestProcessorAnalyzeEmptyInput(t *testing.T) {
	for _, workers := range []int{0, 1, 2, 8} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			t.Parallel()
			opts := &ClocOptions{Workers: workers}
			result, err := NewProcessor(NewDefinedLanguages(), opts).Analyze(nil)
			if err != nil {
				t.Fatal(err)
			}
			hasFiles := len(result.Files) != 0 || len(result.Languages) != 0
			if result.Total.Total != 0 || hasFiles {
				t.Fatalf("expected empty result: %+v", result)
			}
		})
	}
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

	for _, workers := range []int{0, 1, 2, 8} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			result := analyzeDirectory(t, dir, &ClocOptions{Workers: workers})
			assertAnalysisCounts(t, result.Languages, result.Files)
		})
	}
}

func TestProcessorAnalyzeWorkersPreservesDuplicateBehavior(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("nil options count both files", func(t *testing.T) {
		result := analyzeDirectory(t, dir, nil)
		validTotal := result.Total.Total == 2 && result.Total.Code == 2
		if !validTotal || len(result.Files) != 2 {
			t.Fatalf("nil options should count both files: %+v", result.Total)
		}
	})

	for _, skip := range []bool{false, true} {
		wantPaths := []string{filepath.Join(dir, "a.go")}
		if skip {
			wantPaths = append(wantPaths, filepath.Join(dir, "b.go"))
		}
		for _, workers := range []int{0, 1, 2, 8} {
			t.Run(fmt.Sprintf("workers=%d/skip=%t", workers, skip), func(t *testing.T) {
				result := analyzeDirectory(t, dir, &ClocOptions{Workers: workers, SkipDuplicated: skip})
				wantCount := int32(len(wantPaths))
				if result.Total.Total != wantCount || result.Total.Code != wantCount {
					t.Fatalf("total=%+v, want %d files and code lines", result.Total, wantCount)
				}
				if got := result.Languages["Go"].Files; !reflect.DeepEqual(got, wantPaths) {
					t.Fatalf("retained files=%q, want %q", got, wantPaths)
				}
				if len(result.Files) != len(wantPaths) {
					t.Fatalf("analyzed file count=%d, want %d", len(result.Files), len(wantPaths))
				}
				for _, path := range wantPaths {
					if result.Files[path] == nil || result.Files[path].Code != 1 {
						t.Fatalf("file %q was not counted correctly: %+v", path, result.Files[path])
					}
				}
			})
		}
	}
}

func TestProcessorIndividualCallbacksPreserveDeduplication(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.go")
	duplicate := filepath.Join(dir, "b.go")
	for _, path := range []string{first, duplicate} {
		if err := os.WriteFile(path, []byte("package sample\n// comment\n\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		want string
		set  func(*ClocOptions, func(string))
	}{
		{
			name: "code", want: "package sample",
			set: func(opts *ClocOptions, callback func(string)) { opts.OnCode = callback },
		},
		{
			name: "comment", want: "// comment",
			set: func(opts *ClocOptions, callback func(string)) { opts.OnComment = callback },
		},
		{
			name: "blank", want: "",
			set: func(opts *ClocOptions, callback func(string)) { opts.OnBlank = callback },
		},
		{
			name: "debug with callback", want: "package sample",
			set: func(opts *ClocOptions, callback func(string)) {
				opts.Debug = true
				opts.OnCode = callback
			},
		},
	}
	for _, tt := range tests {
		for _, workers := range []int{1, 8} {
			t.Run(fmt.Sprintf("%s/workers=%d", tt.name, workers), func(t *testing.T) {
				t.Parallel()
				var diagnostics bytes.Buffer
				lines := []string{}
				opts := &ClocOptions{Workers: workers, Diagnostics: &diagnostics}
				tt.set(opts, func(line string) { lines = append(lines, line) })
				result := analyzeDirectory(t, dir, opts)
				if result.Total.Total != 1 || result.Files[duplicate] != nil {
					t.Fatalf("duplicate was retained: %+v", result.Files)
				}
				if !reflect.DeepEqual(lines, []string{tt.want}) {
					t.Fatalf("callbacks=%q, want [%q]", lines, tt.want)
				}
				if !opts.Debug {
					return
				}
				log := diagnostics.String()
				ignoreAt := strings.Index(log, fmt.Sprintf("[SKIP] file=%q reason=\"duplicate content\"\n", duplicate))
				analyzeAt := strings.Index(log, fmt.Sprintf("[FILE] file=%q\n", first))
				invalidOrder := ignoreAt < 0 || analyzeAt <= ignoreAt
				analyzedDuplicate := strings.Contains(log, fmt.Sprintf("[FILE] file=%q\n", duplicate))
				if invalidOrder || analyzedDuplicate {
					t.Fatalf("callbacks must exclude duplicates before line analysis: %s", log)
				}
			})
		}
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
