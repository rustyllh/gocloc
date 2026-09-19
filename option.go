package gocloc

import (
	"io"
	"regexp"
)

// MaxWorkers limits fixed worker configurations to a bounded resource footprint.
const MaxWorkers = 64

// ClocOptions is gocloc processor options.
type ClocOptions struct {
	Debug          bool
	SkipDuplicated bool
	ExcludeExts    map[string]struct{}
	IncludeLangs   map[string]struct{}
	ReNotMatch     *regexp.Regexp
	ReMatch        *regexp.Regexp
	ReNotMatchDir  *regexp.Regexp
	ReMatchDir     *regexp.Regexp
	Fullpath       bool
	Workers        int

	// Diagnostics receives warnings and debug logs, never statistical output.
	// Nil uses os.Stderr; io.Discard silences diagnostics. Processor.Analyze
	// serializes writes within a scan, including writes from traversal and workers.
	Diagnostics io.Writer
	diagnostics *diagnosticSink

	// OnCode is triggered for each line of code.
	OnCode func(line string)
	// OnBlank is triggered for each blank line.
	OnBlank func(line string)
	// OnComment is triggered for each line of comments.
	OnComment func(line string)
}

// NewClocOptions create new ClocOptions with default values.
func NewClocOptions() *ClocOptions {
	return &ClocOptions{
		Debug:          false,
		SkipDuplicated: false,
		ExcludeExts:    make(map[string]struct{}),
		IncludeLangs:   make(map[string]struct{}),
	}
}
