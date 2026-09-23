# gocloc

[English](README.md) | [简体中文](README.zh-CN.md)

[![GoDoc](https://godoc.org/github.com/rustyllh/gocloc?status.svg)](https://godoc.org/github.com/rustyllh/gocloc)
[![ci](https://github.com/rustyllh/gocloc/workflows/Go/badge.svg)](https://github.com/rustyllh/gocloc/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/rustyllh/gocloc)](https://goreportcard.com/report/github.com/rustyllh/gocloc)
[![Docker Pulls](https://img.shields.io/docker/pulls/rustyllh/gocloc)](https://hub.docker.com/r/rustyllh/gocloc)
[![Docker Image Size](https://img.shields.io/docker/image-size/rustyllh/gocloc/latest)](https://hub.docker.com/r/rustyllh/gocloc)

A fast, parallel source code line counter written in Go, based on [hhatto/gocloc](https://github.com/hhatto/gocloc).
Inspired by [tokei](https://github.com/Aaronepower/tokei), with performance optimizations for file scanning and line counting.

## Installation

### macOS / Linux

Install the latest stable release (no Go required; SHA-256 verified):

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://raw.githubusercontent.com/rustyllh/gocloc/master/install.sh | sh
```

Requires `curl`, `tar`, and `sha256sum` or `shasum`. Installs to `$HOME/.local/bin`; add it to your `PATH` if needed.

### Windows

Run in PowerShell 5.1+:

```powershell
& ([scriptblock]::Create((Invoke-RestMethod https://raw.githubusercontent.com/rustyllh/gocloc/master/install.ps1 -ErrorAction Stop)))
```

Installs to `%LOCALAPPDATA%\gocloc\bin`; add it to your user `Path`. Supports x86-64 and i386, not Windows ARM64.

Both installers execute downloaded code; review [install.sh](install.sh) or [install.ps1](install.ps1) first if needed.
Run the installer again to upgrade, then check `gocloc --version`.
Alternatively, download a binary and its checksums from [GitHub Releases](https://github.com/rustyllh/gocloc/releases).

### Go

With the latest stable Go release:

```sh
go install github.com/rustyllh/gocloc/cmd/gocloc@latest
```

### Docker

Run against the current directory with a read-only mount:

```sh
docker run --rm --read-only --mount "type=bind,source=$(pwd),target=/workdir,readonly" rustyllh/gocloc:latest .
```

Supports Linux `amd64` and `arm64`, selected automatically. Use `:vX.Y.Z` to pin a release; `:latest` tracks stable releases.
Append CLI options after the image name, for example `--dedup -o json .`.

## Performance

* CPU Apple M3 8-core / 16 GiB / macOS arm64 / Go 1.26.6
* cloc 2.04
* tokei 14.0.0 compiled with serialization support: json
* upstream gocloc [679b457](https://github.com/hhatto/gocloc/commit/679b457182dcf852d90e52f132403d6b7de0e33d)
* optimized gocloc, based on upstream [679b457](https://github.com/hhatto/gocloc/commit/679b457182dcf852d90e52f132403d6b7de0e33d)
* target repository is [golang/go](https://github.com/golang/go), branch `master`, commit [be1160f](https://github.com/golang/go/commit/be1160f2a446665d6c0ccd2344c0cbe365bbc3a4)

All tools scanned the Go repository revision above with `dist`, `node_modules`, and `target` excluded. The command output is from a representative warm-cache run. The `time` lines are warm-cache averages: 10 runs for tokei and upstream gocloc, 30 runs for optimized gocloc (three batches of 10), and 3 runs for cloc. Optimized gocloc uses 8 workers.

Optimized gocloc and tokei were retested on 2026-09-23 with macOS 27.0. Runs were interleaved after two untimed warm-up runs per command, with timed stdout redirected to `/dev/null`.
Go runtime settings were `GOMAXPROCS=8`, `GOGC=100`, and `GOMEMLIMIT=off`. Background applications were active; no samples were discarded.
Median elapsed times were 0.225 s for optimized gocloc with deduplication and 0.210 s for tokei.
cloc and upstream gocloc retain historical measurements on macOS 26.6.1; the full comparison is not a controlled same-environment test.
Both gocloc output blocks below use deduplication; the optimized command enables it explicitly with `--dedup`.
These CLI measurements do not register library callbacks and do not measure deferred callback replay.

### cloc

```
$ time cloc --exclude-dir=dist,node_modules,target .

3 errors:
Line count, exceeded timeout:  ./src/net/http/requestwrite_test.go
Line count, exceeded timeout:  ./src/vendor/golang.org/x/net/idna/tables15.0.0.go
Line count, exceeded timeout:  ./src/vendor/golang.org/x/net/idna/tables17.0.0.go
github.com/AlDanial/cloc v 2.04  T=25.75 s (544.9 files/s, 146368.5 lines/s)
-----------------------------------------------------------------------------------
Language                         files          blank        comment           code
-----------------------------------------------------------------------------------
Go                               11471         273077         457997        2509333
Text                              1470          14788              0         233053
Assembly                           652          16138          24084         149105
HTML                                15           2098             50          20117
Snakemake                           28           2200              0          19016
JSON                                40            124              0          14186
YAML                                68            361            364           6563
C                                  113            968            845           5547
Markdown                            61           1398             35           4684
CSV                                  1              0              0           2118
-----------------------------------------------------------------------------------
SUM:                             14032         312435         485754        2970886
-----------------------------------------------------------------------------------
cloc --exclude-dir=dist,node_modules,target .  22.657s user 1.570s system 90.7% cpu 26.583 total
```

### tokei

```
$ time tokei . -e '{dist,node_modules,target}/' -s lines -C
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Language              Files        Lines         Code     Comments       Blanks
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Go                    11674      3277332      2557810       451710       267812
 Plain Text             1475       247946            0       233140        14806
 GNU Style Assembly      655       189365       153620        19596        16149
 Snakemake                28        21216        19871            0         1345
 JSON                     38        14287        14163            0          124
 HTML                     15        12849        12465           37          347
 C                       117         7431         5657          856          918
 YAML                     59         6782         6135          331          316
 Markdown                 66         5562            0         4165         1397
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Total                 14256      3803655      2787281       711745       304629
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
tokei . -e '{dist,node_modules,target}/' -s lines -C  0.580s user 0.765s system 608.6% cpu 0.222 total
```

### upstream gocloc (https://github.com/hhatto/gocloc)

```
$ time gocloc-upstream --not-match-d='dist|node_modules|target' .
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Go                           11482         273107         481370        2485725
Plain Text                    1465          14779              0         233006
Assembly                       652          16149          24083         149095
HTML                            15           2098            180          19987
JSON                            40            124              0          14186
YAML                            59            326            361           6095
C                              113            968            846           5546
Markdown                        59           1391             35           4674
BASH                            31            362           1144           2228
JavaScript                       9            301            332           1705
-------------------------------------------------------------------------------
TOTAL                        13995         310202         509643        2924932
-------------------------------------------------------------------------------
gocloc-upstream --not-match-d='dist|node_modules|target' .  0.628s user 0.638s system 110.3% cpu 1.142 total
```

### optimized gocloc

```
$ time gocloc --dedup --not-match-d='dist|node_modules|target' --workers=8 .
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Go                           11482         273107         481370        2485725
Plain Text                    1465          14779              0         233006
Assembly                       652          16149          24083         149095
HTML                            15           2098            180          19987
JSON                            40            124              0          14186
YAML                            59            326            361           6095
C                              113            968            846           5546
Markdown                        59           1391             35           4674
BASH                            31            362           1144           2228
JavaScript                       9            301            332           1705
-------------------------------------------------------------------------------
TOTAL                        13995         310202         509643        2924932
-------------------------------------------------------------------------------
gocloc --dedup --not-match-d='dist|node_modules|target' --workers=8 .  0.517s user 0.822s system 581.1% cpu 0.232 total
```

Without `--dedup` (the default), optimized gocloc counted 14,225 files with the same exclusions and 8 workers.
Over 30 interleaved warm-cache runs, elapsed time averaged **0.210 s**, with a median of **0.197 s**.
Counting rules differ between tools, so similar timings do not imply identical work.

## Usage

```sh
# Count one or more directories
gocloc .
gocloc src tests

# Exclude dependency and build directories
gocloc --not-match-d='(^|[/\\])(dist|node_modules|target)([/\\]|$)' .

# Filter languages or exclude extensions
gocloc -l Go,Python .
gocloc -e txt,md .

# Report each file, sorted by code lines
gocloc -f -s code .

# Export JSON outside the scanned directory
gocloc -o json . > ../counts.json

# Count identical contents only once (off by default)
gocloc --dedup .

# List supported languages / show all options
gocloc -L
gocloc -h
```

Workers are selected automatically; use `-w 1` through `-w 64` to override.
`-o` supports `default`, `json`, `markdown`, `cloc-xml`, and `sloccount`.
Short and long options can be mixed; see `gocloc -h` for the full list.

`--not-match-d` uses a regular expression against directory paths, not a glob.
Keep generated reports outside the scanned tree or explicitly exclude them. `>` overwrites the destination.
Older releases enabled deduplication by default; use `--dedup` to retain that behavior.

`--debug` writes analysis activity to stderr without disabling parallel workers.
Line records include the file path and line number; records from different files may interleave.
With `--dedup`, a duplicate can produce analysis logs before a `[SKIP]` record excludes it from the totals.

### Library callbacks

`Processor.Analyze` runs `OnCode`, `OnComment`, and `OnBlank` in workers. With multiple
workers, callbacks from different files may run concurrently; callers must protect shared state.
For example, pass these options to `NewProcessor` (the counter uses `sync/atomic`):

```go
options := gocloc.NewClocOptions()
options.Workers = 8
options.SkipDuplicated = false // Optional: count identical content only once.
var codeLines atomic.Int64
options.OnCode = func(string) { codeLines.Add(1) }
```

Calls within one file follow line order; order across files is unspecified. `Workers` limits
concurrent callback execution as well as file analysis. Set `Workers = 1` for non-overlapping
callbacks within one `Analyze` call; this does not put callbacks on the caller's goroutine.
Callbacks must return for `Analyze` to finish, and `Analyze` waits for all callbacks before
returning, including on errors. `AnalyzeFile` and `AnalyzeReader` remain synchronous.
This changes the earlier library contract that kept processor callbacks on the caller's goroutine.
Existing callers must synchronize shared state or explicitly use one worker.

Callbacks follow the deduplication setting. Without deduplication, every copy triggers callbacks
during analysis; later read errors cannot undo calls already made. With deduplication, workers
read each source file once while hashing, counting, and recording callback events. Results are
deduplicated in discovery order, then only the first retained copy's events are replayed in
callback workers. Failed source reads and discarded copies do not trigger callbacks. Debug
logs still show actual analysis, including discarded copies.

Deferred events share a 16 MiB in-memory buffer-capacity budget per scan (not a total process
memory limit). Large buffers spill to private files in the OS temporary directory and are
removed after replay or exclusion. Queued spools do not hold open file descriptors. Parser
buffers, individual callback strings, and memory retained by callers are outside this budget.
Spool write failures exclude the affected file with a diagnostic; replay or cleanup failures
are returned after workers finish. Already delivered callbacks cannot be rolled back.

## Jenkins

Install the [SLOCCount plugin](https://plugins.jenkins.io/sloccount/) on Jenkins and put `gocloc` on the agent's `PATH`.
Add this stage to a Declarative Pipeline running on a Unix agent with the source checked out:

```groovy
stage('Count lines') {
    steps {
        sh 'mkdir -p reports && gocloc -f -o sloccount --not-match-d=reports . > reports/sloccount.sc'
        sloccountPublish pattern: 'reports/sloccount.sc', encoding: 'UTF-8'
    }
}
```

The report directory is excluded from counting. For Freestyle jobs, run the same shell command and configure
**Publish SLOCCount analysis results** with the pattern `reports/sloccount.sc`.

## License

MIT
