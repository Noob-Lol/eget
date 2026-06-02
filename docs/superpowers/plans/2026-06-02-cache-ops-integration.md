# Cache Ops Integration Enhancements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 cache management Phase 5 中的 A+B 集成增强：`cache list/status`、`cache clean --json`、`cache serve --token`、`cache serve --json-log`。

**Architecture:** 继续保持 `internal/app/cache` 作为 cache 领域能力边界，新增只读报表模型和 token/json-log server 选项；CLI 层只负责参数绑定、文本/JSON 输出和启动 HTTP server。`cache serve --token` 通过 handler 中间件保护 `/`、`/manifest.json`、`/files/*`、`/download/*`，默认允许 `/healthz` 无 token；`--json-log` 在 HTTP handler 外层记录结构化请求日志到 stderr。

**Tech Stack:** Go, net/http, httptest, gookit/gcli, `encoding/json`, `github.com/gookit/goutil/testutil/assert`, GitNexus impact/detect-changes。

---

## 范围确认

本计划会修改超过 3 个逻辑文件，主要涉及 `internal/app/cache`、`internal/cli`、README/docs/TODO 和测试。实施前需要用户再次确认范围；确认后按任务分阶段提交。

本计划不实现：

- cache registry 化、source metadata 搜索、无需第三方 provider 的解析能力。
- token 配置文件持久化；`--token` 仅为启动参数。
- cache mirror client 发送 token；`cache serve --token` 本计划只保护服务端 HTTP 访问，客户端认证下载另起设计。
- TLS、用户体系、多 token、token rotate。
- 复杂日志 sink；`--json-log` 只输出到 CLI stderr，适合 systemd/容器日志采集。

当前工作树可能存在 GitNexus 自动写入的 `AGENTS.md` 未提交变更。实施时不要提交它，除非用户明确要求。

## 设计决策

### `cache list`

命令：

```bash
eget cache list
eget cache list --root sdk
eget cache list --json
```

行为：

- 默认列出非 partial 的可分享缓存：`pkg`、`api`、`sdk`、`sdk-index`。
- `--root all|pkg|api|sdk|sdk-index|partial` 复用已有 root/kind 语义；`partial` 只用于 CLI list/status，不用于 `cache serve --root`。
- JSON 输出每个文件的 `kind`、`path`、`path_key`、`size`、`mod_time`。
- 文本输出保持简洁：cache dir、文件数量、总大小，以及 path/kind/size 表格。

### `cache status`

命令：

```bash
eget cache status
eget cache status --json
```

行为：

- 展示 cache dir、总文件数/大小、各 kind 文件数/大小。
- 展示 `[cache_mirror]` 当前 enable/url/fallback/timeout 状态。
- 给出 `cache serve --host 0.0.0.0 --port 8686` 建议命令。
- JSON 输出包含同样字段。

### `cache clean --json`

命令：

```bash
eget cache clean --dry-run --json
eget cache clean --all --yes --json
```

行为：

- dry-run 时输出 preview JSON，不打印文本提示。
- 真实 clean 时输出 result JSON，不打印文本 summary。
- 仍保留大删除保护；非 dry-run 且需要确认时，如果未传 `--yes`，行为不变，先报错/交互，不输出成功 JSON。

### `cache serve --token`

命令：

```bash
eget cache serve --host 0.0.0.0 --port 8686 --token "$EGET_CACHE_TOKEN"
```

行为：

- token 为空时不启用认证。
- token 非空时，`/`、`/manifest.json`、`/files/*`、`/download/*` 要求 `Authorization: Bearer <token>`。
- `/healthz` 不要求 token，方便健康检查。
- 认证失败统一返回 `401 Unauthorized`，设置 `WWW-Authenticate: Bearer`，不暴露 token 或路径细节。

### `cache serve --json-log`

命令：

```bash
eget cache serve --json-log
```

行为：

- 每个请求输出一行 JSON 到 stderr。
- 字段：`ts`、`method`、`path`、`status`、`bytes`、`duration_ms`、`remote_addr`。
- 不记录 token、query string、Authorization header。

## 前置 GitNexus 要求

实施前刷新并检查索引：

```bash
npx gitnexus status
```

如果状态 stale：

```bash
npx gitnexus analyze
```

修改任意函数、方法、类型前，必须先运行 impact analysis。至少覆盖：

```bash
npx gitnexus impact --repo eget Service
npx gitnexus impact --repo eget Entry
npx gitnexus impact --repo eget CleanOptions
npx gitnexus impact --repo eget CleanResult
npx gitnexus impact --repo eget ServeOptions
npx gitnexus impact --repo eget cacheHandler.ServeHTTP
npx gitnexus impact --repo eget CacheCleanOptions
npx gitnexus impact --repo eget CacheServeOptions
npx gitnexus impact --repo eget newCacheCmd
npx gitnexus impact --repo eget handleCacheClean
npx gitnexus impact --repo eget handleCacheServe
```

如果 GitNexus 对方法名解析失败，使用文件路径或更具体 symbol name 重试。若 impact 返回 HIGH 或 CRITICAL，先向用户说明影响面再继续。

提交前必须运行：

```bash
npx gitnexus detect-changes --repo eget
```

## 文件结构

- Create: `internal/app/cache/report.go`
  - `ListOptions`、`ListFile`、`ListResult`、`StatusResult`、`KindSummary`。
  - `Service.List`、`Service.Status`。
  - path-key 计算和 kind 汇总。
- Create: `internal/app/cache/report_test.go`
  - 覆盖 list root/kind 过滤、JSON 字段需要的 path_key、status kind 汇总、mirror 配置状态。
- Modify: `internal/app/cache/model.go`
  - `ServeOptions` 增加 `Token string`、`JSONLog bool`。
- Create: `internal/app/cache/auth_log.go`
  - token auth wrapper、json log wrapper、status/bytes response writer。
- Create: `internal/app/cache/auth_log_test.go`
  - 覆盖 token 保护、healthz 例外、json log 不泄露 query/token。
- Modify: `internal/app/cache/server.go`
  - `NewHandler` 包装 auth/log middleware；或保持 `cacheHandler` 内部调用。
- Modify: `internal/cli/cache_cmd.go`
  - 新增 `CacheListOptions`、`CacheStatusOptions`，`cache list/status` 命令。
  - `CacheCleanOptions.JSON`，`CacheServeOptions.Token/JSONLog`。
- Modify: `internal/cli/app.go`
  - commandFlagSpecs 增加 cache list/status/clean json/serve token/json-log。
- Modify: `internal/cli/handlers.go`
  - 分发 `cache.list`、`cache.status`。
- Modify: `internal/cli/cache_handler.go`
  - 实现 `handleCacheList`、`handleCacheStatus`，扩展 `handleCacheClean` JSON 输出，serve options 传 token/json-log。
- Modify: `internal/cli/app_cache_test.go`
  - 覆盖新命令和 flag 绑定。
- Modify: `internal/cli/cache_cmd_test.go`
  - 覆盖 list/status 文本和 JSON、clean JSON。
- Modify: `README.md`
  - cache management 示例增加 list/status/clean JSON/token/json-log。
- Modify: `README.zh-CN.md`
  - 中文说明同步。
- Modify: `docs/config.md`
  - 说明 token 是 CLI 启动参数，不属于 `[cache_mirror]`。
- Modify: `docs/config.zh-CN.md`
  - 中文说明同步。
- Modify: `docs/TODO.md`
  - 勾选 Phase 5 已完成项，保留 registry 和 manifest TTL 未完成。

## Task 1: cache list/status app 层报表

**Files:**
- Create: `internal/app/cache/report.go`
- Create: `internal/app/cache/report_test.go`

- [x] **Step 1: impact analysis**

```bash
npx gitnexus impact --repo eget Service
npx gitnexus impact --repo eget Entry
```

Expected: cache service tests and CLI cache handlers. Report if HIGH/CRITICAL.

- [x] **Step 2: 写 list/status 失败测试**

Create `internal/app/cache/report_test.go`:

```go
package cache

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/util"
)

func TestServiceListReportsFilesWithPathKeys(t *testing.T) {
	cacheDir := t.TempDir()
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg-cache", "tool.zip"), "pkg")
	writeCacheTestFile(t, filepath.Join(cacheDir, "sdk-downloads", "go", "1.22.0", "go.zip"), "sdk")
	writeCacheTestFile(t, filepath.Join(cacheDir, "tool.zip.part"), "partial")

	result, err := (Service{}).List(cacheDir, ListOptions{})

	assert.NoErr(t, err)
	assert.Eq(t, cacheDir, result.CacheDir)
	assert.Eq(t, 2, len(result.Files))
	assert.Eq(t, int64(6), result.TotalSize)
	assert.Eq(t, "pkg-cache/tool.zip", result.Files[0].Path)
	assert.Contains(t, result.Files[0].PathKey, "path-md5:")
}

func TestServiceListCanSelectPartialRoot(t *testing.T) {
	cacheDir := t.TempDir()
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg-cache", "tool.zip"), "pkg")
	writeCacheTestFile(t, filepath.Join(cacheDir, "tool.zip.part"), "partial")

	result, err := (Service{}).List(cacheDir, ListOptions{Root: "partial"})

	assert.NoErr(t, err)
	assert.Eq(t, 1, len(result.Files))
	assert.Eq(t, KindPartial, result.Files[0].Kind)
}

func TestServiceStatusSummarizesKindsAndMirrorConfig(t *testing.T) {
	cacheDir := t.TempDir()
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg-cache", "tool.zip"), "pkg")
	writeCacheTestFile(t, filepath.Join(cacheDir, "api-cache", "repo.json"), "{}")
	timeout := 5
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = util.StringPtr(cacheDir)
	cfg.CacheMirror.Enable = util.BoolPtr(true)
	cfg.CacheMirror.URL = util.StringPtr("http://127.0.0.1:8686")
	cfg.CacheMirror.Timeout = &timeout
	cfg.CacheMirror.Fallback = util.BoolPtr(true)

	result, err := (Service{Config: cfg, Now: func() time.Time {
		return time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	}}).Status("")

	assert.NoErr(t, err)
	assert.Eq(t, cacheDir, result.CacheDir)
	assert.Eq(t, 2, result.TotalFiles)
	assert.Eq(t, int64(5), result.TotalSize)
	assert.Eq(t, 1, result.Kinds[string(KindPkg)].Files)
	assert.True(t, result.CacheMirror.Enable)
	assert.Eq(t, "http://127.0.0.1:8686", result.CacheMirror.URL)
	assert.Contains(t, result.ServeCommand, "eget cache serve")
}
```

Run:

```bash
go test ./internal/app/cache -run "ServiceList|ServiceStatus"
```

Expected: FAIL because `ListOptions`, `Service.List`, `Service.Status` do not exist.

- [x] **Step 3: 实现 report 模型和方法**

Create `internal/app/cache/report.go`:

```go
package cache

import (
	"sort"
	"time"

	"github.com/inherelab/eget/internal/cachemirror"
)

type ListOptions struct {
	Root string
}

type ListFile struct {
	Kind    Kind      `json:"kind"`
	Path    string    `json:"path"`
	PathKey string    `json:"path_key,omitempty"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

type ListResult struct {
	CacheDir   string     `json:"cache_dir"`
	Root       string     `json:"root"`
	TotalFiles int        `json:"total_files"`
	TotalSize  int64      `json:"total_size"`
	Files      []ListFile `json:"files"`
}

type KindSummary struct {
	Files int   `json:"files"`
	Size  int64 `json:"size"`
}

type CacheMirrorStatus struct {
	Enable   bool   `json:"enable"`
	URL      string `json:"url,omitempty"`
	Timeout  int    `json:"timeout,omitempty"`
	Fallback bool   `json:"fallback"`
}

type StatusResult struct {
	CacheDir     string                 `json:"cache_dir"`
	GeneratedAt  time.Time              `json:"generated_at"`
	TotalFiles   int                    `json:"total_files"`
	TotalSize    int64                  `json:"total_size"`
	Kinds        map[string]KindSummary `json:"kinds"`
	ServeCommand string                 `json:"serve_command"`
	CacheMirror  CacheMirrorStatus      `json:"cache_mirror"`
}
```

Implementation details:

- `Service.List(cacheDir string, opts ListOptions)` calls `Service.Scan`.
- For `Root == "" || "all"`, include `KindPkg/API/SDK/SDKIndex` and exclude partial.
- For `Root == "partial"`, call scan with `Kinds: []Kind{KindPartial}`.
- Sort files by `Path` for stable CLI/test output.
- Use `cachemirror.KeyForRelPath(entry.RelPath)` only for non-partial entries.
- `Service.Status` scans all five kinds and summarizes by kind.
- Mirror config is read directly from `s.Config.CacheMirror`; default timeout `5` if nil or <=0.

- [x] **Step 4: 运行 app cache 测试**

```bash
go test ./internal/app/cache -run "ServiceList|ServiceStatus|ServiceScan"
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/app/cache/report.go internal/app/cache/report_test.go
git commit -m "feat(cache): add cache list and status reports"
```

## Task 2: CLI cache list/status 和 clean JSON

**Files:**
- Modify: `internal/cli/cache_cmd.go`
- Modify: `internal/cli/app.go`
- Modify: `internal/cli/handlers.go`
- Modify: `internal/cli/cache_handler.go`
- Modify: `internal/cli/app_cache_test.go`
- Modify: `internal/cli/cache_cmd_test.go`

- [x] **Step 1: impact analysis**

```bash
npx gitnexus impact --repo eget CacheCleanOptions
npx gitnexus impact --repo eget newCacheCmd
npx gitnexus impact --repo eget handleCacheClean
```

Expected: CLI cache tests and command wiring. Report if HIGH/CRITICAL.

- [x] **Step 2: 写 CLI flag 绑定失败测试**

Append to `internal/cli/app_cache_test.go`:

```go
func TestMain_CacheListBindsOptions(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"cache", "list", "--root", "sdk", "--json"})

	assert.NoErr(t, err)
	assert.Eq(t, "cache.list", calls[0].name)
	opts, ok := calls[0].options.(*CacheListOptions)
	assert.True(t, ok)
	assert.Eq(t, "sdk", opts.Root)
	assert.True(t, opts.JSON)
}

func TestMain_CacheStatusBindsJSON(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"cache", "status", "--json"})

	assert.NoErr(t, err)
	assert.Eq(t, "cache.status", calls[0].name)
	opts, ok := calls[0].options.(*CacheStatusOptions)
	assert.True(t, ok)
	assert.True(t, opts.JSON)
}

func TestMain_CacheCleanBindsJSON(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"cache", "clean", "--dry-run", "--json"})

	assert.NoErr(t, err)
	assert.Eq(t, "cache.clean", calls[0].name)
	opts := calls[0].options.(*CacheCleanOptions)
	assert.True(t, opts.DryRun)
	assert.True(t, opts.JSON)
}
```

Run:

```bash
go test ./internal/cli -run "CacheListBinds|CacheStatusBinds|CacheCleanBindsJSON"
```

Expected: FAIL because command/options do not exist.

- [x] **Step 3: 写 CLI handler JSON/text 失败测试**

Append to `internal/cli/cache_cmd_test.go`:

```go
func TestCliServiceHandleCacheListJSON(t *testing.T) {
	tmp := newCLICacheDir(t)
	writeCLITestFile(t, filepath.Join(tmp, "pkg-cache", "tool.zip"), "pkg")
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	service := &cliService{cacheService: appcache.Service{Config: cfg}}

	err := service.handleCacheList(&CacheListOptions{JSON: true})

	assert.NoErr(t, err)
}

func TestCliServiceHandleCacheStatusText(t *testing.T) {
	tmp := newCLICacheDir(t)
	writeCLITestFile(t, filepath.Join(tmp, "pkg-cache", "tool.zip"), "pkg")
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	var stderr bytes.Buffer
	service := &cliService{cacheService: appcache.Service{Config: cfg}, stderr: &stderr}

	err := service.handleCacheStatus(&CacheStatusOptions{})

	assert.NoErr(t, err)
	assert.Contains(t, stderr.String(), "Cache status")
	assert.Contains(t, stderr.String(), "cache dir:")
}

func TestCliServiceHandleCacheCleanDryRunJSON(t *testing.T) {
	tmp := newCLICacheDir(t)
	writeCLITestFile(t, filepath.Join(tmp, "old.zip"), "old")
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	service := &cliService{cacheService: appcache.Service{Config: cfg}}

	err := service.handleCacheClean(&CacheCleanOptions{Older: "3d", DryRun: true, JSON: true})

	assert.NoErr(t, err)
	assert.True(t, fileExistsCLI(filepath.Join(tmp, "old.zip")))
}
```

Run:

```bash
go test ./internal/cli -run "HandleCacheList|HandleCacheStatus|HandleCacheCleanDryRunJSON"
```

Expected: FAIL because handlers do not exist / JSON flag not implemented.

- [x] **Step 4: 实现 CLI options、commands 和 flag spec**

Modify `internal/cli/cache_cmd.go`:

- Add:

```go
type CacheListOptions struct {
	Root string
	JSON bool
}

type CacheStatusOptions struct {
	JSON bool
}
```

- Add `JSON bool` to `CacheCleanOptions`.
- Add `newCacheListCmd` and `newCacheStatusCmd`.
- Add new subs in `newCacheCmd` before clean/serve.
- Update help examples.
- Add a separate `isValidCacheListRoot` helper accepting `partial`; keep `isValidCacheRoot` unchanged for `cache serve`.

Modify `internal/cli/app.go` `commandFlagSpecs["cache"].subs`:

```go
"list": {
	bools:  setOf("json", "j"),
	values: setOf("root"),
},
"status": {
	bools: setOf("json", "j"),
},
"clean": {
	bools:  setOf("all", "a", "dry-run", "yes", "y", "pkg", "api", "sdk", "sdk-index", "partial", "json", "j"),
	values: setOf("older"),
},
```

Modify `internal/cli/handlers.go`:

```go
case "cache.list":
	opts := options.(*CacheListOptions)
	return s.handleCacheList(opts)
case "cache.status":
	opts := options.(*CacheStatusOptions)
	return s.handleCacheStatus(opts)
```

- [x] **Step 5: 实现 CLI handlers**

Modify `internal/cli/cache_handler.go`:

- Import `github.com/inherelab/eget/internal/cli/render` if not already imported.
- Add:

```go
func (s *cliService) handleCacheList(opts *CacheListOptions) error {
	result, err := s.cacheService.List("", appcache.ListOptions{Root: opts.Root})
	if err != nil {
		return err
	}
	if opts.JSON {
		return render.PrintJSON(result)
	}
	ccolor.Fprintf(s.stderrWriter(), "Cache files: %d (%s)\n", result.TotalFiles, formatBytes(result.TotalSize))
	ccolor.Fprintf(s.stderrWriter(), " - cache dir: %s\n", result.CacheDir)
	for _, file := range result.Files {
		ccolor.Fprintf(s.stderrWriter(), " - %s\t%s\t%s\n", file.Kind, formatBytes(file.Size), file.Path)
	}
	return nil
}
```

- Add `handleCacheStatus` with text summary.
- In `handleCacheClean`, if `opts.JSON && cleanOpts.DryRun`, print preview JSON and return.
- In `handleCacheClean`, after actual clean, if `opts.JSON`, print result JSON and return.

- [x] **Step 6: 运行 CLI 测试**

```bash
go test ./internal/cli -run "Cache"
```

Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add internal/cli/cache_cmd.go internal/cli/app.go internal/cli/handlers.go internal/cli/cache_handler.go internal/cli/app_cache_test.go internal/cli/cache_cmd_test.go
git commit -m "feat(cache): add list status and clean json commands"
```

## Task 3: cache serve token auth

**Files:**
- Modify: `internal/app/cache/model.go`
- Create: `internal/app/cache/auth_log.go`
- Create/Modify: `internal/app/cache/auth_log_test.go`
- Modify: `internal/app/cache/server.go`
- Modify: `internal/cli/cache_cmd.go`
- Modify: `internal/cli/app.go`
- Modify: `internal/cli/cache_handler.go`
- Modify: `internal/cli/app_cache_test.go`

- [x] **Step 1: impact analysis**

```bash
npx gitnexus impact --repo eget ServeOptions
npx gitnexus impact --repo eget cacheHandler.ServeHTTP
npx gitnexus impact --repo eget CacheServeOptions
npx gitnexus impact --repo eget handleCacheServe
```

Expected: cache server tests and CLI cache serve tests. Report if HIGH/CRITICAL.

- [x] **Step 2: 写 token auth 失败测试**

Create `internal/app/cache/auth_log_test.go`:

```go
package cache

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestCacheServerTokenProtectsManifest(t *testing.T) {
	handler := NewHandler(Service{}, t.TempDir(), ServeOptions{Token: "secret"})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/manifest.json", nil))
	assert.Eq(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "Bearer")

	req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	req.Header.Set("Authorization", "Bearer secret")
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, req)
	assert.Eq(t, http.StatusOK, okRec.Code)
}

func TestCacheServerTokenAllowsHealthz(t *testing.T) {
	handler := NewHandler(Service{}, t.TempDir(), ServeOptions{Token: "secret"})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Eq(t, http.StatusOK, rec.Code)
}
```

Run:

```bash
go test ./internal/app/cache -run "Token"
```

Expected: FAIL because token is ignored.

- [x] **Step 3: 实现 token auth middleware**

Modify `internal/app/cache/model.go`:

```go
type ServeOptions struct {
	Host    string
	Port    int
	Root    string
	NoIndex bool
	Version string
	Token   string
	JSONLog bool
}
```

Create `internal/app/cache/auth_log.go` with:

```go
func withBearerToken(next http.Handler, token string) http.Handler {
	token = strings.TrimSpace(token)
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

Modify `NewHandler` in `server.go`:

```go
handler := http.Handler(cacheHandler{service: service, cacheDir: cacheDir, opts: opts})
handler = withBearerToken(handler, opts.Token)
return handler
```

- [x] **Step 4: 绑定 CLI token flag**

Modify `CacheServeOptions`:

```go
Token string
```

Add gcli option:

```go
c.StrOpt(&opts.Token, "token", "", "", "Bearer token required for cache downloads and manifest")
```

Update `commandFlagSpecs["cache"].subs["serve"].values` to include `"token"`.

Update `serveOptionsFromCLI` to pass `Token`.

Add CLI bind test:

```go
err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"cache", "serve", "--token", "secret"})
assert.NoErr(t, err)
assert.Eq(t, "secret", calls[0].options.(*CacheServeOptions).Token)
```

- [x] **Step 5: 运行 token 测试**

```bash
go test ./internal/app/cache -run "Token|Healthz|Manifest"
go test ./internal/cli -run "CacheServe"
```

Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add internal/app/cache/model.go internal/app/cache/auth_log.go internal/app/cache/auth_log_test.go internal/app/cache/server.go internal/cli/cache_cmd.go internal/cli/app.go internal/cli/cache_handler.go internal/cli/app_cache_test.go
git commit -m "feat(cache): add bearer token for cache serve"
```

## Task 4: cache serve JSON request log

**Files:**
- Modify: `internal/app/cache/auth_log.go`
- Modify: `internal/app/cache/auth_log_test.go`
- Modify: `internal/app/cache/server.go`
- Modify: `internal/cli/cache_cmd.go`
- Modify: `internal/cli/app.go`
- Modify: `internal/cli/cache_handler.go`
- Modify: `internal/cli/app_cache_test.go`

- [ ] **Step 1: impact analysis**

```bash
npx gitnexus impact --repo eget ServeOptions
npx gitnexus impact --repo eget NewHandler
npx gitnexus impact --repo eget CacheServeOptions
```

Expected: cache server and CLI cache serve tests. Report if HIGH/CRITICAL.

- [ ] **Step 2: 写 json-log 失败测试**

Append to `internal/app/cache/auth_log_test.go`:

```go
func TestCacheServerJSONLogWritesOneLineWithoutQueryOrToken(t *testing.T) {
	var log bytes.Buffer
	handler := NewHandler(Service{}, t.TempDir(), ServeOptions{
		Token:     "secret",
		JSONLog:   true,
		LogWriter: &log,
	})
	req := httptest.NewRequest(http.MethodGet, "/manifest.json?token=bad", nil)
	req.Header.Set("Authorization", "Bearer secret")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	var event map[string]any
	assert.NoErr(t, json.Unmarshal(bytes.TrimSpace(log.Bytes()), &event))
	assert.Eq(t, "/manifest.json", event["path"])
	assert.Eq(t, float64(200), event["status"])
	assert.False(t, strings.Contains(log.String(), "secret"))
	assert.False(t, strings.Contains(log.String(), "token=bad"))
}
```

Note: this requires adding `LogWriter io.Writer` to `ServeOptions`. If keeping writer out of public options is preferred, create `NewHandlerWithLogWriter` only for tests; prefer `LogWriter` for minimal plumbing.

Run:

```bash
go test ./internal/app/cache -run "JSONLog"
```

Expected: FAIL because json-log is not implemented.

- [ ] **Step 3: 实现 json-log middleware**

Modify `ServeOptions`:

```go
JSONLog   bool
LogWriter io.Writer `json:"-"`
```

In `NewHandler`:

```go
handler := http.Handler(cacheHandler{...})
if opts.JSONLog {
	writer := opts.LogWriter
	if writer == nil {
		writer = os.Stderr
	}
	handler = withJSONLog(handler, writer, service.now)
}
handler = withBearerToken(handler, opts.Token)
return handler
```

Implementation notes:

- Wrap auth outside or inside log deliberately. Recommended: log outside auth so unauthorized requests are visible.
- If log outside auth, order should be:

```go
handler = withBearerToken(handler, opts.Token)
if opts.JSONLog { handler = withJSONLog(handler, writer, service.now) }
```

- `statusRecorder` should default status to `200` if handler writes body without `WriteHeader`.
- `bytes` counts response bytes.
- `path` uses `r.URL.Path`, not `RequestURI`, so query string is excluded.

- [ ] **Step 4: 绑定 CLI json-log flag**

Modify `CacheServeOptions`:

```go
JSONLog bool
```

Add gcli option:

```go
c.BoolOpt(&opts.JSONLog, "json-log", "", false, "Write one JSON request log line per cache server request")
```

Update command flag spec:

```go
"serve": {
	bools:  setOf("no-index", "json-log"),
	values: setOf("host", "port", "p", "root", "token"),
},
```

Update `serveOptionsFromCLI`:

```go
JSONLog: opts.JSONLog,
LogWriter: s.stderrWriter(), // set in handleCacheServe after conversion if needed
```

If `serveOptionsFromCLI` cannot access `s.stderrWriter()`, set `serveOpts.LogWriter = s.stderrWriter()` in `handleCacheServe`.

- [ ] **Step 5: 运行 json-log 测试**

```bash
go test ./internal/app/cache -run "JSONLog|Token"
go test ./internal/cli -run "CacheServe"
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/cache/model.go internal/app/cache/auth_log.go internal/app/cache/auth_log_test.go internal/app/cache/server.go internal/cli/cache_cmd.go internal/cli/app.go internal/cli/cache_handler.go internal/cli/app_cache_test.go
git commit -m "feat(cache): add json request logs for cache serve"
```

## Task 5: 文档和 TODO

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/config.md`
- Modify: `docs/config.zh-CN.md`
- Modify: `docs/TODO.md`
- Modify: `docs/superpowers/specs/2026-05-26-cache-management-design.md`

- [ ] **Step 1: 更新 README cache management**

Add examples near existing cache section:

```markdown
Inspect cache contents:

```bash
eget cache status
eget cache list --root sdk
eget cache clean --dry-run --json
```

Serve cache with simple bearer protection and JSON request logs:

```bash
eget cache serve --host 0.0.0.0 --port 8686 --token "$EGET_CACHE_TOKEN" --json-log
```
```

Add equivalent Chinese text to `README.zh-CN.md`.

- [ ] **Step 2: 更新 config docs**

In `docs/config.md` and `docs/config.zh-CN.md`, clarify:

- `[cache_mirror]` is client-side mirror lookup config.
- Server token is a runtime CLI flag:

```bash
eget cache serve --token "$EGET_CACHE_TOKEN"
```

- Do not put bearer token in `[cache_mirror]`; clients send it only if future client token support is added. This plan protects browser/manual/API access to server endpoints, not authenticated client mirror downloads.

If implementation adds client token support later, document it in a separate plan.

- [ ] **Step 3: 更新 TODO**

Update `docs/TODO.md`:

```markdown
- [ ] 增强 cache mirror 自动复用能力。
  - [x] `cache serve` 增加 path-key 下载协议，基于缓存相对路径 md5 复用现有老缓存。
  - [x] 客户端 install/download/sdk install 在回源前尝试使用局域网 cache mirror。
  - [ ] 后续 registry 化阶段再设计 source metadata、搜索和不依赖第三方 provider 的解析能力。
  - [x] `cache serve --token`。
  - [ ] manifest TTL。
- [ ] cache 运维和脚本集成增强。
  - [x] `cache list` 和 `cache status`。
  - [x] `cache clean --json` 和 `cache serve --json-log`。
```

- [ ] **Step 4: 链接计划文档到设计文档**

In `docs/superpowers/specs/2026-05-26-cache-management-design.md`, under Phase 5, add:

```markdown
实施计划：

- [2026-06-02 Cache Ops Integration Enhancements](../plans/2026-06-02-cache-ops-integration.md)
```

- [ ] **Step 5: Commit**

```bash
git add README.md README.zh-CN.md docs/config.md docs/config.zh-CN.md docs/TODO.md docs/superpowers/specs/2026-05-26-cache-management-design.md
git commit -m "docs(cache): document cache ops integration enhancements"
```

## Task 6: 最终验证

**Files:**
- Modify: `docs/superpowers/plans/2026-06-02-cache-ops-integration.md`

- [ ] **Step 1: 运行局部测试**

```bash
go test ./internal/app/cache ./internal/cli
```

Expected: PASS.

- [ ] **Step 2: 运行全量测试**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: 手动验证 CLI JSON**

Use a temporary config dir/cache dir:

```bash
eget cache status --json
eget cache list --json
eget cache clean --dry-run --json
```

Expected:

- All commands print valid JSON.
- `cache list --json` contains `files`.
- `cache status --json` contains `kinds` and `cache_mirror`.
- dry-run does not remove files.

- [ ] **Step 4: 手动验证 token 和 json-log**

Start server:

```bash
eget cache serve --host 127.0.0.1 --port 18686 --token secret --json-log
```

Verify:

```bash
curl -i http://127.0.0.1:18686/healthz
curl -i http://127.0.0.1:18686/manifest.json
curl -i -H "Authorization: Bearer secret" http://127.0.0.1:18686/manifest.json
```

Expected:

- `/healthz` returns 200 without token.
- `/manifest.json` returns 401 without token.
- `/manifest.json` returns 200 with correct token.
- stderr receives one JSON log line per request.
- JSON log line does not include query string or Authorization value.

- [ ] **Step 5: detect changes**

```bash
npx gitnexus detect-changes --repo eget
```

Expected: affected symbols limited to cache service/reporting, cache server options/middleware, CLI cache commands/handlers, docs.

- [ ] **Step 6: Commit plan checkbox updates**

```bash
git add docs/superpowers/plans/2026-06-02-cache-ops-integration.md
git commit -m "docs(cache): update cache ops integration plan progress"
```

## Final Verification Checklist

- [ ] `cache list` lists cache files and supports root filtering.
- [ ] `cache list --json` outputs valid JSON with `path_key`.
- [ ] `cache status` summarizes cache dir, totals, kind summaries, mirror config, and serve suggestion.
- [ ] `cache status --json` outputs valid JSON.
- [ ] `cache clean --dry-run --json` outputs preview JSON and removes nothing.
- [ ] `cache clean --json --yes` outputs result JSON after deletion.
- [ ] `cache serve --token` protects `/`, `/manifest.json`, `/files/*`, `/download/*`.
- [ ] `cache serve --token` leaves `/healthz` accessible without token.
- [ ] `cache serve --json-log` writes one JSON line per request to stderr.
- [ ] JSON logs exclude query string, Authorization header, and token values.
- [ ] README/config docs explain A+B usage.
- [ ] Design doc Phase 5 links to this implementation plan.
- [ ] `docs/TODO.md` reflects completed and remaining Phase 5 work.
- [ ] `go test ./...` passes.
- [ ] `npx gitnexus detect-changes --repo eget` has been reviewed.
