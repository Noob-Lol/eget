# 并发下载设计

## 目标

在不改变现有安装主链路形状的前提下，增加两个语义明确的并发控制：

```text
source target -> candidate asset URLs -> select one asset -> download -> verify -> extract -> record
```

本设计区分两类不同的并发：

- `chunk_concurrency`：单个已选 asset 下载时的 HTTP Range 分片并发数。
- `batch_concurrency`：`install --all` 和 `update --all` 的包任务并发数。

面向用户的 CLI 使用更短、更方便的参数：

```bash
eget install --chunk 8 owner/repo
eget download --chunk 8 owner/repo
eget update --chunk 8 fd
eget install --all --batch 3
eget update --all --chunk 4 --batch 3
```

## 非目标

第一版不实现：

- 部分下载文件的断点续传。
- 持久化 partial chunk 文件。
- 超出现有请求行为之外的 chunk 自动重试策略。
- `list --outdated` 中 provider 元数据检查的并发化。
- package 级别的 `batch_concurrency`。
- GUI installer 的并发交互。
- 新的多 asset 下载模式。单次 `install` / `download` 仍然先选出一个最终 asset URL，再下载。

## 配置

全局默认配置：

```toml
[global]
chunk_concurrency = 0
batch_concurrency = 0
```

package 和 repo 配置只允许覆盖 `chunk_concurrency`：

```toml
[repos."BurntSushi/ripgrep"]
chunk_concurrency = 8

[packages.winmerge]
repo = "sourceforge:winmerge"
asset_filters = ["x64", "setup"]
chunk_concurrency = 3
```

`batch_concurrency` 只属于 global 配置，因为它控制的是整个批处理调度器，不属于某一个包。

## 取值语义

两个配置使用同一套取值模型：

| 值 | `chunk_concurrency` | `batch_concurrency` |
| --- | --- | --- |
| `0` | 自动 | 自动 |
| `1` | 单连接下载，不做分片并发 | 串行处理包 |
| `> 1` | 指定 Range 分片数量 | 指定包任务 worker 数量 |

负数非法。

建议校验范围：

```text
chunk_concurrency: 0..32
batch_concurrency: 0..16
```

超出范围时应返回错误，不要静默截断。

## 自动行为

`chunk_concurrency = 0` 表示自动选择 chunk 数。第一版应保持行为可预测：

```text
if file is large enough and server supports HTTP Range and content length is known:
    effective chunk concurrency = min(5, useful chunk count)
else:
    effective chunk concurrency = 1
```

这样既保留“默认最多 5 个下载分片”的预期，也能在服务端不支持 Range 时安全回退。

小文件不应启用分片并发。建议第一版设置最小分片阈值：

```text
min_chunk_size = 4 MiB
```

只有当文件大小至少能切出两个不小于 `min_chunk_size` 的分片时，才启用 Range 并发。例如：

- 文件小于 `8 MiB`：实际使用 `1`，即单连接下载。
- 文件为 `20 MiB` 且 auto chunk：最多切成 `5` 个约 `4 MiB` 的分片。
- 用户指定 `--chunk 16` 但文件只有 `20 MiB`：实际 chunk 数应降为 `5`，避免产生过小分片。

也就是说，`--chunk N` 表示用户允许的最大分片并发数，不表示必须强行切出 `N` 个分片。

`batch_concurrency = 0` 表示自动选择批处理并发。当前实际使用：

```text
effective batch concurrency = min(total packages, 6)
```

`batch_concurrency = 1` 保持串行；用户可以通过 `--batch N` 或 `global.batch_concurrency = N` 显式设置 worker 数。

## 优先级

`chunk_concurrency` 遵循普通安装选项优先级：

```text
CLI --chunk
> packages.<name>.chunk_concurrency
> repos.<repo>.chunk_concurrency
> global.chunk_concurrency
> 0
```

`batch_concurrency` 属于调度器级别：

```text
CLI --batch
> global.batch_concurrency
> 0
```

`--batch` 只允许用于 `install --all` 和 `update --all`。如果单目标命令传入 `--batch`，应返回明确错误：

```text
--batch can only be used with --all
```

## CLI 表面

`install` 支持：

```bash
eget install --chunk N target
eget install --all --batch N
eget install --all --chunk N --batch N
```

`download` 支持：

```bash
eget download --chunk N target
```

`download` 当前没有 `--all` 模式，因此不支持 `--batch`。

`update` 支持：

```bash
eget update --chunk N target-or-name
eget update --all --batch N
eget update --all --chunk N --batch N
```

`add` 可以支持 `--chunk N`，用于持久化 package 级别的 `chunk_concurrency`：

```bash
eget add --name winmerge --chunk 3 sourceforge:winmerge
```

写入：

```toml
[packages.winmerge]
repo = "sourceforge:winmerge"
chunk_concurrency = 3
```

`add` 不支持 `--batch`。

## 内部模型

内部字段使用完整名称：

```go
type Options struct {
    ChunkConcurrency int
    BatchConcurrency int
}
```

配置结构使用 `*int`，这样 merge 时能区分未配置和显式配置为 `0`：

```go
type Section struct {
    ChunkConcurrency *int `toml:"chunk_concurrency" mapstructure:"chunk_concurrency"`
    BatchConcurrency *int `toml:"batch_concurrency" mapstructure:"batch_concurrency"`
}

type CLIOverrides struct {
    ChunkConcurrency *int
}

type Merged struct {
    ChunkConcurrency int
}
```

`BatchConcurrency` 可以放在 `Section` 中用于解码，但批处理调度器只读取 `global.batch_concurrency` 和 CLI `--batch`。

## HTTP Range 分片下载

现有单 asset 下载路径保持入口不变：

```text
InstallRunner.downloadBody
-> install.Download
-> client.Download
```

`client.Download` 在解析出有效 `chunk_concurrency` 后，决定使用单连接下载还是 Range 分片下载。

只有满足以下条件时才尝试分片下载：

- 有效 chunk 并发数大于 `1`。
- URL 不是本地文件。
- 响应具有已知且大于 0 的 content length。
- 服务端能证明支持 Range，优先通过小请求 `Range: bytes=0-0` 返回 `206 Partial Content` 判断。

如果探测阶段不能证明 Range 可用，则回退到现有单连接下载。

一旦已经进入分片下载，某个 chunk 失败时应让整个下载失败，不要静默重新下载整个文件。这样可以避免隐藏部分失败，也避免重复下载大文件。

## 进度输出

单连接下载保持现有进度条行为。

分片下载展示一个聚合的整文件进度条，不展示 chunk 级别子进度。chunk worker 将已读字节写入同一个 `progress.NewConcurrentWriterWithInterval()`，由 `cliui/progress` 的 `ByteTracker` 聚合后定时 flush 到当前 package 的 progress bar。

推荐进度能力：

- 单包下载使用一个 `progress.Progress`。
- batch 下载使用 `progress.MultiProgress`。
- `MultiProgress.AutoRefresh = true`。
- `MultiProgress.RefreshInterval = 100 * time.Millisecond`。
- chunk worker 共享 `progress.NewConcurrentWriterWithInterval(bar, 100*time.Millisecond)`。
- 所有 chunk worker 结束后必须关闭 writer，确保 pending bytes flush 到 progress。
- 交互式终端使用 `RenderDynamic`。
- 非 TTY 输出使用 `RenderPlain` 或 `RenderDisabled`。
- 普通日志、warning、fallback notice 通过 `MultiProgress.RunExclusive()` / `Printf()` / `Println()` 输出，避免破坏多行进度块。

设置 `--quiet` 时，进度条和回退提示都保持静默。

非 quiet 的回退提示示例：

```text
HTTP Range download unsupported, falling back to single-connection download
```

当 batch 和 chunk 并发同时启用时，非 quiet 模式可以打印简短摘要：

```text
Using batch concurrency 3 and chunk concurrency 4; up to 12 download requests may run concurrently.
```

`batch_concurrency > 1` 时使用固定 worker slot，而不是为所有 package 一次性创建进度条。每个 slot 绑定一个 `progress.Progress`，完成一个 package 后通过 `ResetWith()` 复用给下一个 package：

```text
#1 fd       [===================>      ] 73.2% 8.1MiB/11.0MiB downloading chunks:5
#2 ripgrep  [=========>                ] 34.0% 4.4MiB/13.0MiB downloading chunks:4
#3 jq       resolving release...
```

示例用法：

```go
mp := progress.NewMulti()
mp.Writer = stderr
mp.RenderMode = progress.RenderDynamic
mp.AutoRefresh = true
mp.RefreshInterval = 100 * time.Millisecond
mp.Start()
defer mp.Finish()

bar := mp.New()
bar.SetFormat("{@slot} {@name:-12s} [{@bar}] {@percent:5s}% {@curSize}/{@maxSize} {@phase} {@extra}")
bar.ResetWith(func(p *progress.Progress) {
	p.MaxSteps = size
	p.SetMessages(map[string]string{
		"slot":  "#1",
		"name":  "fd",
		"phase": "downloading",
		"extra": "chunks:5",
	})
})

writer := progress.NewConcurrentWriterWithInterval(bar, 100*time.Millisecond)
defer writer.Close()
```

## 批处理并发

`install --all` 当前按包名排序后串行安装。批处理并发应在允许并行执行的同时保持稳定结果顺序。

推荐行为：

- 调度前先按包名排序。
- 最多运行有效 batch 并发数个 worker。
- 创建固定数量的 progress worker slot，并复用 slot 显示当前 package。
- 每个 package 的结果写回排序后的固定下标。
- 返回结果仍按包名排序。
- 首个错误出现后停止调度新任务，并返回该错误。

installed store 写入必须安全。如果 store 本身不是并发安全的，第一版可以在安装完成写入阶段串行化。

对于 `update --all`：

- 第一版保持 `ListUpdateCandidates` 串行。
- 只在 `UpdateCandidates` 阶段应用 batch 并发，即真正执行安装更新的阶段。
- 返回的 update results 仍按包名排序。

## GUI Installer

GUI installer 不适合并发运行，因为它需要用户交互并启动外部进程。批处理调度器应选择以下策略之一：

- 对 GUI installer package 串行执行。
- 或在执行前已知 package 是 GUI 时，拒绝 `--batch > 1`。

保守的第一版做法是：如果解析后的 package options 已标记为 GUI，则串行执行该 package。如果 GUI 状态只能在 asset 选择后才知道，则文档中提示用户避免对 GUI-heavy package set 使用 `--batch > 1`。

## 文档

实现时需要更新：

- `README.md`
- `README.zh-CN.md`
- `docs/DOCS.md`
- `docs/example.eget.toml`
- `docs/TODO.md`

核心用户说明：

```text
--chunk N controls HTTP Range chunk concurrency for one downloaded file.
0 means auto, 1 means single-connection, and values greater than 1 request that many chunks.

--batch N controls package task concurrency for install --all and update --all.
0 means auto, 1 means serial, and values greater than 1 process up to N packages at once.
```

## 测试

需要补充聚焦测试：

- `install --chunk` 的 CLI flag 绑定。
- `download --chunk` 的 CLI flag 绑定。
- `update --chunk` 的 CLI flag 绑定。
- `install --all --batch` 的 CLI flag 绑定。
- `update --all --batch` 的 CLI flag 绑定。
- 无 `--all` 时传入 `--batch` 应报错。
- `chunk_concurrency` 和 `batch_concurrency` 的配置解码。
- CLI/package/repo/global `chunk_concurrency` 的 merge 优先级。
- `batch_concurrency` 只读取 global 和 CLI。
- 负数和超出范围的校验。
- Range 支持时，chunk auto 最多解析为 5 个分片。
- 小文件即使 auto 或 `--chunk > 1` 也保持单连接下载。
- 用户指定的 chunk 数会根据 `min_chunk_size` 下调，避免过小分片。
- `chunk_concurrency = 1` 使用现有单连接路径。
- 服务端不返回 `206` 时 Range 探测回退。
- 多 chunk 响应能正确组装最终 body。
- chunk 失败返回明确错误。
- 缓存命中时无论 chunk 设置如何都不发起网络请求。
- `install --all` 在 `--batch > 1` 时保持结果顺序。
- `update --all` 在 `--batch > 1` 时保持结果顺序。

实现完成前运行：

```bash
go test ./...
```
