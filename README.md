# gocloc

[![GoDoc](https://godoc.org/github.com/rustyllh/gocloc?status.svg)](https://godoc.org/github.com/rustyllh/gocloc)
[![ci](https://github.com/rustyllh/gocloc/workflows/Go/badge.svg)](https://github.com/rustyllh/gocloc/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/rustyllh/gocloc)](https://goreportcard.com/report/github.com/rustyllh/gocloc)

A little fast [cloc(Count Lines Of Code)](https://github.com/AlDanial/cloc), written in Go.
Inspired by [tokei](https://github.com/Aaronepower/tokei).

## Installation

require Go 1.19+

```
$ go install github.com/rustyllh/gocloc/cmd/gocloc@latest
```

Arch Linux user can also install from AUR: [gocloc-git](https://aur.archlinux.org/packages/gocloc-git/).

## Usage

### Basic Usage
```
$ gocloc .
```

```
$ gocloc .
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Markdown                         3              8              0             18
Go                               1             29              1            323
-------------------------------------------------------------------------------
TOTAL                            4             37              1            341
-------------------------------------------------------------------------------
```

### Integration Jenkins CI
use [SLOCCount Plugin](https://wiki.jenkins-ci.org/display/JENKINS/SLOCCount+Plugin).

```
$ cloc --by-file --output-type=sloccount . > sloccount.scc
```

```
$ cat sloccount.scc
398 Go      ./main.go
190 Go      ./language.go
132 Markdown        ./README.md
24  Go      ./xml.go
18  Go      ./file.go
15  Go      ./option.go
```

## Support Languages
use `--show-lang` option

```
$ gocloc --show-lang
```

## Performance

* CPU Apple M3 8-core / 16 GiB / macOS 26.6.1 arm64 / Go 1.26.6
* cloc 2.04
* tokei 14.0.0 compiled with serialization support: json
* upstream gocloc [679b457](https://github.com/hhatto/gocloc/commit/679b457182dcf852d90e52f132403d6b7de0e33d)
* optimized gocloc, based on upstream [679b457](https://github.com/hhatto/gocloc/commit/679b457182dcf852d90e52f132403d6b7de0e33d)
* target repository is [golang/go](https://github.com/golang/go), branch `master`, commit [be1160f](https://github.com/golang/go/commit/be1160f2a446665d6c0ccd2344c0cbe365bbc3a4)

All tools scanned the Go repository revision above with `dist`, `node_modules`, and `target` excluded. The command output is from a representative warm-cache run. The `time` lines are warm-cache averages: 10 runs for tokei and both gocloc versions, and 3 runs for cloc. gocloc uses 8 workers.

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
tokei . -e '{dist,node_modules,target}/' -s lines -C  0.593s user 0.738s system 628.1% cpu 0.212 total
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
$ time gocloc --not-match-d='dist|node_modules|target' --workers=8 .
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
gocloc --not-match-d='dist|node_modules|target' --workers=8 .  0.744s user 0.823s system 603.4% cpu 0.260 total
```

## License
MIT
