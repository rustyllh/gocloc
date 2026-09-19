package gocloc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDiagnosticsSerializeConcurrentWrites(t *testing.T) {
	var output bytes.Buffer
	opts := (&ClocOptions{Diagnostics: &output}).withDiagnostics()
	var workers sync.WaitGroup
	for i := range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := range 50 {
				opts.diagnosticf("worker=%d item=%d\n", i, j)
			}
		}()
	}
	workers.Wait()
	if err := opts.diagnosticError(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	seen := make(map[string]bool)
	for _, line := range lines {
		seen[line] = true
	}
	if len(lines) != 400 || len(seen) != 400 {
		t.Fatalf("diagnostics interleaved or lost: %d lines, %d distinct", len(lines), len(seen))
	}
	for i := range 8 {
		for j := range 50 {
			if !seen[fmt.Sprintf("worker=%d item=%d", i, j)] {
				t.Fatalf("missing diagnostic %d/%d", i, j)
			}
		}
	}
}

type diagnosticFailureWriter struct {
	err error
}

func (w diagnosticFailureWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return len(p) / 2, nil
}

func TestProcessorReportsDiagnosticWriterFailure(t *testing.T) {
	failure := errors.New("diagnostic output is closed")
	for _, tc := range []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "write error", writer: diagnosticFailureWriter{err: failure}, want: failure},
		{name: "short write", writer: diagnosticFailureWriter{}, want: io.ErrShortWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, workers := range []int{1, 8} {
				opts := &ClocOptions{Workers: workers, Diagnostics: tc.writer}
				_, err := NewProcessor(NewDefinedLanguages(), opts).Analyze([]string{filepath.Join(t.TempDir(), "missing")})
				if !errors.Is(err, tc.want) {
					t.Fatalf("workers=%d error=%v, want %v", workers, err, tc.want)
				}
			}
		})
	}
}

func TestProcessorNormalExclusionsDoNotWarn(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"a.go": "package sample\n", "b.go": "package sample\n",
		"unknown.xyz": "unknown\n", "excluded.py": "print(1)\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, workers := range []int{1, 8} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			var diagnostics bytes.Buffer
			opts := &ClocOptions{
				Workers: workers, Diagnostics: &diagnostics, ExcludeExts: map[string]struct{}{"Python": {}},
			}
			result := analyzeDirectory(t, dir, opts)
			if result.Total.Total != 1 || result.Total.Code != 1 || diagnostics.Len() != 0 {
				t.Fatalf("total=%+v diagnostics=%q", result.Total, diagnostics.String())
			}
			if opts.diagnostics != nil {
				t.Fatal("Analyze mutated caller options")
			}
		})
	}
}
