package gocloc

import (
	"bufio"
	"bytes"
	"io"
)

// lineReader borrows short lines directly from bufio and reuses scratch space
// for arbitrarily long lines. Returned bytes are valid until the next read.
type lineReader struct {
	reader   *bufio.Reader
	longLine []byte
	prefix   []byte
}

func newLineReader(source io.Reader) *lineReader {
	return &lineReader{reader: bufio.NewReader(source), longLine: []byte{}, prefix: []byte{}}
}

func (r *lineReader) reset(source io.Reader) {
	r.reader.Reset(source)
	r.prefix = r.prefix[:0]
	r.longLine = r.longLine[:0]
}

func (r *lineReader) release() {
	// Workers keep only their ordinary read buffer and modest long-line scratch.
	// Do not retain a closed file, a detection document, or an unusually long line.
	r.reader.Reset(nil)
	r.prefix = nil
	if cap(r.longLine) > 64*1024 {
		r.longLine = nil
	}
}

func (r *lineReader) next() ([]byte, error) {
	if len(r.prefix) > 0 {
		length := len(r.prefix)
		if end := bytes.IndexByte(r.prefix, '\n'); end >= 0 {
			length = end + 1
		}
		line := r.prefix[:length]
		r.prefix = r.prefix[length:]
		return line, nil
	}
	line, err := r.reader.ReadSlice('\n')
	if err != bufio.ErrBufferFull {
		return line, err
	}
	r.longLine = append(r.longLine[:0], line...)
	for err == bufio.ErrBufferFull {
		line, err = r.reader.ReadSlice('\n')
		r.longLine = append(r.longLine, line...)
	}
	return r.longLine, err
}

func containsCommentBytes(line []byte, multiLines [][]string) bool {
	for _, comments := range multiLines {
		for _, comment := range comments {
			if bytes.Contains(line, []byte(comment)) {
				return true
			}
		}
	}
	return false
}
