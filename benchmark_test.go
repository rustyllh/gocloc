package gocloc

import (
	"flag"
	"os"
	"runtime"
	"testing"
)

var (
	benchmarkPath           = flag.String("gocloc.bench-path", "", "directory to analyze in BenchmarkProcessorAnalyze")
	benchmarkWorkers        = flag.Int("gocloc.bench-workers", runtime.GOMAXPROCS(0), "number of analysis workers in BenchmarkProcessorAnalyze")
	benchmarkSkipDuplicated = flag.Bool("gocloc.bench-skip-duplicated", false, "skip duplicate-file detection in BenchmarkProcessorAnalyze")
)

func TestNewBenchmarkOptions(t *testing.T) {
	opts := newBenchmarkOptions(4, true)

	if opts.Workers != 4 {
		t.Fatalf("Workers = %d, want 4", opts.Workers)
	}
	if !opts.SkipDuplicated {
		t.Fatal("SkipDuplicated = false, want true")
	}
}

func newBenchmarkOptions(workers int, skipDuplicated bool) *ClocOptions {
	opts := NewClocOptions()
	opts.Workers = workers
	opts.SkipDuplicated = skipDuplicated
	return opts
}

// BenchmarkProcessorAnalyze profiles real-directory scanning, duplicate detection,
// and line analysis. Supply -gocloc.bench-path to run it.
func BenchmarkProcessorAnalyze(b *testing.B) {
	b.StopTimer()
	if *benchmarkPath == "" {
		b.Skip("set -gocloc.bench-path to the directory to analyze")
	}
	info, err := os.Stat(*benchmarkPath)
	if err != nil {
		b.Fatalf("stat benchmark path: %v", err)
	}
	if !info.IsDir() {
		b.Fatalf("benchmark path %q is not a directory", *benchmarkPath)
	}

	paths := []string{*benchmarkPath}
	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	for range b.N {
		processor := NewProcessor(NewDefinedLanguages(), newBenchmarkOptions(*benchmarkWorkers, *benchmarkSkipDuplicated))
		if _, err := processor.Analyze(paths); err != nil {
			b.Fatal(err)
		}
	}
}
