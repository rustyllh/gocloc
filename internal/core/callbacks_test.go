package gocloc

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCallbackEventsReplay(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		spill bool
	}{
		{name: "memory", limit: 64 << 10},
		{name: "spill after short events", limit: 64, spill: true},
		{name: "spill on first event", limit: 1, spill: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			events := &callbackEvents{memoryLimit: tc.limit}
			t.Cleanup(func() {
				if err := events.finish(); err != nil {
					t.Error(err)
				}
				if err := events.discard(); err != nil {
					t.Error(err)
				}
			})
			lines := []string{}
			opts := &ClocOptions{
				OnCode:    func(line string) { lines = append(lines, "code:"+line) },
				OnBlank:   func(line string) { lines = append(lines, "blank:"+line) },
				OnComment: func(line string) { lines = append(lines, "comment:"+line) },
			}
			capture := events.capture(opts)
			capture.OnCode("hello\x00世界")
			capture.OnBlank("")
			capture.OnComment("// comment\r\n")
			long := strings.Repeat("long", 2048)
			capture.OnCode(long)
			if len(lines) != 0 {
				t.Fatal("capture invoked a user callback")
			}
			if err := events.finish(); err != nil {
				t.Fatal(err)
			}
			if spilled := events.path != ""; spilled != tc.spill {
				t.Fatalf("spilled=%t, want %t", spilled, tc.spill)
			}
			if cap(events.data) > tc.limit || events.file != nil {
				t.Fatal("queued events exceeded the cache limit or retained an open file")
			}
			path := events.path
			if err := events.replay(opts); err != nil {
				t.Fatal(err)
			}
			if err := events.discard(); err != nil {
				t.Fatal(err)
			}
			want := []string{"code:hello\x00世界", "blank:", "comment:// comment\r\n", "code:" + long}
			if !reflect.DeepEqual(lines, want) {
				t.Fatal("replay changed event order or callback string contents")
			}
			if path != "" {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("spool survived discard: %v", err)
				}
			}
		})
	}
}

func TestCallbackEventsOnlyCaptureRegisteredKinds(t *testing.T) {
	t.Parallel()
	lines := []string{}
	opts := &ClocOptions{OnBlank: func(line string) { lines = append(lines, line) }}
	events := &callbackEvents{memoryLimit: 1024}
	capture := events.capture(opts)
	if capture.OnCode != nil || capture.OnComment != nil {
		t.Fatal("capture enabled unregistered callbacks")
	}
	capture.OnBlank("")
	if err := events.finish(); err != nil {
		t.Fatal(err)
	}
	if err := events.replay(opts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lines, []string{""}) {
		t.Fatalf("callbacks=%q, want one blank line", lines)
	}
}

func TestCallbackEventsSpoolWriteFailure(t *testing.T) {
	t.Parallel()
	events := &callbackEvents{memoryLimit: 1}
	t.Cleanup(func() {
		if err := events.finish(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Error(err)
		}
		if err := events.discard(); err != nil {
			t.Error(err)
		}
	})
	events.record(callbackCode, "first")
	if events.err != nil {
		t.Fatal(events.err)
	}
	if err := events.file.Close(); err != nil {
		t.Fatal(err)
	}
	events.record(callbackCode, strings.Repeat("x", 8192))
	if err := events.finish(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("finish error=%v, want closed spool error", err)
	}
	size := events.size
	events.record(callbackCode, "must not be recorded")
	if events.size != size {
		t.Fatal("record continued after a storage failure")
	}
}

func TestCallbackEventsTruncatedSpool(t *testing.T) {
	t.Parallel()
	events := &callbackEvents{memoryLimit: 1}
	t.Cleanup(func() {
		if err := events.discard(); err != nil {
			t.Error(err)
		}
	})
	events.record(callbackCode, "package sample")
	if err := events.finish(); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(events.path, callbackHeaderSize+1); err != nil {
		t.Fatal(err)
	}
	opts := &ClocOptions{OnCode: func(string) { t.Error("corrupt event triggered callback") }}
	if err := events.replay(opts); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("replay error=%v, want unexpected EOF", err)
	}
}

func TestCallbackDispatcherDrainsAfterReplayFailure(t *testing.T) {
	t.Parallel()
	broken := &callbackEvents{memoryLimit: 1}
	broken.record(callbackCode, "broken")
	if err := broken.finish(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(broken.path); err != nil {
		t.Fatal(err)
	}
	good := &callbackEvents{memoryLimit: 1024}
	good.record(callbackCode, "good")
	if err := good.finish(); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	opts := &ClocOptions{OnCode: func(line string) {
		if line != "good" {
			t.Errorf("unexpected callback: %q", line)
		}
		calls.Add(1)
	}}
	// Both jobs already hold an admission token; replay must return both.
	tokens := make(chan struct{}, 2)
	dispatcher := newCallbackDispatcher(2, opts, tokens)
	dispatcher.jobs <- callbackJob{path: "broken.go", events: broken}
	dispatcher.jobs <- callbackJob{path: "good.go", events: good}
	if err := dispatcher.finish(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("finish error=%v, want missing spool error", err)
	}
	if calls.Load() != 1 || len(tokens) != 2 {
		t.Fatalf("callbacks=%d, returned tokens=%d", calls.Load(), len(tokens))
	}
}

func setCallbackTempDir(t *testing.T, dir string) {
	t.Helper()
	// os.TempDir uses different environment variables on Unix and Windows.
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, dir)
	}
}

func TestCallbackEventsSpoolCreationFailure(t *testing.T) {
	setCallbackTempDir(t, filepath.Join(t.TempDir(), "missing"))
	events := &callbackEvents{memoryLimit: 1}
	events.record(callbackCode, "sample")
	if err := events.finish(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("finish error=%v, want missing temp directory", err)
	}
	if err := events.discard(); err != nil {
		t.Fatal(err)
	}
}
