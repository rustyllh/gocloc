package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
)

func TestBuiltInSyntaxCounts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, language, source string
		code, comments, blanks int32
	}{
		{"JS quoted delimiter", "JavaScript", "const marker = \"/*\";\nconst n = 1;\n", 2, 0, 0},
		{"JS line comment before block marker", "JavaScript", "const n = 1; // /*\nconst m = 2;\n", 2, 0, 0},
		{"JS regular expression", "JavaScript", "const r = /[/*]/;\nconst s = /https?:\\/\\/a/;\n// real\n", 2, 1, 0},
		{"JS division", "JavaScript", "const n = 10 / 2;\n// real\n", 1, 1, 0},
		{"JS return regex", "JavaScript", "function f() { return /[/*]/; }\n", 1, 0, 0},
		{"JS template interpolation", "JavaScript", "const t = `first\n// text\n${1 /* comment */ + 2}\nlast`;\n", 4, 0, 0},
		{"JS nested template", "JavaScript", "const x = `a ${`b ${1}`} c`;\n", 1, 0, 0},
		{"JS escaped template interpolation", "JavaScript", "const x = `\\${/* text */`;\n// real\n", 1, 1, 0},
		{"JS unterminated ordinary string", "JavaScript", "const s = \"unterminated\n// real\n", 1, 1, 0},
		{"TypeScript template", "TypeScript", "const value: string = `/* text`\n", 1, 0, 0},
		{"D nested comments", "D", "/+ outer /+ inner +/ outer +/\nint value;\n", 1, 1, 0},
		{"D ordinary comments do not nest", "D", "/* outer /* inner */ int value;\n", 1, 0, 0},
		{"Go raw literal", "Go", "package p\nvar s = `/*\n// text\n`\n// real\n", 4, 1, 0},
		{"C line splice", "C", "int n; // c \\\n/* still comment\nint m;\n", 2, 1, 0},
		{"BOM and blank block line", "Go", "\xef\xbb\xbfpackage p\r\n/* comment\r\n \t\r\n*/\r\n", 1, 2, 1},
		{"JS BOM CRLF and blank template line", "JavaScript", "\xef\xbb\xbfconst t = `open\r\n\r\n/* text */`\r\n// real", 2, 1, 1},
		{"JS invalid UTF8 and long line", "JavaScript", "const x = \"/*" + strings.Repeat("x", 128*1024) + "\xff\";\n// real\n", 1, 1, 0},
		{"JS unterminated regex recovers", "JavaScript", "const r = /unterminated\n// real\n", 1, 1, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var diagnostics bytes.Buffer
			file := AnalyzeReader(
				"sample",
				NewDefinedLanguages().Langs[test.language],
				strings.NewReader(test.source),
				&ClocOptions{Diagnostics: &diagnostics},
			)
			if file.Code != test.code || file.Comments != test.comments || file.Blanks != test.blanks {
				t.Fatalf("counts = %+v; want code=%d comments=%d blanks=%d",
					file, test.code, test.comments, test.blanks)
			}
			if diagnostics.Len() != 0 {
				t.Fatalf("unexpected diagnostic: %s", &diagnostics)
			}
		})
	}
}

func TestBuiltInSyntaxDebugAndReadFailure(t *testing.T) {
	t.Parallel()
	language := NewDefinedLanguages().Langs["JavaScript"]
	var diagnostics bytes.Buffer
	var calls []string
	readErr := errors.New("source failed")
	reader := io.MultiReader(
		strings.NewReader("const s = `open\n// template text\n`;\n// real\n"),
		iotest.ErrReader(readErr),
	)
	file, err := analyzeReader("sample.js", language, reader, (&ClocOptions{
		Debug:       true,
		Diagnostics: &diagnostics,
		OnCode:      func(line string) { calls = append(calls, "code:"+line) },
		OnComment:   func(line string) { calls = append(calls, "comment:"+line) },
	}).withDiagnostics())
	if !errors.Is(err, readErr) {
		t.Fatalf("read error = %v, want %v", err, readErr)
	}
	if file.Code != 3 || file.Comments != 1 {
		t.Fatalf("partial counts = %+v", file)
	}
	want := []string{"code:const s = `open", "code:// template text", "code:`;", "comment:// real"}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Fatalf("callbacks = %q, want %q", calls, want)
	}
	if !strings.Contains(diagnostics.String(), "[CODE] file=\"sample.js\" line=2") ||
		!strings.Contains(diagnostics.String(), "[COMM] file=\"sample.js\" line=4") {
		t.Fatalf("debug records = %q", &diagnostics)
	}
}

func TestBuiltInSyntaxCustomRules(t *testing.T) {
	t.Parallel()
	custom := NewLanguage("Go", []string{"#"}, [][]string{{"{-", "-}"}})
	result := AnalyzeReader(
		"sample.go", custom,
		strings.NewReader("# comment\nvar s = \"/*\"\n"),
		NewClocOptions(),
	)
	if result.Comments != 1 || result.Code != 1 {
		t.Fatalf("custom language rules were replaced: %+v", result)
	}
}

func TestBuiltInSyntaxPipelineCallbacks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sources := []string{
		"const marker = '/*';\n// comment\n",
		"const marker = '/*';\n// comment\n",
		"const value = /[/*]/;\n",
	}
	for i, source := range sources {
		path := filepath.Join(dir, fmt.Sprintf("%d.js", i))
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name           string
		workers        int
		dedup          bool
		code, comments int32
	}{
		{name: "one worker", workers: 1, code: 3, comments: 2},
		{name: "four workers", workers: 4, code: 3, comments: 2},
		{name: "dedup", workers: 4, dedup: true, code: 2, comments: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var codeCalls, commentCalls atomic.Int32
			result, err := Analyze([]string{dir}, &Options{
				Workers:   test.workers,
				Dedup:     test.dedup,
				OnCode:    func(string) { codeCalls.Add(1) },
				OnComment: func(string) { commentCalls.Add(1) },
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Total.Code != test.code || result.Total.Comments != test.comments ||
				codeCalls.Load() != test.code || commentCalls.Load() != test.comments {
				t.Fatalf("result=%+v callbacks=%d/%d", result.Total, codeCalls.Load(), commentCalls.Load())
			}
		})
	}
}
