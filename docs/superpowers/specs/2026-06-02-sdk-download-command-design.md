# SDK download/dl 命令设计

## 背景

当前 `eget sdk install` 已经具备 SDK 归档下载能力，流程是：

```text
parse target
-> resolve sdk config
-> resolve version/file
-> download archive to {cache_dir}/sdk-downloads
-> extract
-> record sdk.installed.json
```

新增 `sdk download/dl` 的目标是复用前半段能力，只下载 SDK 归档到本地，不解压、不安装、不写安装记录。

## 目标

- 增加 `eget sdk download <target...>` 命令。
- 增加别名 `eget sdk dl <target...>`。
- 默认下载当前平台对应的 SDK 归档。
- 支持通过 `--os` 和 `--arch` 下载非当前平台归档。
- 默认下载到现有 SDK 下载缓存目录：`{cache_dir}/sdk-downloads/{sdk}/{version}/{filename}`。
- 支持通过 `--output/-o` 将最终归档放到指定目录。
- 复用现有 `DownloadArchive` 的缓存命中、断点续传、并发下载、代理、SSL 配置和 cache mirror 能力。
- 支持一次下载多个 target。

## 非目标

- 不解压 SDK 归档。
- 不写 `sdk.installed.json`。
- 不修改 `PATH`、环境变量、shell profile 或 xenv 状态。
- 不新增 `--ext`。MVP 仍通过 `ext_map` 决定平台扩展名。
- 不支持 `--output` 指定单个目标文件名。MVP 只把 `--output` 当作目录。
- 不改变 `sdk install`、`sdk search`、`sdk index` 的现有语义。

## 命令设计

基础用法：

```bash
eget sdk download go:1.22
eget sdk dl go:1.22
eget sdk dl go:1.22 node:20
```

下载非当前平台：

```bash
eget sdk dl --os linux --arch arm64 go:1.22
eget sdk dl --os windows --arch amd64 go:1.22
eget sdk dl --os darwin --arch arm64 node:20
```

下载并输出到指定目录：

```bash
eget sdk dl -o ./downloads go:1.22
eget sdk dl --output ./downloads --os linux --arch arm64 go:1.22
```

参数：

| 参数 | 说明 |
| --- | --- |
| `target...` | SDK target，沿用 `go`、`go@1.22.0`、`go:1.22`、`node:20` 格式 |
| `--os` | 目标操作系统，默认当前 `runtime.GOOS` |
| `--arch` | 目标架构，默认当前 `runtime.GOARCH` |
| `-o, --output` | 指定最终归档输出目录 |

`--os` 和 `--arch` 要么都不传，要么都传。只传一个会返回错误，避免隐式组合出用户没有明确选择的平台。

## 平台覆盖语义

`--os` 和 `--arch` 使用 Go 原始平台名，例如：

```text
windows/amd64
linux/arm64
darwin/arm64
```

命令层不要求用户输入镜像站的发布命名。SDK 配置仍负责映射：

```toml
os_map = { windows = "win", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64" }
ext_map = { windows = "zip", linux = "tar.xz", darwin = "tar.gz" }
```

这样 `eget sdk dl --os windows --arch amd64 node:20` 会先按 `windows/amd64` 解析配置，再映射成 Node 发布文件需要的 `win/x64/zip`。

## 输出路径语义

默认输出路径复用现有 SDK 下载缓存：

```text
{cache_dir}/sdk-downloads/{sdk}/{version}/{filename}
```

示例：

```text
{cache_dir}/sdk-downloads/go/1.22.12/go1.22.12.windows-amd64.zip
```

指定 `--output` 时：

1. 仍先通过 `DownloadArchive` 下载或复用缓存文件。
2. 确保 `--output` 目录存在；不存在则创建。
3. 将最终归档复制到 `--output/{filename}`。
4. 命令结果中的 `Path` 使用复制后的路径。

如果 `--output` 已存在但不是目录，返回错误。

## CLI 输出

下载开始：

```text
 - Download SDK go:1.22 -> 1.22.12 from go.dev
```

下载成功：

```text
✓ Downloaded go@1.22.12 windows/amd64 -> cache/sdk-downloads/go/1.22.12/go1.22.12.windows-amd64.zip
```

缓存命中：

```text
✓ Downloaded go@1.22.12 windows/amd64 -> cache/sdk-downloads/go/1.22.12/go1.22.12.windows-amd64.zip (cached)
```

断点续传：

```text
✓ Downloaded go@1.22.12 linux/arm64 -> cache/sdk-downloads/go/1.22.12/go1.22.12.linux-arm64.tar.gz (resumed)
```

## 服务层设计

现有 `Service.Install()` 中已经包含下载归档解析逻辑。实现 `Download()` 时不应复制这段逻辑，而应抽取共享 helper：

```text
resolveDownloadArchive(rawTarget, platformOverride)
```

这个 helper 负责：

1. `ParseTarget(rawTarget)`。
2. 按平台覆盖解析 SDK 配置。
3. 解析版本和平台文件。
4. 渲染 `url_template` 或使用 index file URL。
5. 生成最终下载请求需要的 SDK 名、版本、URL、文件名、OS、Arch、Ext。

然后两个主流程分别复用它：

```text
Install()
-> resolveDownloadArchive()
-> DownloadArchive()
-> extract
-> record store

Download()
-> resolveDownloadArchive()
-> DownloadArchive()
-> optional copy to output dir
-> return result
```

## 数据结构建议

在 `internal/sdk` 增加下载命令专用 options/result，命名避免和已有底层 `DownloadResult` 混淆：

```go
type PlatformOptions struct {
	OS   string
	Arch string
}

type SDKDownloadOptions struct {
	Platform  PlatformOptions
	OutputDir string
	Progress  func(size int64) io.Writer
	OnStart   func(target string, version string, host string)
}

type SDKDownloadResult struct {
	Name     string
	Version  string
	Path     string
	URL      string
	Filename string
	OS       string
	Arch     string
	Ext      string
	Cached   bool
	Resumed  bool
}
```

在 `Service` 增加：

```go
Download(ctx context.Context, rawTarget string, opts SDKDownloadOptions) (SDKDownloadResult, error)
DownloadMany(ctx context.Context, targets []string, opts SDKDownloadOptions) ([]SDKDownloadResult, error)
```

CLI 侧 `sdkCLIService` 增加 `DownloadMany`。

## 错误处理

需要明确返回的错误：

```text
sdk download target is required
sdk download --os and --arch must be used together
sdk file for linux/arm64.tar.gz not found
sdk "go" extension for freebsd is not configured
sdk download output path is not a directory
```

版本、index、URL 模板和平台文件缺失的错误继续复用现有 SDK service 的错误风格。

## 文件影响范围

预计新增或修改：

```text
internal/sdk/service.go
internal/sdk/install_service.go
internal/sdk/download_service.go
internal/cli/sdk_cmd.go
internal/cli/sdk_handler.go
internal/cli/service.go
internal/cli/handlers.go
internal/cli/app_sdk_test.go
internal/cli/sdk_handler_test.go
internal/sdk/download_service_test.go
docs/sdk-usage.md
```

实现会超过 3 个逻辑文件，也可能超过 100 行代码，因此实施前需要再次确认。

## 测试策略

CLI 参数和路由：

- `sdk download go:1.22` 路由到 `sdk.download`。
- `sdk dl go:1.22` 别名可用。
- `--os`、`--arch`、`-o/--output` 绑定正确。
- 缺少 target 时返回错误。
- 只传 `--os` 或只传 `--arch` 时返回错误。

CLI handler：

- `handleSDKDownload` 调用 `DownloadMany`。
- 传递 target、platform、output 和 progress。
- 正确输出 path、cached、resumed 信息。

SDK service：

- `Download` exact 版本使用 `url_template`。
- `Download` latest/prefix 版本使用 index。
- 默认使用当前平台。
- 传入 `PlatformOptions` 时选择非当前平台文件。
- 不解压归档。
- 不写 `sdk.installed.json`。
- `DownloadMany` 支持多个 target。
- `OutputDir` 会复制归档到指定目录。

完成 MVP 主链路后运行：

```bash
go test ./...
```

## 推荐实现顺序

1. 抽取 SDK 归档解析 helper，并让现有 `Install()` 继续通过测试。
2. 增加 `Service.Download()` 和 `DownloadMany()`。
3. 增加 CLI options、子命令、别名和 handler 路由。
4. 补充 CLI 与 service 单元测试。
5. 更新 `docs/sdk-usage.md`。
6. 运行 `go test ./...`。

## 设计结论

采用方案：

```bash
eget sdk download|dl [--os OS --arch ARCH] [-o DIR] <target...>
```

核心语义：

- 默认当前平台。
- `--os/--arch` 显式选择非当前平台。
- 默认写入 `{cache_dir}/sdk-downloads`。
- `-o/--output` 将最终归档复制到指定目录。
- 不解压。
- 不写 `sdk.installed.json`。
- 复用现有 SDK 下载缓存、断点续传、cache mirror 和并发下载能力。
