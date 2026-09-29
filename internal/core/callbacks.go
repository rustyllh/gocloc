package gocloc

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// This bounds retained event-buffer capacity across the entire result window,
// including files awaiting replay. Parser buffers and caller-owned strings are
// separate. A large event may still require one line-sized allocation on replay.
const callbackMemoryBudget = 16 << 20

const (
	callbackCode byte = iota
	callbackBlank
	callbackComment
	callbackHeaderSize = 9 // Kind byte followed by uint64 byte length.
)

// Ownership moves from one analysis worker to the ordered collector, then to
// one replay worker (or discard). No event buffer is shared while being mutated.
type callbackEvents struct {
	memoryLimit int
	data        []byte
	size        int64
	file        *os.File
	writer      *bufio.Writer
	path        string
	err         error
}

func (events *callbackEvents) capture(opts *ClocOptions) *ClocOptions {
	prepared := *opts
	if opts.OnCode != nil {
		prepared.OnCode = func(line string) { events.record(callbackCode, line) }
	}
	if opts.OnBlank != nil {
		prepared.OnBlank = func(line string) { events.record(callbackBlank, line) }
	}
	if opts.OnComment != nil {
		prepared.OnComment = func(line string) { events.record(callbackComment, line) }
	}
	return &prepared
}

func (events *callbackEvents) record(kind byte, line string) {
	if events.err != nil {
		return
	}
	var header [callbackHeaderSize]byte
	header[0] = kind
	binary.LittleEndian.PutUint64(header[1:], uint64(len(line)))
	if events.writer == nil && len(line) <= events.memoryLimit-len(events.data)-len(header) {
		needed := len(events.data) + len(header) + len(line)
		if needed > cap(events.data) {
			capacity := min(events.memoryLimit, max(needed, 2*cap(events.data)))
			data := make([]byte, len(events.data), capacity)
			copy(data, events.data)
			events.data = data
		}
		events.data = append(events.data, header[:]...)
		events.data = append(events.data, line...)
		events.size += int64(len(header)) + int64(len(line))
		return
	}
	if events.writer == nil {
		if events.err = events.spill(); events.err != nil {
			return
		}
	}
	if _, events.err = events.writer.Write(header[:]); events.err != nil {
		return
	}
	if _, events.err = events.writer.WriteString(line); events.err != nil {
		return
	}
	events.size += int64(len(header)) + int64(len(line))
}

func (events *callbackEvents) spill() error {
	file, err := os.CreateTemp("", "gocloc-callbacks-*")
	if err != nil {
		return err
	}
	events.file = file
	events.path = file.Name()
	events.writer = bufio.NewWriterSize(file, min(4096, events.memoryLimit))
	_, err = events.writer.Write(events.data)
	events.data = nil // Release the in-memory cache once it has been spooled.
	return err
}

// Close spool files before queueing results; queued files must not consume file
// descriptors. Only active analysis and replay workers keep descriptors open.
func (events *callbackEvents) finish() error {
	if events.file != nil {
		events.err = errors.Join(events.err, events.writer.Flush(), events.file.Close())
		events.file = nil
		events.writer = nil
	}
	return events.err
}

func (events *callbackEvents) replay(opts *ClocOptions) (err error) {
	var reader io.Reader = bytes.NewReader(events.data)
	if events.path != "" {
		file, openErr := os.Open(events.path)
		if openErr != nil {
			return openErr
		}
		defer func() { err = errors.Join(err, file.Close()) }()
		reader = bufio.NewReader(file)
	}
	for remaining := events.size; remaining > 0; {
		var header [callbackHeaderSize]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return err
		}
		remaining -= int64(len(header))
		size := binary.LittleEndian.Uint64(header[1:])
		invalidLength := remaining < 0 || size > uint64(remaining) || size > uint64(^uint(0)>>1)
		if invalidLength {
			return fmt.Errorf("invalid callback event length: %d", size)
		}
		line := make([]byte, int(size))
		if _, err := io.ReadFull(reader, line); err != nil {
			return err
		}
		remaining -= int64(size)
		var callback func(string)
		switch header[0] {
		case callbackCode:
			callback = opts.OnCode
		case callbackBlank:
			callback = opts.OnBlank
		case callbackComment:
			callback = opts.OnComment
		}
		if callback == nil {
			return fmt.Errorf("invalid callback event kind: %d", header[0])
		}
		callback(string(line))
	}
	return nil
}

func (events *callbackEvents) discard() error {
	events.data = nil
	if events.path == "" {
		return nil
	}
	err := os.Remove(events.path)
	events.path = ""
	return err
}

type callbackJob struct {
	path   string
	events *callbackEvents
}

type callbackDispatcher struct {
	jobs    chan callbackJob
	workers sync.WaitGroup
	errOnce sync.Once
	err     error
}

func newCallbackDispatcher(workers int, opts *ClocOptions, tokens chan<- struct{}) *callbackDispatcher {
	dispatcher := &callbackDispatcher{jobs: make(chan callbackJob)}
	for range workers {
		dispatcher.workers.Add(1)
		go func() {
			defer dispatcher.workers.Done()
			for job := range dispatcher.jobs {
				err := errors.Join(job.events.replay(opts), job.events.discard())
				if err != nil {
					dispatcher.report(fmt.Errorf("callbacks for %q: %w", job.path, err))
				}
				tokens <- struct{}{}
			}
		}()
	}
	return dispatcher
}

func (dispatcher *callbackDispatcher) report(err error) {
	if err != nil {
		dispatcher.errOnce.Do(func() { dispatcher.err = err })
	}
}

// Always drain and join the workers, including after replay or diagnostic errors.
func (dispatcher *callbackDispatcher) finish() error {
	close(dispatcher.jobs)
	dispatcher.workers.Wait()
	return dispatcher.err
}
