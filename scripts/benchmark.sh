#!/usr/bin/env bash
# Keep each run in a new directory so a later run cannot erase the baseline.
set -euo pipefail

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ "$#" -gt 1 ]; then
    printf 'Usage: bash scripts/benchmark.sh [new-report-directory]\n' >&2
    exit 1
fi
if [ "$#" -eq 1 ]; then
    mkdir -p -- "$(dirname -- "$1")"
    mkdir -- "$1"
    report_dir=$(CDPATH= cd -- "$1" && pwd)
else
    report_dir=$(mktemp -d "${TMPDIR:-/tmp}/gocloc-bench.XXXXXX")
fi
cd -- "$repo_dir"

export GOMAXPROCS=${GOMAXPROCS:-8}
export GOGC=${GOGC:-100}
export GOMEMLIMIT=${GOMEMLIMIT:-off}
bench_time=${BENCHTIME:-200ms}
bench_count=${BENCH_COUNT:-6}
bench_filter=${BENCH_FILTER:-'^BenchmarkProcessorFixtures$'}
benchstat=golang.org/x/perf/cmd/benchstat@v0.0.0-20260908200009-22c9c6c9d4da

{
    date -u '+date: %Y-%m-%dT%H:%M:%SZ'
    printf 'commit: %s\n' "$(git rev-parse HEAD)"
    git status --short
    go version
    go env GOOS GOARCH GOAMD64 GOARM64 CGO_ENABLED GOFLAGS GOTOOLCHAIN
    uname -srm
    printf 'GOMAXPROCS=%s GOGC=%s GOMEMLIMIT=%s\n' "$GOMAXPROCS" "$GOGC" "$GOMEMLIMIT"
    printf 'bench=%s benchtime=%s count=%s\n' "$bench_filter" "$bench_time" "$bench_count"
    printf 'benchstat=%s\n' "$benchstat"
    if command -v lscpu >/dev/null 2>&1; then
        lscpu
    elif [ "$(uname -s)" = Darwin ]; then
        sysctl machdep.cpu.brand_string hw.logicalcpu hw.physicalcpu hw.memsize
        sw_vers
    fi
} > "$report_dir/environment.txt"

printf 'Benchmark report: %s\n' "$report_dir"
# Log fixture fingerprints and verify both modes before measuring anything.
go test . -run '^TestBenchmarkFixtures$' -count=1 -v | tee "$report_dir/fixtures.txt"
go test . -run '^$' -bench "$bench_filter" -benchmem \
    -benchtime="$bench_time" -count="$bench_count" | tee "$report_dir/raw.txt"
if ! grep -q '^BenchmarkProcessorFixtures/' "$report_dir/raw.txt"; then
    printf 'No fixture benchmarks matched %s\n' "$bench_filter" >&2
    exit 1
fi
go run "$benchstat" "$report_dir/raw.txt" | tee "$report_dir/summary.txt"
