# Reproducible benchmarks

## Fixed fixtures

`BenchmarkProcessorFixtures` generates a versioned, deterministic corpus in a temporary directory.
It needs no external checkout, network access, or machine-specific path. The fixtures use Go source;
this is a regression suite for the scanning/counting pipeline, not a language-coverage benchmark.

| Fixture (v1) | Files | Unique contents | Shape |
| --- | ---: | ---: | --- |
| `many_small` | 512 | 512 | Small files across 32 directories |
| `few_large` | 4 | 4 | About 1 MiB per file |
| `duplicates` | 512 | 32 | Repeated contents across directories |
| `comments` | 32 | 32 | Mostly multiline comments and blank lines |
| `long_lines` | 8 | 8 | Four lines exceeding 128 KiB per file |

Each fixture runs with workers **1, 2, 4, 8** and both `dedup=true` and `dedup=false` (40 cases).
`dedup=false` means `SkipDuplicated=true`, corresponding to the CLI's `--skip-duplicated` flag:
it disables duplicate detection and counts every file.

The timed region includes language/option/processor initialization, directory traversal, language detection,
file reading, optional deduplication, and line counting. Fixture generation, correctness assertions, CLI startup,
and output formatting are excluded. Each case is warmed once and checked against independently specified
file/code/comment/blank totals; duplicate fixtures also check which copy was retained.
The final measured result is checked again outside the timer.

These are **warm-cache** measurements, not cold-disk benchmarks. `ns/op` is elapsed time per corpus scan;
`B/op` and `allocs/op` are allocated bytes and allocation count per scan, not peak memory.
`MB/s` uses all input file bytes, including duplicate copies, and is not a disk-bandwidth measurement.
The suffix such as `-8` is GOMAXPROCS; `workers=8` in the name is the separate analysis worker setting.
Four large files cannot occupy eight file workers simultaneously.

## Record and compare

Run from the repository root with an idle machine, on AC power where applicable. Do not run builds,
other benchmarks, or profilers concurrently. The tests support Go 1.23+, but the pinned report tool
(`benchstat`) requires Go 1.26+. The report workflow pins Go 1.26.6.

```sh
bash scripts/benchmark.sh benchmark-results/before
# Make the code change, keeping the fixtures and toolchain unchanged.
bash scripts/benchmark.sh benchmark-results/after
go run golang.org/x/perf/cmd/benchstat@v0.0.0-20260908200009-22c9c6c9d4da benchmark-results/before/raw.txt benchmark-results/after/raw.txt
```

The script refuses an existing report directory. Without an argument it creates a new temporary directory.
It writes raw samples (`raw.txt`), a statistical summary (`summary.txt`), fixture SHA-256 fingerprints
(`fixtures.txt`), and environment/commit/settings (`environment.txt`). Keep these together.
`benchmark-results/` is ignored by Git; existing profile files and README measurements are not overwritten.

Defaults: `GOMAXPROCS=8`, `GOGC=100`, `GOMEMLIMIT=off`, `BENCHTIME=200ms`, `BENCH_COUNT=6`.
For a more stable local comparison:

```sh
BENCHTIME=1s BENCH_COUNT=10 bash scripts/benchmark.sh benchmark-results/long-run
```

Use `BENCH_FILTER` to select cases (Go benchmark regexes are slash-separated):

```sh
BENCH_FILTER='^BenchmarkProcessorFixtures$/^v1$/^many_small$/^workers=8$' bash scripts/benchmark.sh benchmark-results/small-w8
```

Compare the same CPU, OS, Go version, runtime settings, fixture version and fingerprints. If fixture contents
or expected counts change, bump the fixture version so unrelated workloads are not compared silently.
An eight-worker run on a two-core CI runner does not measure eight-core scaling. Check both the distribution
of elapsed times and allocations; do not infer performance from a single run or CPU utilization alone.

The **Benchmarks** workflow runs only on manual dispatch (**Actions → Benchmarks → Run workflow**),
not on pushes or pull requests. It stores reports
as artifacts for 30 days and adds a summary to the run. It has no timing threshold and does not gate releases;
shared GitHub runners are too variable for a reliable percentage-based gate. Test or execution failures
still fail the workflow. Download two artifacts for `benchstat`, but confirm their environments match.

## Focused profiling

First record unprofiled timings. Profiles add overhead; do not mix them into the timing baseline.
Select one case so the profile represents one workload:

```sh
go test . -run '^$' -bench '^BenchmarkProcessorFixtures$/^v1$/^many_small$/^workers=8$/^dedup=true$' -benchtime=3s -count=1 -cpuprofile=cpu-fixture.pprof
go tool pprof -http=127.0.0.1:8080 cpu-fixture.pprof
```

The CPU profile can also include untimed fixture setup and warm-up; use long runs and inspect the call tree.
Collect a separate trace with `-trace=trace-fixture.out` if scheduling or I/O waits need investigation.

## Real repository benchmark

The existing `BenchmarkProcessorAnalyze` remains available and skips unless given a path:

```sh
go test . -run '^$' -bench '^BenchmarkProcessorAnalyze$' -benchmem -benchtime=10x -count=6 -gocloc.bench-path=/path/to/checkout -gocloc.bench-workers=8
```

Add `-gocloc.bench-skip-duplicated=true` to disable deduplication. Record the target repository's commit,
branch, dirty/untracked state and tool settings separately; the real-directory benchmark does not apply
the exclusion flags shown in the README's CLI comparison. Those published CLI results and the synthetic
fixture benchmarks measure different workloads and should not be compared directly.
