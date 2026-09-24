# gocloc

[English](README.md) | [简体中文](README.zh-CN.md)

[![GoDoc](https://godoc.org/github.com/rustyllh/gocloc?status.svg)](https://godoc.org/github.com/rustyllh/gocloc)
[![ci](https://github.com/rustyllh/gocloc/workflows/Go/badge.svg)](https://github.com/rustyllh/gocloc/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/rustyllh/gocloc)](https://goreportcard.com/report/github.com/rustyllh/gocloc)
[![Docker Pulls](https://img.shields.io/docker/pulls/rustyllh/gocloc)](https://hub.docker.com/r/rustyllh/gocloc)
[![Docker Image Size](https://img.shields.io/docker/image-size/rustyllh/gocloc/latest)](https://hub.docker.com/r/rustyllh/gocloc)

一个用 Go 编写的快速、并行源码行数统计工具，基于 [hhatto/gocloc](https://github.com/hhatto/gocloc)。
灵感来自 [tokei](https://github.com/Aaronepower/tokei)，针对文件扫描和行数统计进行了性能优化。

## 安装

### macOS / Linux

安装最新稳定版，无需 Go 环境，自动校验 SHA-256：

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://raw.githubusercontent.com/rustyllh/gocloc/master/install.sh | sh
```

需要 `curl`、`tar` 以及 `sha256sum` 或 `shasum`。默认安装到 `$HOME/.local/bin`，请按需将其加入 `PATH`。

### Windows

在 PowerShell 5.1+ 中执行：

```powershell
& ([scriptblock]::Create((Invoke-RestMethod https://raw.githubusercontent.com/rustyllh/gocloc/master/install.ps1 -ErrorAction Stop)))
```

默认安装到 `%LOCALAPPDATA%\gocloc\bin`，请将其加入用户 `Path`。支持 x86-64 和 i386，不支持 Windows ARM64。

这两种方式会执行下载的脚本，可先查看 [install.sh](install.sh) 或 [install.ps1](install.ps1)。
再次运行安装命令即可升级，使用 `gocloc --version` 确认版本。
也可以从 [GitHub Releases](https://github.com/rustyllh/gocloc/releases) 手动下载二进制文件及校验文件。

### Docker

以只读方式挂载并统计当前目录：

```sh
docker run --rm --read-only --mount "type=bind,source=$(pwd),target=/workdir,readonly" rustyllh/gocloc:latest .
```

支持 Linux `amd64` 和 `arm64`，自动选择匹配的架构。`:vX.Y.Z` 用于固定版本，`:latest` 跟随稳定版更新。
命令行选项放在镜像名称之后，例如 `--dedup -o json .`。

## 性能

* CPU Apple M3 8 核 / 16 GiB / macOS 27.0 arm64 / Go 1.26.6
* cloc 2.04
* tokei 14.0.0，编译时启用了 JSON 序列化支持
* 上游 gocloc：[679b457](https://github.com/hhatto/gocloc/commit/679b457182dcf852d90e52f132403d6b7de0e33d)
* 优化版 gocloc：`0.1.9 (488b23c)`
* 测试目标：[golang/go](https://github.com/golang/go) 仓库，`master` 分支，提交 [6b3800e](https://github.com/golang/go/commit/6b3800e1dd90925b4184acd6391f68bffd16b6a5)

所有工具均使用默认选项扫描上述仓库。命令输出取自一次有代表性的热缓存运行；`time` 行显示热缓存平均值：tokei 和上游 gocloc 各运行 10 次，优化版 gocloc 运行 30 次，cloc 运行 3 次。

### cloc

```
$ time cloc .
3 errors:
Line count, exceeded timeout:  ./src/net/http/requestwrite_test.go
Line count, exceeded timeout:  ./src/vendor/golang.org/x/net/idna/tables15.0.0.go
Line count, exceeded timeout:  ./src/vendor/golang.org/x/net/idna/tables17.0.0.go
github.com/AlDanial/cloc v 2.04  T=26.55 s (530.8 files/s, 142732.0 lines/s)
-----------------------------------------------------------------------------------
Language                         files          blank        comment           code
-----------------------------------------------------------------------------------
Go                               11519         274964         460748        2523775
Text                              1473          14804              0         233134
Assembly                           656          16184          24182         149525
HTML                                15           2098             50          20117
Snakemake                           28           2202              0          19021
JSON                                40            124              0          14186
YAML                                68            361            364           6564
C                                  113            968            845           5547
Markdown                            64           1443             36           4840
CSV                                  1              0              0           2118
Bourne Shell                        19            273            914           1761
JavaScript                           9            301            331           1706
Perl                                 9            163            163           1058
C/C++ Header                        27            153            368            798
Bourne Again Shell                  16            112            248            516
Python                               2            155            187            425
CSS                                  4              5             13            360
Windows Resource File                4             23              0            146
DOS Batch                            5             35             57            120
Logos                                2             16              0            101
Dockerfile                           2             15             18             61
diff                                 1              7             20             36
C++                                  2             11             14             24
make                                 6              9             34             22
Objective-C                          1              2              3             11
Fortran 90                           2              1              3              8
awk                                  1              1              6              7
MATLAB                               1              1              0              4
-----------------------------------------------------------------------------------
SUM:                             14090         314431         488604        2985991
-----------------------------------------------------------------------------------
cloc .  23.151s user 1.617s system 94.3% cpu 26.104 total
```

### tokei

```
$ time tokei .
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Language              Files        Lines         Code     Comments       Blanks
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 AWK                       1           14            7            6            1
 Alex                      2          117          101            0           16
 GNU Style Assembly      659       189929       154061        19670        16198
 Autoconf                  9          283          274            0            9
 BASH                     16          876          502          264          110
 Batch                     5          212          120           57           35
 C                       117         7431         5657          856          918
 C Header                 28         1326          801          371          154
 C++                       2           49           24           14           11
 CSS                       4          378          360           13            5
 Dockerfile                2           94           61           18           15
 Forge Config              3            3            3            0            0
 FORTRAN Modern            2           12            8            3            1
 Go                    11728      3302392      2575946       455985       270461
 JavaScript                9         2338         1705          332          301
 JSON                     38        14287        14163            0          124
 Makefile                  6           65           22           34            9
 Objective-C               2           21           15            3            3
 Perl                      9         1384         1049          172          163
 Python                    2          767          482          142          143
 Shell                    19         2948         2371          380          197
 Snakemake                28        21223        19878            0         1345
 Templ                     8           20           20            0            0
 Plain Text             1478       248043            0       233221        14822
 YAML                     59         6783         6136          331          316
─────────────────────────────────────────────────────────────────────────────────
 HTML                     15        12849        12465           37          347
 |- CSS                    3         2060         1841           10          209
 |- HTML                   1          219          212            0            7
 |- JavaScript             8         7143         7026           91           26
 (Total)                            22271        21544          138          589
─────────────────────────────────────────────────────────────────────────────────
 Markdown                 68         5751            0         4311         1440
 |- Go                     2          562          559            3            0
 (Total)                             6313          559         4314         1440
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Total                 14319      3829579      2805869       716324       307386
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
tokei .  0.569s user 0.758s system 552.5% cpu 0.261 total
```

### 上游 gocloc（https://github.com/hhatto/gocloc）

```
$ time gocloc-upstream .
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Go                           11534         275074         485096        2500003
Plain Text                    1468          14795              0         233087
Assembly                       656          16198          24181         149512
HTML                            15           2098            180          19987
JSON                            40            124              0          14186
YAML                            59            326            361           6096
C                              113            968            846           5546
Markdown                        62           1436             36           4830
BASH                            31            362           1144           2228
JavaScript                       9            301            332           1705
C Header                        27            153            368            798
Perl                             9            163            581            640
Python                           2            155            187            425
CSS                              4              5             13            360
Batch                            5             35              0            177
Plan9 Shell                      4             23             47             99
Dockerfile                       2             15             18             61
Bourne Shell                     4             23             18             49
C++                              2             11             14             24
Makefile                         6              9             34             22
Objective-C                      2              3              3             15
FORTRAN Modern                   2              1              3              8
Awk                              1              1              6              7
-------------------------------------------------------------------------------
TOTAL                        14057         312279         513468        2939865
-------------------------------------------------------------------------------
gocloc-upstream .  0.609s user 0.947s system 105.3% cpu 1.476 total
```

### 优化版 gocloc

```
$ time gocloc .
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Go                           11746         277212         489930        2536004
Plain Text                    1476          14813              0         233179
Assembly                       659          16207          24198         149524
HTML                            15           2098            180          19987
JSON                            41            124              0          14186
YAML                            59            326            361           6096
C                              117            976            855           5600
Markdown                        68           1440             36           4837
BASH                            31            362           1144           2228
JavaScript                       9            301            332           1705
C Header                        28            154            371            801
Perl                             9            163            581            640
Python                           2            155            187            425
CSS                              4              5             13            360
Batch                            5             35              0            177
Plan9 Shell                      4             23             47             99
Dockerfile                       2             15             18             61
Bourne Shell                     4             23             18             49
C++                              2             11             14             24
Makefile                         6              9             34             22
Objective-C                      2              3              3             15
FORTRAN Modern                   2              1              3              8
Awk                              1              1              6              7
-------------------------------------------------------------------------------
TOTAL                        14292         314457         518331        2976034
-------------------------------------------------------------------------------
gocloc .  0.308s user 0.895s system 550.3% cpu 0.222 total
```

## 使用

```sh
# 统计一个或多个目录
gocloc .
gocloc src tests

# 排除依赖和构建目录
gocloc --not-match-d='(^|[/\\])(dist|node_modules|target)([/\\]|$)' .

# 筛选语言或排除扩展名
gocloc -l Go,Python .
gocloc -e txt,md .

# 按文件展示，并按代码行数排序
gocloc -f -s code .

# 将 JSON 导出到扫描目录之外
gocloc -o json . > ../counts.json

# 内容相同的文件只统计一次（默认关闭）
gocloc --dedup .

# 列出支持的语言 / 查看全部选项
gocloc -L
gocloc -h
```

默认自动选择 worker 数量，也可使用 `-w 1` 至 `-w 64` 手动指定。
`-o` 支持 `default`、`json`、`markdown`、`cloc-xml` 和 `sloccount`。
长选项与短选项可以混用，完整列表见 `gocloc -h`。

`--not-match-d` 使用正则表达式匹配目录路径，不是 glob 通配符。
生成的报告应放在扫描目录之外，或显式排除。`>` 会覆盖目标文件。
旧版本默认开启去重；如需保留旧版统计行为，请添加 `--dedup`。

`--debug` 将实际分析过程写入 stderr，不会关闭 worker 并发。
逐行日志包含文件路径和行号，不同文件的日志允许交错。
开启 `--dedup` 时，重复文件可能先输出分析日志，再通过 `[SKIP]` 日志标记为不计入统计。

### 库回调

`Processor.Analyze` 在 worker 中执行 `OnCode`、`OnComment` 和 `OnBlank`。
使用多个 worker 时，不同文件的回调可能并发执行，调用方需要保护共享状态。
例如，将以下配置传给 `NewProcessor`（计数器使用 `sync/atomic`）：

```go
options := gocloc.NewClocOptions()
options.Workers = 8
options.SkipDuplicated = false // 可选：内容相同的文件只统计一次。
var codeLines atomic.Int64
options.OnCode = func(string) { codeLines.Add(1) }
```

同一文件内按行顺序调用，文件之间不保证调用顺序。`Workers` 同时限制文件分析和回调执行的并发度。
设置 `Workers = 1` 可保证单次 `Analyze` 内回调不重叠，但不保证运行在调用方的 goroutine 上。
回调必须返回，`Analyze` 才能结束；即使发生错误，`Analyze` 也会等待全部回调执行完毕再返回。
`AnalyzeFile` 和 `AnalyzeReader` 仍然同步调用回调。
这改变了旧版库在调用方 goroutine 中执行处理器回调的约定，已有调用方需要保护共享状态或显式使用单 worker。

回调遵循去重设置。不去重时，每份文件都会在分析过程中触发回调，后续读取失败不能撤销已经执行的调用。
开启去重时，worker 单次读取源文件，同时计算哈希、统计行数并记录回调事件；按发现顺序去重后，
只有首次保留副本的事件会交给回调 worker 执行。源文件读取失败或被去重排除时，不触发回调。
debug 日志仍然反映实际分析过程，包括被排除的副本。

每次扫描的延迟事件缓存共享 16 MiB 内存缓冲容量预算（不是进程总内存上限）。
超过额度的缓存写入系统临时目录的私有文件，回放或排除后删除；排队中的临时文件不占用打开的文件描述符。
解析缓冲、单条回调字符串以及调用方保留的内存不计入该预算。
临时缓存写入失败时，给出诊断并排除相应文件；回放或清理失败时，等待 worker 完成后返回错误。
已经执行的回调无法回滚。

## Jenkins

在 Jenkins 中安装 [SLOCCount 插件](https://plugins.jenkins.io/sloccount/)，并确保执行节点的 `PATH` 中有 `gocloc`。
在已检出源码、使用 Unix 节点的声明式 Pipeline 中添加以下阶段：

```groovy
stage('Count lines') {
    steps {
        sh 'mkdir -p reports && gocloc -f -o sloccount --not-match-d=reports . > reports/sloccount.sc'
        sloccountPublish pattern: 'reports/sloccount.sc', encoding: 'UTF-8'
    }
}
```

报告目录已从统计中排除。对于 Freestyle 任务，执行同一条 shell 命令，再添加
**Publish SLOCCount analysis results** 构建后操作，将文件匹配模式设为 `reports/sloccount.sc`。

## 许可证

MIT
