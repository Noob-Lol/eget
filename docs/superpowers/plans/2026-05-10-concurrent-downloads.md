# 并发下载实现计划

> **给执行 Agent：** 实施本计划时必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，逐任务执行并维护下面的 checkbox 状态。

**目标：** 为 `install` / `download` 增加单文件 HTTP Range 分片并发下载，并为后续 `install --all` / `update --all` 增加批量任务并发能力。

**架构：** 保持现有主链路不变：解析目标 -> 选择 asset -> 下载 -> 校验 -> 解压/安装 -> 写入记录。新增 `chunk_concurrency` 只影响单个 asset 的下载策略，新增 `batch_concurrency` 只影响 `install --all` / `update --all` 的包任务调度。进度条使用已更新的 `github.com/gookit/cliui/progress`，单文件显示聚合进度，批量场景使用 multi progress 和 concurrent writer 避免输出互相覆盖。

**技术栈：** Go 1.24.2、`net/http`、现有 `internal/client` 网络层、现有 `internal/install` 安装 runner、现有 config merge 模型、`github.com/gookit/cliui/progress` byte tracker / concurrent writer / multi progress 能力。

---

## 设计约束

- 配置项使用 `chunk_concurrency` 和 `batch_concurrency`。
- 命令行选项使用 `--chunk N` 和 `--batch N`，方便输入。
- 值语义统一：
  - `0`：自动。
  - `1`：不并发。
  - `>1`：指定并发上限。
- `chunk_concurrency = 0` 首版自动策略：服务端支持 Range 且文件足够大时最多 5 个分片。
- `batch_concurrency = 0` 自动策略：最多使用 `min(total packages, 6)` 个 worker。
- 小文件不启用 Range 分片。建议最小分片大小为 `4 MiB`，至少能拆出 2 个有效分片才并发。
- `--chunk N` 是最大分片数，不是强制分片数。
- 不做断点续传，不改变缓存文件语义，不引入 partial cache。
- 不展示每个 chunk 的进度条；一个下载 asset 只显示一个聚合进度条。
- `batch_concurrency` 只允许 `[global]` 和 CLI 覆盖，不进入 package/repo 层配置。
- `--batch` 只对 `install --all` / `update --all` 生效，普通单包命令使用时报错。
- 涉及 MVP 主链路修改后必须运行 `go test ./...`。

## 文件职责

- `go.mod` / `go.sum`：固定包含 byte tracker / concurrent writer 能力的 `cliui` 版本。
- `internal/client/network.go`：实现 Range 探测、分片计算、并发分片下载和单连接 fallback。
- `internal/install/options.go`：新增 `ChunkConcurrency` / `BatchConcurrency`。
- `internal/install/network.go`：把 install options 转换到 client options。
- `internal/install/runner.go`：维持单包下载进度条，同时为并发 writer 保留兼容入口。
- `internal/config/model.go`：新增配置模型字段。
- `internal/config/merge.go`：实现 `chunk_concurrency` 的 CLI/package/repo/global 优先级。
- `internal/config/gookit.go`：支持配置读写、dump、`config set` 的 int 解析。
- `internal/app/install.go`：验证并发参数，实现 `install --all` 批量调度。
- `internal/app/update.go`：实现 `update --all` 批量调度。
- `internal/app/config.go`：`eget add --chunk N` 持久化 package 级别 chunk 配置。
- `internal/cli/*.go`：增加 flags，完成 option 映射，校验 `--batch` 使用范围。
- `README.md` / `README.zh-CN.md` / `docs/DOCS.md` / `docs/example.eget.toml` / `docs/TODO.md`：更新用户文档和示例配置。

---

## Task 1：确认 cliui 版本并建立 option 模型

**文件：**
- 修改：`go.mod`
- 修改：`go.sum`
- 修改：`internal/install/options.go`
- 修改：`internal/client/network.go`
- 修改：`internal/install/network.go`

- [x] **Step 1：确认 cliui 版本**

执行：

```bash
go list -m github.com/gookit/cliui
```

期望输出包含：

```text
github.com/gookit/cliui v0.2.5-0.20260509152712-bda530f32df3
```

如果版本不符合，执行：

```bash
go get github.com/gookit/cliui@main
go mod tidy
```

- [x] **Step 2：在 install options 增加字段**

在 `internal/install/options.go` 的 `Options` 结构体里新增字段：

```go
ChunkConcurrency int
BatchConcurrency int
```

字段放在 `FallbackVersions` 附近，保证安装/下载行为相关字段集中。

- [x] **Step 3：在 client options 增加字段**

在 `internal/client/network.go` 的 `Options` 结构体里新增：

```go
ChunkConcurrency int
```

- [x] **Step 4：转发 chunk 配置**

在 `internal/install/network.go` 的 `ClientOptions` 返回值中新增：

```go
ChunkConcurrency: opts.ChunkConcurrency,
```

- [x] **Step 5：验证编译**

执行：

```bash
go test ./internal/client ./internal/install
```

期望：PASS。

- [x] **Step 6：提交**

```bash
git add go.mod go.sum internal/install/options.go internal/client/network.go internal/install/network.go
git commit -m "feat(download): add concurrency option model"
```

---

## Task 2：实现配置读取、合并和参数校验

**文件：**
- 修改：`internal/config/model.go`
- 修改：`internal/config/merge.go`
- 修改：`internal/config/gookit.go`
- 修改：`internal/app/install.go`
- 测试：`internal/config/*_test.go`
- 测试：`internal/app/install_test.go`

- [x] **Step 1：补充配置读取测试**

在 `internal/config/loader_test.go` 增加测试，覆盖 global/package/repo 层级：

```go
func TestLoadFileDecodesConcurrencyOptions(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	writeTestFile(t, configPath, `
[global]
chunk_concurrency = 0
batch_concurrency = 3

[packages.fd]
repo = "sharkdp/fd"
chunk_concurrency = 2

["sharkdp/fd"]
chunk_concurrency = 4
`)

	cfg, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	assert.Eq(t, 0, *cfg.Global.ChunkConcurrency)
	assert.Eq(t, 3, *cfg.Global.BatchConcurrency)
	assert.Eq(t, 2, *cfg.Packages["fd"].ChunkConcurrency)
	assert.Eq(t, 4, *cfg.Repos["sharkdp/fd"].ChunkConcurrency)
}
```

- [x] **Step 2：补充 merge 优先级测试**

在 `internal/config/merge_test.go` 增加：

```go
func TestMergeInstallOptionsChunkConcurrencyPrecedence(t *testing.T) {
	globalChunk := 1
	repoChunk := 2
	pkgChunk := 3
	cliChunk := 4

	merged := MergeInstallOptions(
		Section{ChunkConcurrency: &globalChunk},
		Section{ChunkConcurrency: &repoChunk},
		Section{ChunkConcurrency: &pkgChunk},
		CLIOverrides{ChunkConcurrency: &cliChunk},
	)
	assert.Eq(t, 4, merged.ChunkConcurrency)

	merged = MergeInstallOptions(
		Section{ChunkConcurrency: &globalChunk},
		Section{ChunkConcurrency: &repoChunk},
		Section{ChunkConcurrency: &pkgChunk},
		CLIOverrides{},
	)
	assert.Eq(t, 3, merged.ChunkConcurrency)
}
```

- [x] **Step 3：补充配置 dump/config set 测试**

在 `internal/config/gookit_test.go` 增加 dump 覆盖：

```go
func TestDumpConfigIncludesConcurrencyOptions(t *testing.T) {
	chunk := 0
	batch := 3
	pkgChunk := 2
	cfg := NewFile()
	cfg.Global.ChunkConcurrency = &chunk
	cfg.Global.BatchConcurrency = &batch
	cfg.Packages["fd"] = Section{
		Repo:             util.StringPtr("sharkdp/fd"),
		ChunkConcurrency: &pkgChunk,
	}

	out, err := dumpConfigString(cfg)
	if err != nil {
		t.Fatalf("dump config string: %v", err)
	}
	assert.Contains(t, out, "chunk_concurrency = 0")
	assert.Contains(t, out, "batch_concurrency = 3")
	assert.Contains(t, out, "chunk_concurrency = 2")
}
```

- [x] **Step 4：先运行测试确认失败**

```bash
go test ./internal/config -run 'Concurrency|DumpConfigIncludesConcurrencyOptions' -v
```

期望：字段未实现时 FAIL。

- [x] **Step 5：增加配置模型字段**

在 `internal/config/model.go` 的 `Section` 增加：

```go
ChunkConcurrency *int `toml:"chunk_concurrency" mapstructure:"chunk_concurrency"`
BatchConcurrency *int `toml:"batch_concurrency" mapstructure:"batch_concurrency"`
```

在 merged/CLI override 相关结构增加：

```go
ChunkConcurrency int
```

以及：

```go
ChunkConcurrency *int
```

- [x] **Step 6：实现 chunk merge 优先级**

在 `internal/config/merge.go` 中按下面优先级合并：

```go
merged.ChunkConcurrency = firstInt(
	cli.ChunkConcurrency,
	pkg.ChunkConcurrency,
	repo.ChunkConcurrency,
	global.ChunkConcurrency,
)
```

新增辅助函数：

```go
func firstInt(values ...*int) int {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return 0
}
```

- [x] **Step 7：支持 gookit config 读写**

在 `internal/config/gookit.go` 的配置值解析中把这两个 key 作为 int：

```go
case "cache_time", "chunk_concurrency", "batch_concurrency":
	parsed, err := strconv.Atoi(text)
	if err != nil {
		return nil, false
	}
	return parsed, true
```

在 section 转 map 时输出：

```go
if section.ChunkConcurrency != nil {
	data["chunk_concurrency"] = *section.ChunkConcurrency
}
if section.BatchConcurrency != nil {
	data["batch_concurrency"] = *section.BatchConcurrency
}
```

- [x] **Step 8：增加参数校验**

在 `internal/app/install.go` 增加：

```go
const (
	maxChunkConcurrency = 32
	maxBatchConcurrency = 16
)

func validateConcurrencyOptions(opts install.Options) error {
	if opts.ChunkConcurrency < 0 || opts.ChunkConcurrency > maxChunkConcurrency {
		return fmt.Errorf("chunk concurrency must be between 0 and %d", maxChunkConcurrency)
	}
	if opts.BatchConcurrency < 0 || opts.BatchConcurrency > maxBatchConcurrency {
		return fmt.Errorf("batch concurrency must be between 0 and %d", maxBatchConcurrency)
	}
	return nil
}
```

在 `InstallTarget`、`DownloadTarget`、`InstallAllPackages` 完成配置合并后调用。

- [x] **Step 9：验证**

```bash
go test ./internal/config ./internal/app -run 'Concurrency|InstallTarget|DownloadTarget|InstallAllPackages' -v
```

期望：PASS。

- [x] **Step 10：提交**

```bash
git add internal/config/model.go internal/config/merge.go internal/config/gookit.go internal/config/*_test.go internal/app/install.go internal/app/install_test.go
git commit -m "feat(config): add concurrency settings"
```

---

## Task 3：增加 CLI flags 和 option 映射

**文件：**
- 修改：`internal/cli/install_cmd.go`
- 修改：`internal/cli/download_cmd.go`
- 修改：`internal/cli/update_cmd.go`
- 修改：`internal/cli/add_cmd.go`
- 修改：`internal/cli/options.go`
- 修改：`internal/cli/handlers.go`
- 测试：`internal/cli/*_test.go`

- [x] **Step 1：补充 CLI 解析测试**

覆盖这些命令：

```text
eget install --chunk 8 owner/repo
eget download --chunk 8 owner/repo
eget update --chunk 8 fd
eget add --chunk 3 sharkdp/fd
eget install --all --batch 3
eget update --all --batch 3
```

断言对应 options 中的 `ChunkConcurrency` / `BatchConcurrency` 被正确设置。

- [x] **Step 2：补充 `--batch` 约束测试**

在 handler 测试中覆盖：

```go
func TestHandleRejectsBatchWithoutAll(t *testing.T) {
	svc := &cliService{}
	err := svc.handle("install", &InstallOptions{Target: "owner/repo", BatchConcurrency: 2})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "--batch can only be used with --all")

	err = svc.handle("update", &UpdateOptions{Target: "fd", BatchConcurrency: 2})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "--batch can only be used with --all")
}
```

- [x] **Step 3：确认测试先失败**

```bash
go test ./internal/cli -run 'ConcurrencyFlags|RejectsBatch' -v
```

期望：FAIL。

- [x] **Step 4：增加 CLI option 字段**

在 install/update options 增加：

```go
ChunkConcurrency int
BatchConcurrency int
```

在 download/add options 增加：

```go
ChunkConcurrency int
```

- [x] **Step 5：增加 flags**

`install` / `update`：

```go
cmd.IntVar(&opts.ChunkConcurrency, "chunk", 0, "HTTP Range chunk concurrency: 0 auto, 1 single connection")
cmd.IntVar(&opts.BatchConcurrency, "batch", 0, "Concurrent package tasks for --all: 0 auto, 1 serial")
```

`download` / `add` 只增加：

```go
cmd.IntVar(&opts.ChunkConcurrency, "chunk", 0, "HTTP Range chunk concurrency: 0 auto, 1 single connection")
```

- [x] **Step 6：映射到 install.Options**

在 CLI 到 app/install options 的转换函数中填充：

```go
ChunkConcurrency: cli.ChunkConcurrency,
BatchConcurrency: cli.BatchConcurrency,
```

download/add 场景只映射 `ChunkConcurrency`。

- [x] **Step 7：实现 `--batch` 使用范围校验**

在 handler 中增加：

```go
if opts.BatchConcurrency != 0 && !opts.All {
	return fmt.Errorf("--batch can only be used with --all")
}
```

`install` 和 `update` 都需要校验。

- [x] **Step 8：验证**

```bash
go test ./internal/cli -run 'ConcurrencyFlags|RejectsBatch|Install|Download|Update|Add' -v
```

期望：PASS。

- [x] **Step 9：提交**

```bash
git add internal/cli/*.go internal/cli/*_test.go
git commit -m "feat(cli): add chunk and batch flags"
```

---

## Task 4：实现 HTTP Range 分片并发下载

**文件：**
- 修改：`internal/client/network.go`
- 测试：`internal/client/*_test.go`

- [x] **Step 1：补充 Range 下载测试**

用 `httptest.Server` 模拟支持 Range 的服务：

```go
func TestDownloadUsesRangeChunksForLargeFiles(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 10*1024*1024)
	var rangeRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		if header := r.Header.Get("Range"); header != "" {
			rangeRequests.Add(1)
			start, end := parseTestRange(t, header)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[start : end+1])
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Download(server.URL, &out, nil, Options{ChunkConcurrency: 4})
	assert.NoErr(t, err)
	assert.Eq(t, body, out.Bytes())
	assert.True(t, rangeRequests.Load() > 1)
}
```

- [x] **Step 2：补充小文件 fallback 测试**

```go
func TestDownloadSkipsRangeChunksForSmallFiles(t *testing.T) {
	body := bytes.Repeat([]byte("b"), 1024*1024)
	var rangeRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		if r.Header.Get("Range") != "" {
			rangeRequests.Add(1)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Download(server.URL, &out, nil, Options{ChunkConcurrency: 8})
	assert.NoErr(t, err)
	assert.Eq(t, body, out.Bytes())
	assert.Eq(t, int64(0), rangeRequests.Load())
}
```

- [x] **Step 3：补充不支持 Range fallback 测试**

服务端不返回 `Accept-Ranges: bytes` 时，断言仍然能单连接下载成功。

- [x] **Step 4：确认测试先失败**

```bash
go test ./internal/client -run 'Range|SmallFile|Fallback' -v
```

期望：FAIL。

- [x] **Step 5：增加分片策略常量和计算函数**

在 `internal/client/network.go` 增加：

```go
const (
	defaultAutoChunkConcurrency = 5
	minChunkSize                = 4 * 1024 * 1024
)

type byteRange struct {
	Start int64
	End   int64
}

func effectiveChunkCount(requested int, size int64) int {
	if requested == 1 || size < 2*minChunkSize {
		return 1
	}
	maxBySize := int(size / minChunkSize)
	if maxBySize < 2 {
		return 1
	}
	limit := requested
	if limit <= 0 {
		limit = defaultAutoChunkConcurrency
	}
	if limit > maxBySize {
		limit = maxBySize
	}
	if limit < 2 {
		return 1
	}
	return limit
}
```

- [x] **Step 6：实现 Range 探测**

增加 HEAD 探测函数，满足以下条件才允许分片：

```text
status 2xx
Accept-Ranges 包含 bytes
Content-Length > 0
effectiveChunkCount(...) > 1
```

如果 HEAD 被服务端拒绝或信息不完整，直接 fallback 单连接下载。

- [x] **Step 7：实现 range 拆分和并发下载**

拆分逻辑要求：

```text
总长度 size
分片数 chunks
前 chunks-1 个分片按 size/chunks 分配
最后一个分片结束到 size-1
```

每个 goroutine 使用 `Range: bytes=start-end` 请求，要求返回 `206 Partial Content`，写入预分配 `[]byte` 的对应 slice。所有分片成功后再一次性写入目标 writer。

- [x] **Step 8：接入聚合进度**

Range 分片下载时调用 `getbar(size)` 获取一个聚合 writer。多个 chunk worker 不直接操作 progress bar，而是写入 `progress.NewConcurrentWriterWithInterval(bar, 100*time.Millisecond)` 包装后的 writer。下载完成后 close writer，确保刷新剩余 byte。

- [x] **Step 9：接入 Download fallback**

在现有 `Download` 单连接逻辑前增加：

```go
if opts.ChunkConcurrency != 1 {
	if size, ok := probeRangeSupport(rawURL, opts); ok {
		chunks := effectiveChunkCount(opts.ChunkConcurrency, size)
		if chunks > 1 {
			bar := getbar(size)
			if bar == nil {
				bar = io.Discard
			}
			return downloadRangeChunks(rawURL, out, bar, size, chunks, opts)
		}
	}
}
```

如果分片下载返回错误，本任务先返回错误，不做隐式重试单连接，避免写入目标 writer 后产生混合数据。

- [x] **Step 10：验证**

```bash
go test ./internal/client -run 'Download|Range|SmallFile|Fallback|Cache' -v
```

期望：PASS。

- [x] **Step 11：提交**

```bash
git add internal/client/network.go internal/client/*_test.go
git commit -m "feat(download): support range chunk downloads"
```

---

## Task 5：调整下载进度条接入方式

**文件：**
- 修改：`internal/install/runner.go`
- 测试：`internal/install/runner_test.go`

- [x] **Step 1：补充进度 writer 兼容测试**

测试 `newDownloadProgress` 返回的对象仍可作为 `io.Writer` 使用，且不会影响现有单包下载。

```go
func TestNewDownloadProgressReturnsWriter(t *testing.T) {
	var out bytes.Buffer
	writer := newDownloadProgress(&out, 1024)
	if writer == nil {
		t.Fatal("expected progress writer")
	}
	_, ok := any(writer).(io.Writer)
	assert.True(t, ok)
}
```

- [x] **Step 2：保持单包行为稳定**

`newDownloadProgress` 继续返回单个 progress bar，不在这个任务中引入 multi progress。batch 场景单独处理。

- [x] **Step 3：验证**

```bash
go test ./internal/install -run 'Download|Progress|Cache' -v
```

期望：PASS。

- [x] **Step 4：提交**

```bash
git add internal/install/runner.go internal/install/runner_test.go
git commit -m "test(install): cover download progress writer behavior"
```

---

## Task 6：实现 `install --all` 批量并发

**文件：**
- 修改：`internal/app/install.go`
- 测试：`internal/app/install_test.go`

- [x] **Step 1：补充并发调度测试**

扩展 `fakeBatchRunner`，记录最大并发数、调用目标和 options。新增测试：

```go
func TestInstallAllPackagesUsesBatchConcurrencyAndPreservesResultOrder(t *testing.T) {
	block := make(chan struct{})
	runner := &fakeBatchRunner{block: block}
	svc := newInstallAllTestService(runner)

	done := make(chan []InstallAllResult, 1)
	errCh := make(chan error, 1)
	go func() {
		results, err := svc.InstallAllPackages(install.Options{BatchConcurrency: 2, Quiet: true})
		if err != nil {
			errCh <- err
			return
		}
		done <- results
	}()

	waitUntilMaxActive(t, runner, 2)
	close(block)

	select {
	case err := <-errCh:
		t.Fatalf("install all packages: %v", err)
	case results := <-done:
		assert.Eq(t, []string{"fd", "fzf", "rg"}, []string{results[0].Name, results[1].Name, results[2].Name})
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for install all")
	}
	assert.Eq(t, 2, runner.maxActive)
}
```

辅助函数可以放在同一测试文件中，使用 mutex 读取 `maxActive`。

- [x] **Step 2：确认测试先失败**

```bash
go test ./internal/app -run TestInstallAllPackagesUsesBatchConcurrencyAndPreservesResultOrder -v
```

期望：当前串行实现下 FAIL。

- [x] **Step 3：增加有效 batch 计算**

在 `internal/app/install.go` 增加：

```go
func effectiveBatchConcurrency(value, total int) int {
	if total <= 1 {
		return 1
	}
	if value <= 0 {
		return 1
	}
	if value > total {
		return total
	}
	return value
}
```

- [x] **Step 4：实现 worker scheduler**

`InstallAllPackages` 保持现有排序逻辑。`batch <= 1` 时继续走原串行逻辑；`batch > 1` 时：

```text
创建固定 worker 数量
jobs 中包含 index/name
每个 worker 独立 resolve package config
调用 installResolvedTarget
按 index 写入 results slice
任一任务失败时 cancel，最终返回错误
```

必须保证返回结果顺序仍然按原 package name 排序。

- [x] **Step 5：验证**

```bash
go test ./internal/app -run 'InstallAllPackages' -v
```

期望：PASS。

- [x] **Step 6：提交**

```bash
git add internal/app/install.go internal/app/install_test.go
git commit -m "feat(install): support batch concurrency"
```

---

## Task 7：实现 `update --all` 批量并发

**文件：**
- 修改：`internal/app/update.go`
- 测试：`internal/app/update_test.go`

- [x] **Step 1：补充 update batch 测试**

扩展 fake install service，记录最大并发数。新增测试覆盖：

```text
UpdateAllPackages(install.Options{BatchConcurrency: 2})
```

断言：

```text
最大并发数为 2
返回结果顺序稳定
每个 candidate 都被安装更新
```

- [x] **Step 2：确认测试先失败**

```bash
go test ./internal/app -run TestUpdateAllPackagesUsesBatchConcurrencyAndPreservesResultOrder -v
```

期望：当前串行实现下 FAIL。

- [x] **Step 3：复用 batch 策略**

`UpdateCandidates` 使用 `effectiveBatchConcurrency(cli.BatchConcurrency, len(candidates))`。

`batch <= 1` 保持现有串行逻辑；`batch > 1` 使用固定 worker 调度，按 candidate index 写回结果。

- [x] **Step 4：验证**

```bash
go test ./internal/app -run 'UpdateAll|UpdateCandidates|ListUpdateCandidates' -v
```

期望：PASS。

- [x] **Step 5：提交**

```bash
git add internal/app/update.go internal/app/update_test.go
git commit -m "feat(update): support batch concurrency"
```

---

## Task 8：支持 `eget add --chunk N` 持久化

**文件：**
- 修改：`internal/app/config.go`
- 修改：`internal/app/add_test.go`
- 修改：`internal/cli/add_cmd.go`
- 修改：`internal/cli/options.go`

- [x] **Step 1：补充持久化测试**

在 `internal/app/add_test.go` 增加：

```go
func TestAddPackagePersistsChunkConcurrency(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	svc := ConfigService{ConfigPath: configPath}

	err := svc.AddPackage("sharkdp/fd", "fd", install.Options{ChunkConcurrency: 3})
	assert.NoErr(t, err)

	cfg, err := cfgpkg.LoadFile(configPath)
	assert.NoErr(t, err)
	assert.Eq(t, 3, *cfg.Packages["fd"].ChunkConcurrency)
}
```

- [x] **Step 2：确认测试先失败**

```bash
go test ./internal/app -run TestAddPackagePersistsChunkConcurrency -v
```

期望：FAIL。

- [x] **Step 3：写入 package 配置**

在 `ConfigService.AddPackage` 创建 package section 时加入：

```go
if opts.ChunkConcurrency != 0 {
	section.ChunkConcurrency = &opts.ChunkConcurrency
}
```

不要持久化 `BatchConcurrency`。

- [x] **Step 4：验证**

```bash
go test ./internal/app -run 'AddPackage|ResolvePackageName' -v
go test ./internal/cli -run 'Add|ConcurrencyFlags' -v
```

期望：PASS。

- [x] **Step 5：提交**

```bash
git add internal/app/config.go internal/app/add_test.go internal/cli/add_cmd.go internal/cli/options.go internal/cli/app_test.go
git commit -m "feat(config): persist chunk concurrency for packages"
```

---

## Task 9：补充 batch 场景进度条设计接入

**文件：**
- 修改：`internal/app/install.go`
- 修改：`internal/app/update.go`
- 修改：`internal/install/runner.go`
- 测试：`internal/app/install_test.go`
- 测试：`internal/app/update_test.go`

- [x] **Step 1：定义输出规则**

实现时遵循：

```text
quiet=true：不展示 progress
非 TTY：使用 plain 或 disabled，不输出动态多行刷新
TTY 且 batch > 1：使用 MultiProgress
每个 worker slot 一个 bar
slot 被复用时用 ResetWith 更新 max/message
普通日志通过 RunExclusive/Printf/Println 输出
```

- [x] **Step 2：为 batch 下载传递 worker slot**

在 app 层调度 worker 时，给每个 worker 分配稳定 slot。slot 只代表当前 worker 正在处理的包，不代表 package 固定身份。

- [x] **Step 3：chunk 下载使用 byte tracker**

单个 asset 下载进度只按总 byte 聚合。chunk worker 写入 concurrent writer，不创建子 bar。

- [x] **Step 4：验证输出互斥**

增加轻量测试：使用 buffer 作为输出，模拟 batch > 1 的两个任务，断言输出不 panic、不出现交错错误标记。这里不做终端 UI 快照测试。

- [x] **Step 5：验证**

```bash
go test ./internal/app ./internal/install -run 'Progress|InstallAll|UpdateAll' -v
```

期望：PASS。

- [x] **Step 6：提交**

```bash
git add internal/app/install.go internal/app/update.go internal/install/runner.go internal/app/*_test.go internal/install/*_test.go
git commit -m "feat(progress): support concurrent download output"
```

---

## Task 10：更新文档和示例配置

**文件：**
- 修改：`README.md`
- 修改：`README.zh-CN.md`
- 修改：`docs/DOCS.md`
- 修改：`docs/example.eget.toml`
- 修改：`docs/TODO.md`

- [x] **Step 1：更新 README**

英文 README 增加：

```markdown
- `--chunk N`: Control HTTP Range chunk concurrency for one downloaded file. `0` means auto, `1` means single-connection download, and values greater than `1` request up to that many chunks.
- `--batch N`: Control package task concurrency for `install --all` and `update --all`. `0` means auto, `1` means serial, and values greater than `1` process up to that many packages at once.
```

- [x] **Step 2：更新中文 README**

中文 README 增加：

```markdown
- `--chunk N`: 控制单个下载文件的 HTTP Range 分片并发。`0` 表示自动，`1` 表示单连接下载，大于 `1` 表示最多使用该数量的分片。
- `--batch N`: 控制 `install --all` / `update --all` 的包任务并发。`0` 表示自动，`1` 表示串行，大于 `1` 表示最多同时处理该数量的包。
```

- [x] **Step 3：更新 `docs/DOCS.md`**

增加并发配置说明：

```markdown
## 并发下载

`chunk_concurrency` 控制单个 asset 下载的 HTTP Range 分片并发。

- `0`: 自动，当前在服务端支持 Range 且文件足够大时最多使用 5 个分片。
- `1`: 单连接下载。
- `>1`: 请求的最大分片数。

`batch_concurrency` 控制 `install --all` 和 `update --all` 的包任务并发。

- `0`: 自动，当前最多使用 `min(total packages, 6)` 个 worker。
- `1`: 串行。
- `>1`: 请求的 worker 数。

package/repo 配置只支持 `chunk_concurrency`。`batch_concurrency` 只支持 `[global]` 和 CLI `--batch`。
```

- [x] **Step 4：更新示例配置**

在 `docs/example.eget.toml` 的 `[global]` 增加：

```toml
chunk_concurrency = 0
batch_concurrency = 0
```

- [x] **Step 5：更新 TODO**

所有实现和测试完成后，`docs/TODO.md` 中对应项标记为完成：

```markdown
- [x] 增强 install/download/update 支持并发下载
  - `--chunk N` / `global.chunk_concurrency` 控制单文件 HTTP Range 分片并发
  - `--batch N` / `global.batch_concurrency` 控制 `install --all` / `update --all` 批处理并发
```

- [x] **Step 6：验证**

```bash
go test ./...
```

期望：PASS。

- [x] **Step 7：提交**

```bash
git add README.md README.zh-CN.md docs/DOCS.md docs/example.eget.toml docs/TODO.md
git commit -m "docs: document concurrent downloads"
```

---

## Task 11：最终验证和收尾

**文件：**
- 无计划源码变更。

- [x] **Step 1：格式化 Go 文件**

```bash
gofmt -w internal/client/network.go internal/install/options.go internal/install/network.go internal/install/runner.go internal/config/model.go internal/config/merge.go internal/config/gookit.go internal/app/install.go internal/app/update.go internal/app/config.go internal/cli/install_cmd.go internal/cli/download_cmd.go internal/cli/update_cmd.go internal/cli/add_cmd.go internal/cli/options.go internal/cli/handlers.go
```

- [x] **Step 2：完整测试**

```bash
go test ./...
```

期望：PASS。

- [x] **Step 3：检查 diff**

```bash
git diff --check
git status --short
```

期望：

```text
git diff --check 无输出
git status --short 只包含本功能相关文件
```

- [x] **Step 4：必要时提交机械清理**

如果 gofmt 或文档修正产生新 diff：

```bash
git add internal README.md README.zh-CN.md docs go.mod go.sum
git commit -m "chore: finalize concurrent download support"
```

如果没有新 diff，跳过。

## 自检

- [x] 覆盖配置项：`chunk_concurrency` / `batch_concurrency`。
- [x] 覆盖 CLI：`--chunk N` / `--batch N`。
- [x] 覆盖值语义：`0` 自动、`1` 串行或单连接、`>1` 指定并发上限。
- [x] 覆盖小文件不分片策略。
- [x] 覆盖 HTTP Range 支持探测和 fallback。
- [x] 覆盖 chunk 聚合进度，避免每个分片一个进度条。
- [x] 覆盖 `install --all` / `update --all` 批量并发。
- [x] 覆盖 `eget add --chunk N` 持久化 package 配置。
- [x] 覆盖 README、中文 README、内部文档、示例配置、TODO。
