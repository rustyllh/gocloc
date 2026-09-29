package gocloc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestLineReader(t *testing.T) {
	for _, size := range []int{0, 1, 4095, 4096, 4097, 128 * 1024} {
		for _, terminated := range []bool{false, true} {
			name := strings.Repeat("x", size)
			if terminated {
				name += "\n"
			}
			t.Run(fmt.Sprintf("size=%d/terminated=%t", size, terminated), func(t *testing.T) {
				reader := newLineReader(strings.NewReader(name))
				line, err := reader.next()
				if string(line) != name {
					t.Fatalf("read %d bytes, want %d", len(line), len(name))
				}
				if terminated && err != nil {
					t.Fatalf("terminated line: %v", err)
				}
				if !terminated && !errors.Is(err, io.EOF) {
					t.Fatalf("unterminated line: %v, want EOF", err)
				}
				line, err = reader.next()
				if len(line) != 0 || !errors.Is(err, io.EOF) {
					t.Fatalf("after final line: %q/%v", line, err)
				}
			})
		}
	}
}

func TestLineReaderDoesNotCountPartialLineOnReadFailure(t *testing.T) {
	failure := errors.New("injected error")
	for _, size := range []int{10, 20000} {
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			partial := strings.Repeat("x", size)
			source := io.MultiReader(strings.NewReader("code\n"+partial), failingSourceReader{err: failure})
			var diagnostics bytes.Buffer
			got := AnalyzeReader(
				"sample.go",
				NewDefinedLanguages().Langs["Go"],
				source,
				&ClocOptions{Diagnostics: &diagnostics},
			)
			if got.Code != 1 || !strings.Contains(diagnostics.String(), failure.Error()) {
				t.Fatalf("partial failing line counted or failure lost: %+v, %q", got, diagnostics.String())
			}
		})
	}
}

func TestLineReaderReplaysDetectionPrefix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "short", content: "first\nsecond\nlast"},
		{name: "long", content: strings.Repeat("x", 8192) + "\nlast\n"},
	} {
		for _, all := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/all=%t", tc.name, all), func(t *testing.T) {
				reader := newLineReader(strings.NewReader(tc.content))
				prefix := []byte{}
				var err error
				if all {
					prefix, err = io.ReadAll(reader.reader)
				} else {
					prefix, err = reader.next()
				}
				if err != nil {
					t.Fatal(err)
				}
				reader.prefix = prefix
				var got bytes.Buffer
				for {
					line, err := reader.next()
					got.Write(line)
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if got.String() != tc.content {
					t.Fatal("replayed prefix lost or repeated bytes")
				}
			})
		}
	}
}

func TestLineReaderReleasesOversizedScratch(t *testing.T) {
	reader := newLineReader(strings.NewReader(strings.Repeat("x", 1024*1024) + "\n"))
	line, err := reader.next()
	if err != nil || len(line) != 1024*1024+1 {
		t.Fatalf("long line: length=%d err=%v", len(line), err)
	}
	reader.prefix = line
	reader.release()
	if cap(reader.prefix) != 0 || cap(reader.longLine) > 64*1024 {
		t.Fatal("worker retained an oversized detection or line buffer")
	}
	reader.reset(strings.NewReader("next\n"))
	line, err = reader.next()
	if err != nil || string(line) != "next\n" {
		t.Fatalf("next file contains stale bytes: %q/%v", line, err)
	}
}
