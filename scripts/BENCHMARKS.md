# 可重复的性能基准

在仓库根目录运行：

```sh
bash scripts/benchmark.sh
```

脚本先验证固定夹具的统计结果、内容指纹与去重行为，再测量耗时、吞吐量、
分配字节数和分配次数。报告保存在新建的临时目录，包含环境、原始样本和
benchstat 汇总；GitHub 的 Benchmarks 工作流仍由手动触发。

现有 `BenchmarkProcessorFixtures` 覆盖大量小文件、少量大文件、重复文件、
大量注释和超长行。新增 `BenchmarkProcessorLanguages` 分别覆盖 Go、
JavaScript、C++、Java、Rust、Python、C#、SQL、Lua、Swift、BASH、XML
以及混合语言目录，包含原始字符串、文本块、嵌套注释和文档字符串。
多语言夹具版本为 `v2`；与 `v1` 比较时注意语言集合及内容指纹的变化。
新增夹具使用固定文件名和内容，
每种语言的文件成对重复，验证首次发现的副本保留。

多语言测量使用 1、8 个 worker，并分别启用、关闭去重。计时期间不注册回调；
配套正确性测试额外验证并发回调的总数与去重结果一致。新增计数规则的逐行
回归用例、分片读取与异常源码用例位于 `internal/core/syntax_languages_test.go`。

只测多语言夹具：

```sh
BENCH_FILTER='^BenchmarkProcessorLanguages$' bash scripts/benchmark.sh
```

延长单组测量并指定新报告目录：

```sh
BENCHTIME=1s BENCH_COUNT=10 bash scripts/benchmark.sh /tmp/gocloc-language-baseline
```

与另一份报告比较（使用与脚本相同的固定 benchstat 版本）：

```sh
go run golang.org/x/perf/cmd/benchstat@v0.0.0-20260908200009-22c9c6c9d4da \
  /tmp/gocloc-language-baseline/raw.txt /tmp/gocloc-language-current/raw.txt
```

比较时保持硬件、Go 版本和运行时配置一致，逐次运行测试，保留所有样本。
共享 GitHub runner 的结果仅作报告，不作为发布性能门槛。正确性以显式的
逐行期望为准，其他工具的输出只适合作为差异排查线索。
