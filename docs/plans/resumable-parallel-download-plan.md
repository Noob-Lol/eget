# Resumable Parallel Download Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use test-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `eget install`、`eget download` 和 SDK 下载统一支持“断点续传 + HTTP Range 并发分片”，大文件下载失败后只补未完成分片，并保留现有 cache、进度条、校验和安装流程。

**Architecture:** 在 `internal/client` 建立统一的文件下载器，使用固定 offset 写入同一个 `.part` 文件，并通过 `.meta.json` 记录远端指纹和每个分片完成状态。`internal/install` 继续通过 `DownloadFile()` 写 cache 文件；`internal/sdk` 改为复用 client 下载器，只保留 SDK 自己的路径规划、完整 cache 命中判断和 `DownloadResult` 返回语义。

**Tech Stack:** Go 标准库 `net/http`、`os.File.WriteAt`、`sync`、`encoding/json`、`context`；现有 `client.Options.ChunkConcurrency`、`httptest`、`github.com/gookit/goutil/testutil/assert`。

---

## 结论：SDK 下载场景可以复用

可以，而且应该复用。

当前有两套类似能力：

- `internal/client.DownloadFile()`：给 install/download cache 使用，支持单连接 `.part` 断点续传。
- `internal/sdk.DownloadArchive()`：SDK 专用，自己实现了 HEAD 探测、`.part`、meta、单连接续传。

继续分别增强会导致两套并发分片状态机、两套元数据格式、两套错误处理。更好的方案是：

- `internal/client` 负责所有“远程 URL 下载到文件”的传输细节。
- `internal/install` 和 `internal/sdk` 只负责业务路径、完整 cache 命中、校验/解压/返回值。
- SDK 的 `DownloadArchive()` 调用 client 层下载器后，根据 client 返回的 `FromCache/Resumed/Size/ETag/Modified` 映射到现有 `DownloadResult`。

## 设计边界

- 本计划只处理“下载到文件路径”的场景：`DownloadFile(rawURL, target, progress, opts)`。
- 原有 `Download(rawURL, out, progress, opts)` 内存下载路径暂不接入分片落盘，避免影响已有需要 `io.Writer` 的代码。
- 大文件且支持 `Accept-Ranges: bytes` 时使用并发分片；不支持 Range 或无法确认大小时走单连接文件下载。
- 写入策略使用一个 `.part` 文件和 `WriteAt`，不创建多个 chunk 文件，减少清理成本。
- 元数据采用新 schema，老的单连接 meta 不做兼容迁移。当前项目 v0 阶段，不需要兼容旧格式。

## 关键数据结构

建议将下载文件相关代码从 `internal/client/network.go` 拆到新文件，避免继续扩大 `network.go`。

```go
type DownloadFileResult struct {
	Path         string
	FromCache    bool
	Resumed      bool
	Parallel     bool
	Size         int64
	ETag         string
	LastModified string
}

type downloadFileMeta struct {
	Schema       int                 `json:"schema"`
	URL          string              `json:"url"`
	Size         int64               `json:"size"`
	ETag         string              `json:"etag,omitempty"`
	LastModified string              `json:"last_modified,omitempty"`
	ChunkSize    int64               `json:"chunk_size,omitempty"`
	Chunks       []downloadChunkMeta `json:"chunks,omitempty"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

type downloadChunkMeta struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  bool  `json:"done"`
}
```

## 目标行为

- 第一次下载大文件时：
  - HEAD 获取 `Content-Length`、`Accept-Ranges`、`ETag`、`Last-Modified`。
  - 分片规划写入 meta。
  - 并发请求未完成分片，每个请求使用 `Range: bytes=start-end`。
  - 每个分片成功写完后更新 meta 中对应 `Done=true`。
  - 全部分片完成后，校验 `.part` 大小等于远端大小，重命名为正式文件，保留完整文件 meta。
- 中断后再次下载：
  - 读取 `.part` 和 meta。
  - 远端指纹匹配时，只下载 `Done=false` 的分片。
  - 已完成分片不重复下载。
- 指纹变化：
  - 删除旧 `.part` 和 meta。
  - 重新规划并下载。
- 服务端不支持 Range：
  - 走单连接文件下载。
  - 失败时不保留不可续传 `.part`。
- SDK：
  - 完整 cache 命中仍由 `sdkDownloadFinalPath()` 和 meta 判断。
  - 未命中时调用 client 下载器。
  - SDK 结果中的 `Resumed` 来自 client result。

## 文件结构

- Create: `internal/client/download_file.go`
  - 文件下载主入口、meta 加载保存、分片规划、并发分片调度、单连接 fallback。
- Modify: `internal/client/network.go`
  - 移出或保留少量公共 helper，例如 `requestWithOptions`、`progressFinisher`、`downloadProgressWriter`。
  - 删除旧 `DownloadFile()` 的单连接主体，改为调用新文件实现。
  - 将 `DownloadFile()` 签名从只返回 `error` 改为返回 `(DownloadFileResult, error)`。
- Modify: `internal/client/network_download_test.go`
  - 保留现有 `Download()` 测试。
  - 新增/迁移文件下载测试。
- Modify: `internal/install/network.go`
  - `DownloadFile()` wrapper 返回 `client.DownloadFileResult`，继续转发 test hook、proxy、verbose。
- Modify: `internal/install/runner.go`
  - 接受 `DownloadFile()` 的返回值但不改变现有解压/校验流程。
- Modify: `internal/install/runner_test.go`
  - 调整断点续传测试，增加并发分片行为断言。
- Modify: `internal/sdk/download.go`
  - 删除 SDK 自己的 HEAD、GET、append/write 文件下载实现。
  - 改为调用 `client.DownloadFile()`。
- Modify: `internal/sdk/download_test.go`
  - 保留 SDK cache 命中测试。
  - 将单连接续传预期调整为并发分片续传预期。
- Modify: `docs/plans/resumable-parallel-download-plan.md`
  - 实施时逐步更新 checkbox。

## Task 1: Client 并发分片状态模型

**Files:**
- Create: `internal/client/download_file.go`
- Modify: `internal/client/network.go`
- Test: `internal/client/network_download_test.go`
- Update: `docs/plans/resumable-parallel-download-plan.md`

- [x] **Step 1: 写失败测试：首次大文件使用多个 Range 分片**

测试名称：`TestDownloadFileUsesParallelRangeChunksForLargeFiles`

测试要点：

- 使用 `httptest.NewServer`。
- HEAD 返回 `Accept-Ranges: bytes`、`Content-Length`、`ETag`。
- GET 必须收到多个不同的 `Range`，例如 `bytes=0-...`、`bytes=...-...`。
- 测试体使用至少 `12*1024*1024` 字节，确保 `minChunkSize=4MB` 时可以规划 3 个分片。
- 测试内临时降低 `resumableDownloadMinSize`，避免分配真实 100MB。
- `result, err := DownloadFile(server.URL+"/tool.zip", target, nil, Options{ChunkConcurrency: 3})` 成功。
- 断言目标文件内容等于完整 body。
- 断言至少收到 3 个 Range 请求。
- 断言 `.part` 不存在。
- 断言 `result.Parallel == true`，`result.Resumed == false`。

Run:

```bash
go test ./internal/client -run TestDownloadFileUsesParallelRangeChunksForLargeFiles
```

Expected: FAIL。当前 `DownloadFile()` 是单连接续传，不会并发分片。

- [x] **Step 2: 新增 meta v2 和分片规划 helper**

实现：

- `planDownloadChunks(size int64, chunks int) []downloadChunkMeta`
- `effectiveFileChunkCount(requested int, size int64) int`
- `downloadFileMeta.Schema = 2`
- `metaMatchesDownloadFile(remote, rawURL)` 检查 URL、size、ETag、Last-Modified。
- 将公开入口签名调整为：

```go
func DownloadFile(rawURL, target string, getbar func(size int64) io.Writer, opts Options) (DownloadFileResult, error)
```

约束：

- 分片数量沿用 `effectiveChunkCount()` 的语义。
- 每个分片最小大小继续复用 `minChunkSize`。
- 单测通过构造 `12MB+` 的 body 触发 3 个分片；不要改 `minChunkSize` 常量。
- 单测只覆盖分片状态，不需要真实 100MB；通过覆盖 `resumableDownloadMinSize` 触发文件下载分片路径。

Run:

```bash
go test ./internal/client -run TestDownloadFileUsesParallelRangeChunksForLargeFiles
```

Expected: 仍 FAIL，直到分片下载实现完成。

- [x] **Step 3: 实现并发分片写 `.part`**

实现：

- `downloadFileParallel(rawURL, target, partPath, metaPath string, remote downloadFileRemote, getbar func(int64) io.Writer, opts Options) (DownloadFileResult, error)`
- 打开 `.part`：`os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY, 0o644)`。
- 预分配文件大小：优先 `file.Truncate(remote.Size)`。
- 每个 goroutine 请求一个未完成 chunk：
  - `Range: bytes=start-end`
  - 期望 `206 Partial Content`
  - 读取响应并用 `file.WriteAt()` 写入固定 offset。
  - 成功后锁住 meta，标记 `Done=true` 并保存 meta。
- 进度条用 `concurrentProgressWriter()` 统一汇总。
- 全部分片完成后关闭 progress，重命名 `.part` 到正式文件。

Run:

```bash
go test ./internal/client -run TestDownloadFileUsesParallelRangeChunksForLargeFiles
```

Expected: PASS。

- [x] **Step 4: 提交 Task 1**

Run:

```bash
git add internal/client/download_file.go internal/client/network.go internal/client/network_download_test.go docs/plans/resumable-parallel-download-plan.md
git commit -m "feat(download): add parallel resumable file chunks"
```

## Task 2: 断点续传只补未完成分片

**Files:**
- Modify: `internal/client/download_file.go`
- Test: `internal/client/network_download_test.go`
- Update: `docs/plans/resumable-parallel-download-plan.md`

- [x] **Step 1: 写失败测试：已完成分片不重复请求**

测试名称：`TestDownloadFileResumesOnlyMissingChunks`

测试要点：

- 预先创建 `.part`，写入完整大小，其中前两个分片内容正确。
- 预先写 meta v2：前两个 chunk `Done=true`，最后一个 `Done=false`。
- 服务端如果收到前两个分片的 Range 请求则 `t.Fatalf`。
- 服务端只允许最后一个分片请求。
- 下载完成后目标文件完整。
- `DownloadFileResult.Resumed == true`，`Parallel == true`。

Run:

```bash
go test ./internal/client -run TestDownloadFileResumesOnlyMissingChunks
```

Expected: FAIL。当前实现还没有按 chunk meta 跳过已完成分片。

- [x] **Step 2: 实现 meta 恢复逻辑**

实现：

- `loadOrCreateDownloadFileMeta(rawURL string, remote downloadFileRemote, partPath, metaPath string, chunks int) (downloadFileMeta, bool, error)`
- 如果 meta 匹配并且 chunks 合法，复用 `Done` 状态。
- 如果 meta 不匹配，删除 `.part` 并重新创建 meta。
- 如果 `.part` 不存在但 meta 存在，删除 meta 重建。
- 如果 `.part` 大小不等于 remote.Size，允许 `Truncate(remote.Size)`，但只信任 `Done=true` 的 chunk。

Run:

```bash
go test ./internal/client -run TestDownloadFileResumesOnlyMissingChunks
```

Expected: PASS。

- [x] **Step 3: 提交 Task 2**

Run:

```bash
git add internal/client/download_file.go internal/client/network_download_test.go docs/plans/resumable-parallel-download-plan.md
git commit -m "feat(download): resume missing file chunks"
```

## Task 3: Fallback 和错误语义

**Files:**
- Modify: `internal/client/download_file.go`
- Test: `internal/client/network_download_test.go`
- Update: `docs/plans/resumable-parallel-download-plan.md`

- [x] **Step 1: 写失败测试：Range 返回 200 时回退单连接重下**

测试名称：`TestDownloadFileRestartsWhenRangeChunkReturnsOK`

测试要点：

- 预置可续传 meta。
- 服务端对 Range GET 返回 `200 OK` 完整 body。
- 断言最终文件正确。
- 断言旧 `.part` 不残留。
- 断言 `DownloadFileResult.Resumed == false`。

Run:

```bash
go test ./internal/client -run TestDownloadFileRestartsWhenRangeChunkReturnsOK
```

Expected: FAIL。

- [x] **Step 2: 写失败测试：某个分片失败后保留状态**

测试名称：`TestDownloadFileKeepsCompletedChunksWhenOneChunkFails`

测试要点：

- 服务端让某个分片中途断开。
- 其他分片正常完成。
- 调用返回错误。
- `.part` 存在。
- meta 中成功分片 `Done=true`，失败分片 `Done=false`。

Run:

```bash
go test ./internal/client -run TestDownloadFileKeepsCompletedChunksWhenOneChunkFails
```

Expected: FAIL。

- [x] **Step 3: 实现 fallback 和失败保留规则**

实现：

- 任一分片返回 `200 OK`：取消并发分片，删除当前 `.part`/meta，调用单连接文件下载。
- 任一分片返回非 206 且非 200：返回错误，保留已完成 chunk 状态。
- 任一分片 copy/write 失败：返回错误，保留已完成 chunk 状态。
- `DownloadFileResult` 填充 `Path`、`Resumed`、`Parallel`、`Size`、`ETag`、`LastModified`。

Run:

```bash
go test ./internal/client
```

Expected: PASS。

- [x] **Step 4: 提交 Task 3**

Run:

```bash
git add internal/client/download_file.go internal/client/network_download_test.go docs/plans/resumable-parallel-download-plan.md
git commit -m "fix(download): handle parallel resume fallbacks"
```

## Task 4: Install/Download 接入返回结果

**Files:**
- Modify: `internal/install/network.go`
- Modify: `internal/install/runner.go`
- Test: `internal/install/runner_test.go`
- Update: `docs/plans/resumable-parallel-download-plan.md`

- [ ] **Step 1: 调整 install wrapper 返回 `DownloadFileResult`**

实现：

```go
func DownloadFile(url, target string, getbar func(size int64) io.Writer, opts Options) (client.DownloadFileResult, error)
```

`runner.downloadBody()` 调用后忽略 result，仍 `os.ReadFile(cachePath)`。

Run:

```bash
go test ./internal/install
```

Expected: PASS。

- [ ] **Step 2: 增加 install 并发续传集成测试**

测试名称：`TestDownloadBodyResumesMissingParallelChunks`

测试要点：

- 通过 cache path 预置 `.part` 和 meta v2。
- 服务端只允许请求未完成 chunk。
- `runner.downloadBody()` 返回完整 body。
- 正式 cache 文件存在，`.part` 不存在。

Run:

```bash
go test ./internal/install -run TestDownloadBodyResumesMissingParallelChunks
```

Expected: PASS。

- [ ] **Step 3: 提交 Task 4**

Run:

```bash
git add internal/install/network.go internal/install/runner.go internal/install/runner_test.go docs/plans/resumable-parallel-download-plan.md
git commit -m "feat(install): use parallel resumable cache downloads"
```

## Task 5: SDK 复用 client 下载器

**Files:**
- Modify: `internal/sdk/download.go`
- Test: `internal/sdk/download_test.go`
- Update: `docs/plans/resumable-parallel-download-plan.md`

- [ ] **Step 1: 写失败测试：SDK 使用并发分片下载**

测试名称：`TestDownloadArchiveUsesParallelClientDownload`

测试要点：

- SDK 请求大文件。
- 服务端统计 Range 请求数。
- 设置 `DownloadRequest.ClientOpts.ChunkConcurrency = 3`。
- 下载成功后断言收到多个 Range 请求。
- 断言 `DownloadResult.Path`、`Size`、`ETag` 正确。

Run:

```bash
go test ./internal/sdk -run TestDownloadArchiveUsesParallelClientDownload
```

Expected: FAIL。当前 SDK 是单连接续传。

- [ ] **Step 2: 修改 `DownloadArchive()` 调用 client**

实现：

- 保留：
  - `sdkDownloadFinalPath()`
  - `sdkDownloadPartPath()` 如果 client 仍支持外部指定 part path，否则删除该 helper。
  - `completeCacheMatches()` 作为 SDK 完整 cache 命中判断。
- 删除 SDK 自己的：
  - `newDownloadHTTPClient`
  - `probeDownload`
  - `downloadRequest`
  - `appendResponseToFile`
  - `writeResponseToFile`
  - `progressWriter`
  - SDK 私有 `fileSize`
- 调用：

```go
result, err := client.DownloadFile(req.URL, finalPath, req.Progress, req.ClientOpts)
```

- 保存 SDK meta 时使用 client result 中的 size、etag、last_modified。
- 映射：
  - `DownloadResult.Path = finalPath`
  - `DownloadResult.Resumed = result.Resumed`
  - `DownloadResult.Size = result.Size`
  - `DownloadResult.ETag = result.ETag`
  - `DownloadResult.Modified = result.LastModified`

Run:

```bash
go test ./internal/sdk
```

Expected: PASS。

- [ ] **Step 3: 调整旧 SDK 续传测试**

实现：

- `TestDownloadArchiveResumesPartWhenMetaMatches` 改为使用 client meta v2 或改名为 `TestDownloadArchiveResumesMissingChunksWhenMetaMatches`。
- 如果不再兼容 SDK 旧 `.part` meta，删除依赖旧 meta 的断言。
- 保留 `TestDownloadArchiveUsesCompleteCacheWhenMetaMatches`，确保完整 cache 命中不发网络请求。

Run:

```bash
go test ./internal/sdk
```

Expected: PASS。

- [ ] **Step 4: 提交 Task 5**

Run:

```bash
git add internal/sdk/download.go internal/sdk/download_test.go docs/plans/resumable-parallel-download-plan.md
git commit -m "feat(sdk): reuse resumable parallel downloader"
```

## Task 6: 验证和文档收尾

**Files:**
- Modify: `docs/plans/resumable-parallel-download-plan.md`

- [ ] **Step 1: 聚焦测试**

Run:

```bash
go test ./internal/client ./internal/install ./internal/sdk
```

Expected: PASS。

- [ ] **Step 2: 全量测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 3: 更新计划 checkbox 并提交**

Run:

```bash
git add docs/plans/resumable-parallel-download-plan.md
git commit -m "docs(download): complete parallel resume plan"
```

## 风险和取舍

- `WriteAt` 并发写同一文件需要确保每个 goroutine 只写自己的 `[start,end]` 范围，不能使用共享 seek offset。
- 每个 chunk 完成后写 meta，会增加少量磁盘写入；但分片数量很少，换来失败后精确续传，值得。
- 首次实现不做 chunk 校验哈希，因为远端通常只提供整体校验，且已有安装校验器会验证最终文件。
- SDK 不兼容旧单连接 `.part` meta；v0 阶段可以接受。完整 cache meta 仍保留，以避免重新下载已完成 SDK 包。
- 并发分片只用于支持 Range 且 size 明确的下载；其他情况继续单连接，行为清晰。
