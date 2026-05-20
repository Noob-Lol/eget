# Resumable Download Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use test-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `eget install` 和 `eget download` 下载大于 100MB 的远程资源时，自动保留 `.part` 部分文件，并在下一次运行时通过 HTTP Range 断点续传。

**Architecture:** 在 `internal/client` 新增面向文件路径的下载 API，负责 `.part` 文件、元数据、Range 续传和原子替换。`internal/install` 只在存在 cache path 时调用该 API，后续校验、解压、安装流程仍读取完整 cache 文件内容，避免扩大改动面。

**Tech Stack:** Go 标准库 `net/http`、`os`、`io`、`encoding/json`；现有 `client.Options`、`install.Options`、`httptest` 测试体系。

---

## 成功标准

- 大于 100MB 且响应支持 `Accept-Ranges: bytes` 的下载失败后，缓存目录保留 `cachePath + ".part"` 和 `cachePath + ".meta.json"`。
- 下一次运行同一个 URL 时，如果 `.part` 元数据匹配远端 `Content-Length` / `ETag` / `Last-Modified`，请求必须带 `Range: bytes=<partSize>-` 并追加写入。
- 成功完成后，`.part` 被替换为正式 cache 文件，`downloadBody()` 返回完整 bytes，后续校验和解压逻辑不变。
- 小文件、不支持 Range、没有 cache dir、元数据不匹配时，不使用续传，保持完整重下。
- 必须通过 `go test ./internal/client ./internal/install` 和 `go test ./...`。

## 文件结构

- Modify: `internal/client/network.go`
  - 新增 `DownloadFile()`、`.part`/meta helper、Range 续传请求逻辑。
  - 复用现有 `downloadGetWithOptions`、`requestWithOptions`、`downloadProgressWriter`、proxy/verbose 逻辑。
- Modify: `internal/client/network_download_test.go`
  - 新增客户端级断点续传测试，覆盖首次失败保留 `.part`、二次 Range 续传成功、不支持 Range 时不续传。
- Modify: `internal/install/network.go`
  - 增加 install 层 `DownloadFile()` wrapper，把 install 测试 hook 转发到 client。
- Modify: `internal/install/runner.go`
  - `downloadBody()` 在 cache path 可用时改为下载到 cache 文件，再读取 bytes。
- Modify: `internal/install/runner_test.go`
  - 新增 install/download 主链路自动续传测试。
- Modify: `AGENTS.md`
  - 在正在进行区域登记/清理本计划链接。
- Modify: `docs/resumable-download-plan.md`
  - 每个阶段完成后更新 checkbox。

## Task 1: Client File Download API

**Files:**
- Modify: `internal/client/network.go`
- Test: `internal/client/network_download_test.go`
- Update: `docs/resumable-download-plan.md`

- [x] **Step 1: 写失败测试：首次大文件中断保留 `.part`**

测试名称：`TestDownloadFileKeepsPartForLargeRangeDownloadFailure`

测试要点：
- 使用 `httptest.NewServer` 返回 `Content-Length = resumableDownloadMinSize + 1024`。
- GET 响应带 `Accept-Ranges: bytes`、`ETag`。
- 服务端写入前 512KB 后断开连接。
- 调用 `DownloadFile(server.URL, target, nil, Options{})` 预期返回错误。
- 断言 `target + ".part"` 存在且大小大于 0。
- 断言 `target + ".meta.json"` 存在。
- 断言正式 `target` 不存在。

Run:

```bash
go test ./internal/client -run TestDownloadFileKeepsPartForLargeRangeDownloadFailure
```

Expected: FAIL，原因是 `DownloadFile` 尚未定义。

- [x] **Step 2: 实现最小 `DownloadFile()`**

实现约束：
- 无 `.part` 时使用现有 `downloadGetWithOptions(rawURL, opts)` 发起普通 GET，避免破坏现有 install 单测 hook。
- 如果响应 `Content-Length > 100MB` 且 `Accept-Ranges` 包含 `bytes`，写入 `target + ".part"`，发生 copy 错误时保留 `.part`。
- 如果不满足续传条件，写入 `.part` 但失败时删除 `.part`，成功时重命名为正式目标。
- 成功重命名前先删除旧目标，兼容 Windows。

Run:

```bash
go test ./internal/client -run TestDownloadFileKeepsPartForLargeRangeDownloadFailure
```

Expected: PASS。

- [x] **Step 3: 写失败测试：已有 `.part` 时 Range 续传**

测试名称：`TestDownloadFileResumesLargeRangeDownload`

测试要点：
- 预先创建 `target + ".part"`，内容为完整 body 的前半部分。
- 预先写入匹配 URL、size、ETag 的 meta 文件。
- 服务端 HEAD 返回 `Content-Length`、`Accept-Ranges: bytes`、`ETag`。
- 服务端 GET 必须收到 `Range: bytes=<partSize>-`，返回 `206 Partial Content` 和剩余内容。
- 调用 `DownloadFile()` 后断言正式 target 内容等于完整 body，`.part` 不存在。

Run:

```bash
go test ./internal/client -run TestDownloadFileResumesLargeRangeDownload
```

Expected: FAIL，原因是当前实现尚未处理已有 `.part` 续传。

- [x] **Step 4: 实现 Range 续传和元数据匹配**

实现约束：
- 新增 `downloadFileMeta`，字段包含 `schema`、`url`、`size`、`etag`、`last_modified`。
- `.part` 存在时先 HEAD 探测远端信息。
- 只有 URL、size、ETag/Last-Modified 匹配且远端支持 Range 时才发送 `Range: bytes=<partSize>-`。
- Range 返回 `206` 时 append；返回 `200` 时截断重下；其他状态返回错误并保留 `.part`。
- `.part` 大小等于远端大小时直接替换为正式目标。

Run:

```bash
go test ./internal/client -run "TestDownloadFile(KeepsPart|Resumes)"
```

Expected: PASS。

- [x] **Step 5: 写并通过不支持 Range 的回归测试**

测试名称：`TestDownloadFileRemovesPartWhenLargeDownloadCannotResume`

测试要点：
- 服务端返回大文件大小，但不带 `Accept-Ranges`。
- 传输中断后，断言 `.part` 被删除。
- 二次下载不应带 Range。

Run:

```bash
go test ./internal/client
```

Expected: PASS。

- [x] **Step 6: 更新计划并提交 Task 1**

Run:

```bash
git add internal/client/network.go internal/client/network_download_test.go docs/resumable-download-plan.md
git commit -m "feat(download): add resumable file download"
```

## Task 2: Install/Download Cache Integration

**Files:**
- Modify: `internal/install/network.go`
- Modify: `internal/install/runner.go`
- Test: `internal/install/runner_test.go`
- Update: `docs/resumable-download-plan.md`

- [x] **Step 1: 写失败测试：`downloadBody()` 自动续传 cache `.part`**

测试名称：`TestDownloadBodyResumesLargeCachedDownload`

测试要点：
- 使用 `httptest.NewServer` 提供 HEAD 和 Range GET。
- 根据 `CacheFilePathWithMeta(cacheDir, url, CacheMeta{})` 预写 `.part` 和 `.meta.json`。
- 调用 `runner.downloadBody(url, Options{CacheDir: cacheDir})`。
- 断言服务端收到正确 Range header。
- 断言返回 body 是完整内容，正式 cache 文件存在，`.part` 不存在。

Run:

```bash
go test ./internal/install -run TestDownloadBodyResumesLargeCachedDownload
```

Expected: FAIL，原因是 `downloadBody()` 尚未调用文件下载 API。

- [x] **Step 2: 增加 install 层 `DownloadFile()` wrapper**

实现约束：
- 在 `internal/install/network.go` 增加 `DownloadFile(url, path string, getbar func(int64) io.Writer, opts Options) error`。
- 和现有 `Download()` 一样转发 `downloadGetWithOptions`、`httpDo`、proxy notice writer、verbose 到 client。
- 返回 client 错误即可，runner 不需要暴露 client result。

Run:

```bash
go test ./internal/install -run TestDownloadBodyResumesLargeCachedDownload
```

Expected: 仍 FAIL，直到 runner 接入。

- [x] **Step 3: 修改 `downloadBody()` 使用 cache 文件下载**

实现约束：
- 如果 `cachePath != "" && !IsLocalFile(url)`：
  - 先保留现有完整 cache 命中逻辑。
  - 未命中时调用 `DownloadFile(url, cachePath, progressFactory, opts)`。
  - 成功后 `os.ReadFile(cachePath)` 返回 bytes。
- 如果没有 cache path 或本地文件，继续使用现有 `Download()` 到内存。
- 不改变 verifier/extractor 的入参类型。

Run:

```bash
go test ./internal/install -run "TestDownloadBody(UsesCache|WritesCache|UsesCacheMetadata|Resumes)"
```

Expected: PASS。

- [x] **Step 4: 更新计划并提交 Task 2**

Run:

```bash
git add internal/install/network.go internal/install/runner.go internal/install/runner_test.go docs/resumable-download-plan.md
git commit -m "feat(install): resume cached downloads"
```

## Task 3: Verification

**Files:**
- Modify: `docs/resumable-download-plan.md`
- Modify: `AGENTS.md`

- [ ] **Step 1: 聚焦测试**

Run:

```bash
go test ./internal/client ./internal/install
```

Expected: PASS。

- [ ] **Step 2: 全量测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 3: 清理进行中登记并提交验证状态**

实现约束：
- 将本计划剩余 checkbox 更新为完成。
- 从 `AGENTS.md` 正在进行区域移除本计划链接。

Run:

```bash
git add AGENTS.md docs/resumable-download-plan.md
git commit -m "docs(download): complete resumable download plan"
```

## 风险和取舍

- 首次大文件下载使用单连接流式写入 `.part`，优先保证失败可续传；现有并发分片下载仍保留在 `client.Download()` 路径中。
- 只有响应头能确认大于 100MB 且支持 Range 时才保留 `.part`，避免为不可续传资源留下无用临时文件。
- 元数据只用于判断 `.part` 是否可续传，不改变现有完整 cache 命中策略，避免引入额外 cache 失效行为。
