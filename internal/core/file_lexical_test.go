package core

import (
	"strconv"
	"strings"
	"testing"
)

func TestAnalyzeReaderCommentContext(t *testing.T) {
	tests := []struct {
		name     string
		language string
		content  string
		code     int32
		comments int32
		blanks   int32
	}{
		{
			name: "C quoted opening delimiter", language: "C",
			content: "const char *marker = \"/*\";\nint n = 1;\nint m = 2;\n", code: 3,
		},
		{
			name: "C inline comment delimiter", language: "C",
			content: "int n = 1; // /* only a line comment\nint m = 2;\n", code: 2,
		},
		{
			name: "header quoted delimiter", language: "C Header",
			content: "#define MARKER \"/*\"\nint n;\n", code: 2,
		},
		{
			name: "escaped quote", language: "C",
			content: "const char *s = \"\\\"/*\";\nint n;\n", code: 2,
		},
		{
			name: "escaped backslash before closing quote", language: "C",
			content: "const char *s = \"\\\\\"; /* real\ncomment */\nint n;\n", code: 2, comments: 1,
		},
		{
			name: "character quote", language: "C",
			content: "char c = '\"'; /* real\ncomment */\nint n;\n", code: 2, comments: 1,
		},
		{
			name: "comments do not nest", language: "C",
			content: "/* outer /* still comment */\nint n;\n", code: 1, comments: 1,
		},
		{
			name: "quote inside block comment", language: "C",
			content: "/* \" ' //\n*/ int n; // /*\nint m;\n", code: 2, comments: 1,
		},
		{
			name: "code between comments", language: "C",
			content: "/* a */ int n; /* b */\n/* c */ // d\n", code: 1, comments: 1,
		},
		{
			name: "blank inside block comment", language: "C",
			content: "/*\n \t\n*/\n", comments: 2, blanks: 1,
		},
		{
			name: "Go quoted delimiter", language: "Go",
			content: "var s = \"/*\"\nvar n = 1\n", code: 2,
		},
		{
			name: "Go inline comment delimiter", language: "Go",
			content: "var n = 1 // /*\nvar m = 2\n", code: 2,
		},
		{
			name: "Go raw multiline string", language: "Go",
			content: "var s = `hello\n// not a comment\n/* not a comment\n\n`\n// real\n", code: 4, comments: 1, blanks: 1,
		},
		{
			name: "Go raw closing then comment", language: "Go",
			content: "var s = `hello\n` /* real\ncomment */\nvar n = 1\n", code: 3, comments: 1,
		},
		{
			name: "Go rune escape", language: "Go",
			content: "var r = '\\'' // /*\nvar n = 1\n", code: 2,
		},
		{
			name: "delimiter across buffer boundary", language: "C",
			content: "char *s = \"" + strings.Repeat("x", 4090) + "/*\";\nint n;", code: 2,
		},
		{
			name: "continued C string", language: "C",
			content: "char *s = \"start\\\n/* still a string\";\nint n;\n", code: 3,
		},
		{
			name: "continued C line comment", language: "C",
			content: "int n; // comment \\\n/* still a line comment\nint m;\n", code: 2, comments: 1,
		},
		{
			name: "spaces after slash do not continue C comment", language: "C",
			content: "// comment \\ \nint n;\n", code: 1, comments: 1,
		},
		{
			name: "CRLF and unterminated final line", language: "C",
			content: "char *s = \"/*\";\r\n// comment\r\nint n;", code: 2, comments: 1,
		},
		{
			name: "leading BOM", language: "Go",
			content: "\xef\xbb\xbf// comment\nvar s = \"/*\"\n", code: 1, comments: 1,
		},
		{
			name: "invalid UTF8 after block comment", language: "C",
			content: "/*\xff*/x\n", code: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var codeCalls, commentCalls, blankCalls int32
			got := AnalyzeReader(
				"sample",
				NewDefinedLanguages().Langs[tt.language],
				strings.NewReader(tt.content),
				&ClocOptions{
					OnCode:    func(string) { codeCalls++ },
					OnComment: func(string) { commentCalls++ },
					OnBlank:   func(string) { blankCalls++ },
				},
			)
			if got.Code != tt.code || got.Comments != tt.comments || got.Blanks != tt.blanks {
				t.Fatalf("got code=%d comment=%d blank=%d; want code=%d comment=%d blank=%d",
					got.Code, got.Comments, got.Blanks, tt.code, tt.comments, tt.blanks)
			}
			if codeCalls != tt.code || commentCalls != tt.comments || blankCalls != tt.blanks {
				t.Fatalf("callback counts = %d/%d/%d", codeCalls, commentCalls, blankCalls)
			}
		})
	}
}

func FuzzAnalyzeReaderQuotedCommentMarkers(f *testing.F) {
	for _, content := range []string{"", "/*", "// /*", "\\\"/*", "`\n/*", "\xff", "世界"} {
		f.Add(content)
	}
	languages := NewDefinedLanguages()
	f.Fuzz(func(t *testing.T, content string) {
		// Quoting must isolate any comment markers, escapes and newlines in the
		// input. Check the public reader API, not just the lexer in isolation.
		source := "value = " + strconv.Quote(content) + "; // /* ignored\n" +
			"next = 1;\n/* real comment */\n\n"
		for _, name := range []string{"C", "C Header", "Go"} {
			got := AnalyzeReader("sample", languages.Langs[name], strings.NewReader(source), &ClocOptions{})
			if got.Code != 2 || got.Comments != 1 || got.Blanks != 1 {
				t.Fatalf("%s: quoted input %q produced %+v", name, content, got)
			}
		}
	})
}
