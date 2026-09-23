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
	"sync/atomic"
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

func TestProcessorDebugDrainsWorkersAfterDiagnosticFailure(t *testing.T) {
	dir := t.TempDir()
	// A failed debug writer must not strand candidates beyond the result window.
	for i := range parallelResultWindowSize(8) + 1 {
		path := filepath.Join(dir, fmt.Sprintf("sample%03d.go", i))
		if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	failure := errors.New("debug output is closed")
	for _, tc := range []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "write error", writer: diagnosticFailureWriter{err: failure}, want: failure},
		{name: "short write", writer: diagnosticFailureWriter{}, want: io.ErrShortWrite},
	} {
		for _, workers := range []int{1, 8} {
			t.Run(fmt.Sprintf("%s/workers=%d", tc.name, workers), func(t *testing.T) {
				t.Parallel()
				opts := &ClocOptions{Workers: workers, Debug: true, Diagnostics: tc.writer}
				_, err := NewProcessor(NewDefinedLanguages(), opts).Analyze([]string{dir})
				if !errors.Is(err, tc.want) {
					t.Fatalf("error=%v, want %v", err, tc.want)
				}
			})
		}
	}
}

func TestProcessorConcurrentCallbacksFinishAfterDiagnosticFailure(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "write error", writer: diagnosticFailureWriter{err: io.ErrClosedPipe}, want: io.ErrClosedPipe},
		{name: "short write", writer: diagnosticFailureWriter{}, want: io.ErrShortWrite},
	} {
		for _, skip := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/skip=%t", tc.name, skip), func(t *testing.T) {
				t.Parallel()
				var calls atomic.Int32
				opts := &ClocOptions{
					Workers: 8, Debug: true,
					SkipDuplicated: skip, Diagnostics: tc.writer,
					OnCode: func(string) { calls.Add(1) },
				}
				_, err := NewProcessor(NewDefinedLanguages(), opts).Analyze([]string{dir})
				if !errors.Is(err, tc.want) {
					t.Fatalf("error=%v, want %v", err, tc.want)
				}
				wantCalls := int32(1)
				if skip {
					wantCalls = 2
				}
				if calls.Load() != wantCalls {
					t.Fatalf("completed callbacks=%d, want %d", calls.Load(), wantCalls)
				}
			})
		}
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
