package gocloc

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

func checkReaderAgainstReference(t testing.TB, language *Language, content string) {
	t.Helper()
	type observation struct {
		file        *ClocFile
		err         error
		lines       []string
		diagnostics bytes.Buffer
	}
	observe := func(parse func(string, *Language, io.Reader, *ClocOptions) (*ClocFile, error)) observation {
		got := observation{lines: []string{}}
		opts := &ClocOptions{
			Debug:       true,
			Diagnostics: &got.diagnostics,
			OnCode:      func(line string) { got.lines = append(got.lines, "code:"+line) },
			OnComment:   func(line string) { got.lines = append(got.lines, "comment:"+line) },
			OnBlank:     func(line string) { got.lines = append(got.lines, "blank:"+line) },
		}
		got.file, got.err = parse("sample", language, strings.NewReader(content), opts)
		return got
	}
	want := observe(referenceAnalyzeReader)
	got := observe(analyzeReader)
	if !reflect.DeepEqual(got.file, want.file) || fmt.Sprint(got.err) != fmt.Sprint(want.err) {
		t.Fatalf("counts/error = %+v/%v, want %+v/%v", got.file, got.err, want.file, want.err)
	}
	if !reflect.DeepEqual(got.lines, want.lines) || got.diagnostics.String() != want.diagnostics.String() {
		t.Fatal("callback contents/order or debug output differ from the reference parser")
	}
	// Also check the allocation-sensitive path without callbacks or debug output.
	fast, err := analyzeReader("sample", language, strings.NewReader(content), &ClocOptions{})
	if !reflect.DeepEqual(fast, want.file) || fmt.Sprint(err) != fmt.Sprint(want.err) {
		t.Fatalf("plain counts/error = %+v/%v, want %+v/%v", fast, err, want.file, want.err)
	}
}

func TestAnalyzeReaderMatchesReference(t *testing.T) {
	contents := []struct {
		name string
		text string
	}{
		{name: "empty"},
		{name: "whitespace", text: "\n\r\n\t \v\f\u0085\u00a0\u2003\u3000\n"},
		{name: "bom", text: "\xef\xbb\xbf// comment\n\xef\xbb\xbfcode\n"},
		{name: "shebang", text: "\n\u2003#!/usr/bin/env python\n# comment\nprint(1)"},
		{name: "comments", text: "a /* outer /* inner */ tail */ b\n/*\n\n*/\n// comment\n"},
		{name: "delimiters", text: "(* outer (* nested *) *)\n\"\"\"text\nend\"\"\"\n<!-- c -->\n{- c -}\n"},
		{name: "unterminated", text: "a\n/* comment\nlast"},
		{name: "invalid utf8", text: "\xff\xfe\n/*\xff*/x\n\u2003\xff\n"},
		{name: "long lines", text: "//" + strings.Repeat("x", 128*1024) + "\n\nlast"},
		{name: "buffer boundary", text: strings.Repeat("x", 4095) + "\n// boundary\n"},
	}
	for name, language := range NewDefinedLanguages().Langs {
		t.Run(name, func(t *testing.T) {
			for _, content := range contents {
				t.Run(content.name, func(t *testing.T) {
					checkReaderAgainstReference(t, language, content.text)
				})
			}
		})
	}
}

func TestAnalyzeReaderRetainsCallbackStrings(t *testing.T) {
	lines := []string{}
	want := []string{}
	for i := range 300 {
		want = append(want, fmt.Sprintf("value%03d = %s", i, strings.Repeat("x", 50)))
	}
	opts := &ClocOptions{OnCode: func(line string) { lines = append(lines, line) }}
	content := strings.Join(want, "\n")
	got := AnalyzeReader("sample.go", NewDefinedLanguages().Langs["Go"], strings.NewReader(content), opts)
	if got.Code != int32(len(want)) || !reflect.DeepEqual(lines, want) {
		t.Fatal("callbacks must own strings after the read buffer is reused")
	}
}

func TestAnalyzeReaderFragmentedInput(t *testing.T) {
	content := "\u2003// comment\r\n" + strings.Repeat("x", 8192) + "\n/*\n\n*/\nlast"
	language := NewDefinedLanguages().Langs["Go"]
	want, err := referenceAnalyzeReader("sample", language, strings.NewReader(content), &ClocOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]io.Reader{
		"one byte": iotest.OneByteReader(strings.NewReader(content)),
		"half":     iotest.HalfReader(strings.NewReader(content)),
		"data eof": iotest.DataErrReader(strings.NewReader(content)),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := analyzeReader("sample", language, source, &ClocOptions{})
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v/%v, want %+v", got, err, want)
			}
		})
	}
}

func TestDetectAndAnalyzeFileReusesBufferAcrossFiles(t *testing.T) {
	languages := NewDefinedLanguages()
	reader := newLineReader(nil)
	dir := t.TempDir()
	for _, tc := range []struct {
		name    string
		content string
		lang    string
	}{
		{name: "long.go", content: "//" + strings.Repeat("x", 1024*1024) + "\nvar x = 1", lang: "Go"},
		{name: "short.go", content: "/* comment\n", lang: "Go"},
		{name: "unknown.zzz", content: "ignore this\n"},
		{name: "detected.ts", content: "export const x: number = 1;\n\n// hello\n", lang: "TypeScript"},
		{name: "short.py", content: "print(1)\n# comment", lang: "Python"},
		{name: "empty.go", lang: "Go"},
		{name: "override.go", content: "\u2003#!/usr/bin/env python\n# comment\nprint(1)", lang: "Python"},
		{name: "Makefile", content: "all:\n\techo hello\n", lang: "Makefile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, skip := range []bool{false, true} {
				opts := &ClocOptions{SkipDuplicated: skip}
				got := detectAndAnalyzeFile(fileCandidate{path: path}, languages, opts, reader)
				if tc.lang == "" {
					if got.err != nil || !got.ignored {
						t.Fatalf("excluded file: %+v", got)
					}
					continue
				}
				want, err := referenceAnalyzeReader(path, languages.Langs[tc.lang], strings.NewReader(tc.content), opts)
				if err != nil || got.err != nil || got.ignored || !reflect.DeepEqual(got.clocFile, want) {
					t.Fatalf("got %+v/%+v, want %+v/%v", got, got.clocFile, want, err)
				}
				if !skip && got.digest != md5.Sum([]byte(tc.content)) {
					t.Fatal("buffer reuse changed the digest")
				}
				if skip && got.digest != [md5.Size]byte{} {
					t.Fatal("hash leaked from the previous file")
				}
			}
		})
	}
}

func FuzzAnalyzeReaderMatchesReference(f *testing.F) {
	for _, content := range []string{"", "\n", "a /* b */ c\n", "\xef\xbb\xbf//x", "\xff\n", "#!/bin/sh\n"} {
		f.Add(content, uint8(0))
	}
	defined := NewDefinedLanguages()
	languages := []*Language{defined.Langs["Go"], defined.Langs["Python"], defined.Langs["ATS"], defined.Langs["Perl"]}
	f.Fuzz(func(t *testing.T, content string, index uint8) {
		checkReaderAgainstReference(t, languages[int(index)%len(languages)], content)
	})
}

func BenchmarkAnalyzeReader(b *testing.B) {
	language := NewDefinedLanguages().Langs["Go"]
	opts := NewClocOptions()
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "code", body: strings.Repeat("var value = 123456789\n", 8192)},
		{name: "comments", body: strings.Repeat("/* block\ncomment text\n*/\n\n", 2048)},
		{name: "long_lines", body: strings.Repeat(strings.Repeat("x", 128*1024)+"\n", 4)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.body)))
			for range b.N {
				result, err := analyzeReader("sample.go", language, strings.NewReader(tc.body), opts)
				if err != nil || result.Code+result.Comments+result.Blanks == 0 {
					b.Fatalf("invalid analysis: %+v/%v", result, err)
				}
			}
		})
	}
}
