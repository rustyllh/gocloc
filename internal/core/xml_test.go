package gocloc

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"testing"
)

func TestXMLResultEncodeTo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result XMLResult
		want   string
	}{
		{name: "empty", result: XMLResult{}, want: "<results></results>"},
		{
			name: "files", result: XMLResult{XMLFiles: &XMLResultFiles{Total: XMLTotalFiles{Code: 2}}},
			want: "<results>\n  <files>\n    <total code=\"2\" comment=\"0\" blank=\"0\"></total>\n  </files>\n</results>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := tc.result.EncodeTo(&output); err != nil {
				t.Fatal(err)
			}
			if got, want := output.String(), xml.Header+tc.want+"\n"; got != want {
				t.Fatalf("XML=%q, want %q", got, want)
			}
		})
	}
}

func TestXMLResultEncodeToReturnsWriteError(t *testing.T) {
	failure := errors.New("write failure")
	for _, tc := range []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "error", writer: diagnosticFailureWriter{err: failure}, want: failure},
		{name: "short write", writer: diagnosticFailureWriter{}, want: io.ErrShortWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := (&XMLResult{}).EncodeTo(tc.writer); !errors.Is(err, tc.want) {
				t.Fatalf("error=%v, want %v", err, tc.want)
			}
		})
	}
}
