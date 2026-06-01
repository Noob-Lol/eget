# Cache Mirror Path-Key Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 path-key cache mirror：`cache serve` 基于缓存相对路径提供 `/download/path-md5:<hash>`，客户端 `install/download/sdk install` 在回源前优先从局域网 mirror 复用现有老缓存。

**Architecture:** 新增共享 `internal/cachemirror` 子包，集中 path-key 计算、配置默认值和 mirror 下载逻辑；服务端 `internal/app/cache` 只负责把现有扫描结果映射成 path-key 并安全返回文件；普通 install/download 和 SDK download 在本地缓存未命中后调用共享 mirror helper，命中后写入现有 cache path，后续校验、解压、安装流程不变。

**Tech Stack:** Go, net/http, crypto/md5, httptest, gookit config, `github.com/gookit/goutil/testutil/assert`, GitNexus impact/detect-changes。

---

## 范围确认

本计划会修改超过 3 个逻辑文件且触碰 install/download/sdk 主链路。实施前需要再次向用户确认范围；确认后按任务分阶段提交。每个任务完成后更新本计划 checkbox，并执行对应 git commit。

本计划只实现 path-key mirror，不实现 registry 化，不实现 source metadata 搜索，不实现 `cache serve --token` 和 manifest TTL。

## 前置 GitNexus 要求

实施前必须刷新并检查索引：

```bash
npx gitnexus status
```

如果状态 stale：

```bash
npx gitnexus analyze
```

修改任意函数、方法、类型前，必须先运行 impact analysis。至少覆盖这些符号：

```bash
npx gitnexus impact --repo eget ManifestFile
npx gitnexus impact --repo eget cacheHandler.ServeHTTP
npx gitnexus impact --repo eget downloadBody
npx gitnexus impact --repo eget DownloadArchive
npx gitnexus impact --repo eget Options
npx gitnexus impact --repo eget File
```

如果 GitNexus 对方法名解析失败，使用文件路径或更具体 symbol name 重试。若 impact 返回 HIGH 或 CRITICAL，先向用户说明影响面再继续。

提交前必须运行：

```bash
npx gitnexus detect-changes --repo eget
```

## 文件结构

- Create: `internal/cachemirror/key.go`
  - path-key 计算、cache 相对路径规范化、mirror URL 拼接。
- Create: `internal/cachemirror/key_test.go`
  - 覆盖 slash path 规范化、Windows 分隔符、路径越界拒绝、MD5 key 稳定性。
- Create: `internal/cachemirror/options.go`
  - 定义 mirror 配置运行时结构和默认值。
- Create: `internal/cachemirror/download.go`
  - 根据 mirror base URL 和 path-key 下载文件到目标 cache path；404 返回 miss，非 200 返回错误；写入 `.mirror-part` 后原子替换。
- Create: `internal/cachemirror/download_test.go`
  - 覆盖 200 命中、404 miss、超时、非 200 错误、目标目录创建和 part 文件清理。
- Modify: `internal/app/cache/server.go`
  - `ManifestFile` 增加 `path_key`；新增 `/download/path-md5:<hash>` handler。
- Modify: `internal/app/cache/server_test.go`
  - 覆盖 manifest `path_key`、path-key 下载命中、404 miss、root scope、partial 拒绝、symlink escape。
- Modify: `internal/config/model.go`
  - 新增 `[cache_mirror]` 配置结构和 `File.CacheMirror` 字段。
- Modify: `internal/config/gookit.go`
  - decode/encode/cache_mirror；reserved root key；`config set` bool/int/path 值解析覆盖 `fallback`、`timeout`。
- Modify: `internal/config/loader_sections_test.go`
  - 覆盖 `[cache_mirror]` 加载。
- Modify: `internal/config/gookit_test.go` 或现有配置保存测试
  - 覆盖 `[cache_mirror]` 保存。
- Create: `internal/app/cache_mirror_options.go`
  - 将 `[cache_mirror]` 配置转换为运行时 `cachemirror.Options`，供 app 和 cli 复用。
- Modify: `internal/install/options.go`
  - 在 `install.Options` 增加 `CacheMirror cachemirror.Options`。
- Modify: `internal/app/install_resolve.go`
  - 从 config 解析 `[cache_mirror]`，注入普通 install/download 运行选项。
- Modify: `internal/cli/options.go`
  - `applyGlobalNetworkConfig` 同步注入 mirror 配置，保证直接 handler 路径一致。
- Modify: `internal/install/runner_download.go`
  - 本地 cache miss 后、回源前尝试 mirror；命中后继续现有读取和校验路径。
- Modify: `internal/install/runner_download_cache_test.go`
  - 覆盖普通下载 mirror 命中、404 fallback、超时 fallback、fallback=false 报错。
- Modify: `internal/sdk/download.go`
  - `DownloadRequest` 增加 `CacheMirror`；本地完整 SDK cache miss 后、回源前尝试 mirror；命中后写入 SDK meta。
- Modify: `internal/sdk/download_test.go`
  - 覆盖 SDK mirror 命中、404 fallback、fallback=false 报错、命中后 meta 可复用。
- Modify: `internal/sdk/service.go`
  - `Service` 增加 `CacheMirror cachemirror.Options`，`Install` 传入 `DownloadRequest`。
- Modify: `internal/app/sdk.go`
  - 从 config 解析 `[cache_mirror]` 注入 SDK service。
- Modify: `internal/app/sdk_test.go`
  - 覆盖 SDK service 读取 cache mirror 配置。
- Modify: `docs/config.md`
  - 增加 `[cache_mirror]` 配置说明。
- Modify: `docs/config.zh-CN.md`
  - 增加中文配置说明。
- Modify: `README.md`
  - 简要说明自动 cache mirror。
- Modify: `README.zh-CN.md`
  - 简要说明自动 cache mirror。
- Modify: `docs/TODO.md`
  - 任务完成后勾选 path-key 和客户端自动复用条目。

## 配置语义

新增配置块：

```toml
[cache_mirror]
enable = true
url = "http://192.168.1.10:8686"
timeout = 5
fallback = true
```

运行时默认值：

| 字段 | 默认 | 行为 |
| --- | --- | --- |
| `enable` | false | 未启用时完全不请求 mirror |
| `url` | 空 | 为空时视为未启用 |
| `timeout` | 5 | mirror 请求超时秒数 |
| `fallback` | true | mirror miss/error 后回源；false 时返回 mirror 错误 |

`timeout <= 0` 时使用默认 5 秒。`url` 需要 trim 末尾 `/` 后拼接 `/download/<key>`。

## Task 1: 共享 path-key 和 mirror 下载 helper

**Files:**
- Create: `internal/cachemirror/key.go`
- Create: `internal/cachemirror/options.go`
- Create: `internal/cachemirror/download.go`
- Create: `internal/cachemirror/key_test.go`
- Create: `internal/cachemirror/download_test.go`

- [ ] **Step 1: impact analysis**

本任务新增文件，不修改现有 symbol。运行：

```bash
npx gitnexus status
```

Expected: current 或提示 stale；stale 时先运行 `npx gitnexus analyze`。

- [ ] **Step 2: 写 key 失败测试**

在 `internal/cachemirror/key_test.go` 新增测试：

```go
package cachemirror

import (
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestKeyForRelPathNormalizesSlashPath(t *testing.T) {
	got := KeyForRelPath(`pkg-cache\tool-1.2.3-a1b2c3d4.zip`)
	assert.Eq(t, "path-md5:e3bd999bec663dd9ec8612d4f87dc7d4", got)
}

func TestRelPathForCacheFile(t *testing.T) {
	cacheDir := t.TempDir()
	fullPath := filepath.Join(cacheDir, "pkg-cache", "tool.zip")
	got, err := RelPath(cacheDir, fullPath)
	assert.NoErr(t, err)
	assert.Eq(t, "pkg-cache/tool.zip", got)
}

func TestRelPathRejectsOutsideCacheDir(t *testing.T) {
	cacheDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "tool.zip")
	_, err := RelPath(cacheDir, outside)
	assert.Err(t, err)
}

func TestDownloadURLTrimsBaseSlash(t *testing.T) {
	got, err := DownloadURL("http://mirror.local:8686/", "path-md5:abc")
	assert.NoErr(t, err)
	assert.Eq(t, "http://mirror.local:8686/download/path-md5:abc", got)
}
```

Run:

```bash
go test ./internal/cachemirror
```

Expected: FAIL because package/functions do not exist.

- [ ] **Step 3: 实现 key/options 最小代码**

Create `internal/cachemirror/options.go`:

```go
package cachemirror

import (
	"strings"
	"time"
)

const defaultTimeoutSeconds = 5

type Options struct {
	Enable   bool
	URL      string
	Timeout  time.Duration
	Fallback bool
}

func NormalizeOptions(opts Options) Options {
	opts.URL = strings.TrimRight(strings.TrimSpace(opts.URL), "/")
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeoutSeconds * time.Second
	}
	return opts
}

func (opts Options) Active() bool {
	opts = NormalizeOptions(opts)
	return opts.Enable && opts.URL != ""
}
```

Create `internal/cachemirror/key.go`:

```go
package cachemirror

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

const PathMD5Prefix = "path-md5:"

func KeyForRelPath(rel string) string {
	rel = normalizeRelPath(rel)
	sum := md5.Sum([]byte(rel))
	return PathMD5Prefix + hex.EncodeToString(sum[:])
}

func RelPath(cacheDir, fullPath string) (string, error) {
	root, err := filepath.Abs(cacheDir)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cache file %q is outside cache dir %q", fullPath, cacheDir)
	}
	return normalizeRelPath(rel), nil
}

func DownloadURL(baseURL, key string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("cache mirror url is empty")
	}
	if !strings.HasPrefix(key, PathMD5Prefix) {
		return "", fmt.Errorf("unsupported cache mirror key %q", key)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid cache mirror url %q", baseURL)
	}
	return baseURL + "/download/" + key, nil
}

func normalizeRelPath(rel string) string {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	return rel
}
```

Run:

```bash
go test ./internal/cachemirror
```

Expected: PASS for key tests except download helper tests not yet added.

- [ ] **Step 4: 写 mirror 下载失败测试**

Append to `internal/cachemirror/download_test.go`:

```go
package cachemirror

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestDownloadToFileWritesMirrorHit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Eq(t, "/download/path-md5:abc", r.URL.Path)
		_, _ = w.Write([]byte("archive"))
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "pkg-cache", "tool.zip")
	result, err := DownloadToFile(context.Background(), Options{Enable: true, URL: server.URL}, "path-md5:abc", target)

	assert.NoErr(t, err)
	assert.True(t, result.Hit)
	assert.Eq(t, int64(len("archive")), result.Size)
	data, err := os.ReadFile(target)
	assert.NoErr(t, err)
	assert.Eq(t, "archive", string(data))
}

func TestDownloadToFileReturnsMissOn404(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	result, err := DownloadToFile(context.Background(), Options{Enable: true, URL: server.URL}, "path-md5:missing", filepath.Join(t.TempDir(), "tool.zip"))

	assert.NoErr(t, err)
	assert.False(t, result.Hit)
}

func TestDownloadToFileReturnsTimeoutError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer server.Close()

	_, err := DownloadToFile(context.Background(), Options{Enable: true, URL: server.URL, Timeout: time.Millisecond}, "path-md5:abc", filepath.Join(t.TempDir(), "tool.zip"))

	assert.Err(t, err)
}
```

Run:

```bash
go test ./internal/cachemirror
```

Expected: FAIL because `DownloadToFile` is missing.

- [ ] **Step 5: 实现 mirror 下载 helper**

Create `internal/cachemirror/download.go`:

```go
package cachemirror

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type DownloadResult struct {
	Hit  bool
	Size int64
}

func DownloadToFile(ctx context.Context, opts Options, key, target string) (DownloadResult, error) {
	opts = NormalizeOptions(opts)
	if !opts.Active() {
		return DownloadResult{}, nil
	}
	downloadURL, err := DownloadURL(opts.URL, key)
	if err != nil {
		return DownloadResult{}, err
	}
	client := &http.Client{Timeout: opts.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return DownloadResult{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return DownloadResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return DownloadResult{Hit: false}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return DownloadResult{}, fmt.Errorf("cache mirror download error: %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return DownloadResult{}, err
	}
	partPath := target + ".mirror-part"
	out, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return DownloadResult{}, err
	}
	size, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(partPath)
		return DownloadResult{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(partPath)
		return DownloadResult{}, closeErr
	}
	if err := os.Rename(partPath, target); err != nil {
		_ = os.Remove(partPath)
		return DownloadResult{}, err
	}
	return DownloadResult{Hit: true, Size: size}, nil
}
```

Run:

```bash
go test ./internal/cachemirror
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cachemirror
git commit -m "feat(cache): add path-key mirror helpers"
```

## Task 2: cache serve path-key manifest 和下载端点

**Files:**
- Modify: `internal/app/cache/server.go`
- Modify: `internal/app/cache/server_test.go`

- [ ] **Step 1: impact analysis**

```bash
npx gitnexus impact --repo eget ManifestFile
npx gitnexus impact --repo eget cacheHandler.ServeHTTP
```

Expected: report direct callers/tests; risk should be MEDIUM or lower. If HIGH/CRITICAL, pause and report.

- [ ] **Step 2: 写 manifest path_key 测试**

Modify `TestCacheServerManifest` in `internal/app/cache/server_test.go` to assert:

```go
assert.Eq(t, "path-md5:7d666be70f6586be664607040ebc2977", manifest.Files[0].PathKey)
```

The expected key is `md5("pkg.zip")`.

Run:

```bash
go test ./internal/app/cache -run TestCacheServerManifest
```

Expected: FAIL because `PathKey` is not present.

- [ ] **Step 3: 增加 manifest path_key**

Modify `internal/app/cache/server.go`:

```go
import "github.com/inherelab/eget/internal/cachemirror"

type ManifestFile struct {
	Kind    string    `json:"kind"`
	Path    string    `json:"path"`
	PathKey string    `json:"path_key,omitempty"`
	URL     string    `json:"url"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}
```

In `handleManifest`, set:

```go
PathKey: cachemirror.KeyForRelPath(entry.RelPath),
```

Run:

```bash
go test ./internal/app/cache -run TestCacheServerManifest
```

Expected: PASS.

- [ ] **Step 4: 写 /download path-key 测试**

Append tests to `internal/app/cache/server_test.go`:

```go
func TestCacheServerDownloadPathKey(t *testing.T) {
	cacheDir := t.TempDir()
	file := filepath.Join(cacheDir, "pkg-cache", "tool.zip")
	assert.NoErr(t, os.MkdirAll(filepath.Dir(file), 0o755))
	assert.NoErr(t, os.WriteFile(file, []byte("pkg"), 0o644))
	key := "path-md5:4daf7ce3cfcade0f558f3d7b7cd38227" // md5("pkg-cache/tool.zip")
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})

	req := httptest.NewRequest(http.MethodGet, "/download/"+key, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	assert.Eq(t, "pkg", rec.Body.String())
}

func TestCacheServerDownloadPathKeyMiss(t *testing.T) {
	cacheDir := t.TempDir()
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})

	req := httptest.NewRequest(http.MethodGet, "/download/path-md5:missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusNotFound, rec.Code)
}
```

Run:

```bash
go test ./internal/app/cache -run "TestCacheServerDownloadPathKey"
```

Expected: FAIL because `/download` is not implemented.

- [ ] **Step 5: 实现 /download handler**

Modify `ServeHTTP`:

```go
case strings.HasPrefix(r.URL.Path, "/download/"):
	h.handleDownload(w, r)
```

Add method in `internal/app/cache/server.go`:

```go
func (h cacheHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/download/")
	if !strings.HasPrefix(key, cachemirror.PathMD5Prefix) {
		http.NotFound(w, r)
		return
	}
	entries, err := h.service.Scan(h.cacheDir, CacheScanOptions{
		Root:  h.opts.Root,
		Kinds: []Kind{KindPkg, KindAPI, KindSDK, KindSDKIndex},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, entry := range entries {
		if cachemirror.KeyForRelPath(entry.RelPath) != key {
			continue
		}
		if !pathStaysInDirAfterSymlinks(h.cacheDir, entry.Path) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.ServeFile(w, r, entry.Path)
		return
	}
	http.NotFound(w, r)
}
```

Run:

```bash
go test ./internal/app/cache
```

Expected: PASS.

- [ ] **Step 6: 增加安全测试**

Add tests:

```go
func TestCacheServerDownloadPathKeyRespectsRootScope(t *testing.T) {
	cacheDir := t.TempDir()
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "pkg.zip"), []byte("pkg"), 0o644))
	key := "path-md5:7d666be70f6586be664607040ebc2977" // md5("pkg.zip")
	handler := NewHandler(Service{}, cacheDir, ServeOptions{Root: "sdk"})

	req := httptest.NewRequest(http.MethodGet, "/download/"+key, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusNotFound, rec.Code)
}

func TestCacheServerDownloadPathKeyRejectsPartial(t *testing.T) {
	cacheDir := t.TempDir()
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "pkg.zip.part"), []byte("partial"), 0o644))
	key := "path-md5:97e0f4918b1dd360ef9dc653f989080e" // md5("pkg.zip.part")
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})

	req := httptest.NewRequest(http.MethodGet, "/download/"+key, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusNotFound, rec.Code)
}
```

Run:

```bash
go test ./internal/app/cache
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/app/cache/server.go internal/app/cache/server_test.go
git commit -m "feat(cache): serve cache files by path key"
```

## Task 3: 配置读取和运行时选项传递

**Files:**
- Modify: `internal/config/model.go`
- Modify: `internal/config/gookit.go`
- Modify: `internal/config/loader_sections_test.go`
- Modify: `internal/install/options.go`
- Modify: `internal/app/install_resolve.go`
- Modify: `internal/cli/options.go`
- Modify: `internal/sdk/service.go`
- Modify: `internal/app/sdk.go`
- Modify: `internal/app/sdk_test.go`

- [ ] **Step 1: impact analysis**

```bash
npx gitnexus impact --repo eget File
npx gitnexus impact --repo eget Options
npx gitnexus impact --repo eget resolveInstallOptionsWithConfig
npx gitnexus impact --repo eget NewDefaultSDKService
```

Expected: config and install/sdk option construction callers; risk likely MEDIUM. Report if higher.

- [ ] **Step 2: 写 config 加载测试**

Append to `internal/config/loader_sections_test.go`:

```go
func TestLoadFileSupportsCacheMirrorSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eget.toml")
	assert.NoErr(t, os.WriteFile(path, []byte(`
[cache_mirror]
enable = true
url = "http://mirror.local:8686"
timeout = 3
fallback = false
`), 0o644))

	cfg, err := LoadFile(path)

	assert.NoErr(t, err)
	assert.True(t, cfg.CacheMirror.Enable != nil && *cfg.CacheMirror.Enable)
	assert.Eq(t, "http://mirror.local:8686", *cfg.CacheMirror.URL)
	assert.Eq(t, 3, *cfg.CacheMirror.Timeout)
	assert.True(t, cfg.CacheMirror.Fallback != nil && !*cfg.CacheMirror.Fallback)
}
```

Run:

```bash
go test ./internal/config -run TestLoadFileSupportsCacheMirrorSection
```

Expected: FAIL because `CacheMirror` does not exist.

- [ ] **Step 3: 实现配置模型和 decode/encode**

Modify `internal/config/model.go`:

```go
type CacheMirrorSection struct {
	Enable   *bool   `toml:"enable" mapstructure:"enable"`
	URL      *string `toml:"url" mapstructure:"url"`
	Timeout  *int    `toml:"timeout" mapstructure:"timeout"`
	Fallback *bool   `toml:"fallback" mapstructure:"fallback"`
}

type File struct {
	Meta struct {
		Keys []string
	}
	Global      Section            `toml:"global" mapstructure:"global"`
	ApiCache    APICacheSection    `toml:"api_cache" mapstructure:"api_cache"`
	Ghproxy     GhproxySection     `toml:"ghproxy" mapstructure:"ghproxy"`
	CacheMirror CacheMirrorSection `toml:"cache_mirror" mapstructure:"cache_mirror"`
	Repos       map[string]Section
	Packages    map[string]Section    `toml:"packages" mapstructure:"packages"`
	SDK         map[string]SDKSection `toml:"sdk" mapstructure:"sdk"`
}
```

Modify `internal/config/gookit.go`:

```go
if err := cfg.MapOnExists("cache_mirror", &conf.CacheMirror); err != nil {
	return nil, err
}
```

Add to encoded data:

```go
"cache_mirror": cacheMirrorToMap(file.CacheMirror),
```

Add reserved key:

```go
case "global", "api_cache", "ghproxy", "cache_mirror", "packages", "sdk":
```

Add map helper:

```go
func cacheMirrorToMap(section CacheMirrorSection) map[string]any {
	data := map[string]any{}
	if section.Enable != nil {
		data["enable"] = *section.Enable
	}
	if section.URL != nil {
		data["url"] = *section.URL
	}
	if section.Timeout != nil {
		data["timeout"] = *section.Timeout
	}
	if section.Fallback != nil {
		data["fallback"] = *section.Fallback
	}
	return data
}
```

Extend `normalizePathValue` switch:

```go
case "extract_all", "is_gui", "download_only", "quiet", "show_hash", "download_source", "upgrade_only", "disable_ssl", "enable", "support_api", "fallback":
```

```go
case "cache_time", "chunk_concurrency", "batch_concurrency", "timeout":
```

Run:

```bash
go test ./internal/config -run TestLoadFileSupportsCacheMirrorSection
```

Expected: PASS.

- [ ] **Step 4: 写 install option 注入测试**

Append to `internal/app/install_config_test.go`:

```go
func TestInstallOptionsIncludeCacheMirrorConfig(t *testing.T) {
	cacheDir := t.TempDir()
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = util.StringPtr(cacheDir)
	cfg.CacheMirror.Enable = util.BoolPtr(true)
	cfg.CacheMirror.URL = util.StringPtr("http://mirror.local:8686/")
	cfg.CacheMirror.Timeout = intPtr(2)
	cfg.CacheMirror.Fallback = util.BoolPtr(false)
	svc := Service{LoadConfig: func() (*cfgpkg.File, error) { return cfg, nil }}

	opts, err := svc.resolveInstallOptions("owner/repo", install.Options{}, false)

	assert.NoErr(t, err)
	assert.True(t, opts.CacheMirror.Enable)
	assert.Eq(t, "http://mirror.local:8686", opts.CacheMirror.URL)
	assert.Eq(t, 2*time.Second, opts.CacheMirror.Timeout)
	assert.False(t, opts.CacheMirror.Fallback)
}
```

Add this local helper in the same test file if it does not already exist:

```go
func intPtr(v int) *int { return &v }
```

Run:

```bash
go test ./internal/app -run TestInstallOptionsIncludeCacheMirrorConfig
```

Expected: FAIL because install options do not include mirror config.

- [ ] **Step 5: 实现 install/sdk option 注入**

Modify `internal/install/options.go` imports and struct:

```go
import "github.com/inherelab/eget/internal/cachemirror"

type Options struct {
	...
	CacheMirror cachemirror.Options
	...
}
```

Create `internal/app/cache_mirror_options.go`:

```go
package app

import (
	"time"

	"github.com/inherelab/eget/internal/cachemirror"
	cfgpkg "github.com/inherelab/eget/internal/config"
)

func CacheMirrorOptionsFromConfig(cfg *cfgpkg.File) cachemirror.Options {
	if cfg == nil {
		return cachemirror.NormalizeOptions(cachemirror.Options{Fallback: true})
	}
	opts := cachemirror.Options{Fallback: true}
	if cfg.CacheMirror.Enable != nil {
		opts.Enable = *cfg.CacheMirror.Enable
	}
	if cfg.CacheMirror.URL != nil {
		opts.URL = *cfg.CacheMirror.URL
	}
	if cfg.CacheMirror.Timeout != nil {
		opts.Timeout = time.Duration(*cfg.CacheMirror.Timeout) * time.Second
	}
	if cfg.CacheMirror.Fallback != nil {
		opts.Fallback = *cfg.CacheMirror.Fallback
	}
	return cachemirror.NormalizeOptions(opts)
}
```

Set returned install options field:

```go
CacheMirror: CacheMirrorOptionsFromConfig(cfg),
```

Modify `internal/cli/options.go` to set direct CLI option path:

```go
opts.CacheMirror = app.CacheMirrorOptionsFromConfig(cfg)
```

Modify `internal/sdk/service.go`:

```go
CacheMirror cachemirror.Options
```

Modify `internal/app/sdk.go`:

```go
CacheMirror: CacheMirrorOptionsFromConfig(cfg),
```

Modify `internal/sdk/download.go` `DownloadRequest`:

```go
CacheMirror cachemirror.Options
```

Modify `internal/sdk/install_service.go`:

```go
CacheMirror: s.CacheMirror,
```

Run:

```bash
go test ./internal/config ./internal/app ./internal/cli ./internal/sdk
```

Expected: PASS after adjusting imports.

- [ ] **Step 6: Commit**

```bash
git add internal/config internal/install/options.go internal/app/cache_mirror_options.go internal/app/install_resolve.go internal/cli/options.go internal/sdk/service.go internal/app/sdk.go internal/sdk/download.go internal/sdk/install_service.go
git commit -m "feat(config): add cache mirror options"
```

## Task 4: 普通 install/download 客户端 mirror 尝试

**Files:**
- Modify: `internal/install/runner_download.go`
- Modify: `internal/install/runner_download_cache_test.go`

- [ ] **Step 1: impact analysis**

```bash
npx gitnexus impact --repo eget downloadBody
```

Expected: install/download tests and run flow. Report if HIGH/CRITICAL.

- [ ] **Step 2: 写普通下载 mirror 命中测试**

Append to `internal/install/runner_download_cache_test.go`:

```go
func TestDownloadBodyUsesCacheMirrorBeforeOrigin(t *testing.T) {
	cacheDir := t.TempDir()
	var originHit bool
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originHit = true
		_, _ = w.Write([]byte("origin"))
	}))
	defer origin.Close()

	cachePath := CacheFilePath(cacheDir, origin.URL+"/tool.zip")
	rel, err := cachemirror.RelPath(cacheDir, cachePath)
	assert.NoErr(t, err)
	expectedPath := "/download/" + cachemirror.KeyForRelPath(rel)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Eq(t, expectedPath, r.URL.Path)
		_, _ = w.Write([]byte("mirror"))
	}))
	defer mirror.Close()

	runner := &InstallRunner{}
	got, err := runner.downloadBody(origin.URL+"/tool.zip", Options{
		CacheDir: cacheDir,
		CacheMirror: cachemirror.Options{Enable: true, URL: mirror.URL, Fallback: true},
	})

	assert.NoErr(t, err)
	assert.Eq(t, "mirror", string(got.Body))
	assert.False(t, originHit)
}
```

Run:

```bash
go test ./internal/install -run TestDownloadBodyUsesCacheMirrorBeforeOrigin
```

Expected: FAIL because mirror is not called.

- [ ] **Step 3: 实现普通下载 mirror 尝试**

Modify `internal/install/runner_download.go` imports:

```go
import (
	"context"
	...
	"github.com/inherelab/eget/internal/cachemirror"
)
```

Add helper:

```go
func tryCacheMirrorDownload(cachePath string, opts Options) (bool, error) {
	if cachePath == "" || !opts.CacheMirror.Active() {
		return false, nil
	}
	rel, err := cachemirror.RelPath(opts.CacheDir, cachePath)
	if err != nil {
		return false, err
	}
	key := cachemirror.KeyForRelPath(rel)
	result, err := cachemirror.DownloadToFile(context.Background(), opts.CacheMirror, key, cachePath)
	if err != nil {
		if opts.CacheMirror.Fallback {
			verbosef("cache mirror failed: %v", err)
			return false, nil
		}
		return false, err
	}
	return result.Hit, nil
}
```

In `downloadBody`, after local cache read miss and before `DownloadFile`:

```go
if hit, err := tryCacheMirrorDownload(cachePath, opts); err != nil {
	return downloadBodyResult{}, err
} else if hit {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return downloadBodyResult{}, err
	}
	if !isInvalidCachedDownload(cachePath, data) {
		ccolor.Fprintf(output, " - Using cache mirror file <cyan>%s</>\n", filepath.Base(cachePath))
		return downloadBodyResult{Body: data, ModTime: fileModTime(cachePath)}, nil
	}
	if !opts.CacheMirror.Fallback {
		return downloadBodyResult{}, fmt.Errorf("cache mirror returned invalid archive: %s", filepath.Base(cachePath))
	}
	verbosef("discard invalid cache mirror archive: %s", cachePath)
	_ = os.Remove(cachePath)
}
```

Run:

```bash
go test ./internal/install -run "TestDownloadBodyUsesCacheMirrorBeforeOrigin|TestDownloadBody"
```

Expected: PASS.

- [ ] **Step 4: 写 fallback 行为测试**

Add tests:

```go
func TestDownloadBodyFallsBackWhenCacheMirrorMisses(t *testing.T) {
	mirror := httptest.NewServer(http.NotFoundHandler())
	defer mirror.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("origin"))
	}))
	defer origin.Close()

	got, err := (&InstallRunner{}).downloadBody(origin.URL+"/tool.zip", Options{
		CacheDir: t.TempDir(),
		CacheMirror: cachemirror.Options{Enable: true, URL: mirror.URL, Fallback: true},
	})

	assert.NoErr(t, err)
	assert.Eq(t, "origin", string(got.Body))
}

func TestDownloadBodyErrorsWhenCacheMirrorFallbackDisabled(t *testing.T) {
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer mirror.Close()

	_, err := (&InstallRunner{}).downloadBody("https://example.com/tool.zip", Options{
		CacheDir: t.TempDir(),
		CacheMirror: cachemirror.Options{Enable: true, URL: mirror.URL, Fallback: false},
	})

	assert.Err(t, err)
}
```

Run:

```bash
go test ./internal/install -run "CacheMirror|DownloadBody"
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/install/runner_download.go internal/install/runner_download_cache_test.go
git commit -m "feat(install): use cache mirror before origin download"
```

## Task 5: SDK download 客户端 mirror 尝试

**Files:**
- Modify: `internal/sdk/download.go`
- Modify: `internal/sdk/download_test.go`

- [ ] **Step 1: impact analysis**

```bash
npx gitnexus impact --repo eget DownloadArchive
npx gitnexus impact --repo eget DownloadRequest
```

Expected: SDK install and download tests. Report if HIGH/CRITICAL.

- [ ] **Step 2: 写 SDK mirror 命中测试**

Append to `internal/sdk/download_test.go`:

```go
func TestDownloadArchiveUsesCacheMirrorBeforeOrigin(t *testing.T) {
	var originHit bool
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originHit = true
		_, _ = w.Write([]byte("origin"))
	}))
	defer origin.Close()

	cacheDir := t.TempDir()
	req := DownloadRequest{
		URL:      origin.URL + "/go.zip",
		CacheDir: cacheDir,
		SDK:      "go",
		Version:  "1.22.0",
		Filename: "go.zip",
	}
	rel, err := cachemirror.RelPath(cacheDir, sdkDownloadFinalPath(req))
	assert.NoErr(t, err)
	expectedPath := "/download/" + cachemirror.KeyForRelPath(rel)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Eq(t, expectedPath, r.URL.Path)
		_, _ = w.Write([]byte("mirror"))
	}))
	defer mirror.Close()
	req.CacheMirror = cachemirror.Options{Enable: true, URL: mirror.URL, Fallback: true}

	result, err := DownloadArchive(context.Background(), req)

	assert.NoErr(t, err)
	assert.False(t, originHit)
	assert.Eq(t, sdkDownloadFinalPath(req), result.Path)
	assert.True(t, result.FromCache)
	data, err := os.ReadFile(result.Path)
	assert.NoErr(t, err)
	assert.Eq(t, "mirror", string(data))
}
```

Run:

```bash
go test ./internal/sdk -run TestDownloadArchiveUsesCacheMirrorBeforeOrigin
```

Expected: FAIL because SDK mirror is not implemented.

- [ ] **Step 3: 实现 SDK mirror 尝试并写 meta**

Modify `internal/sdk/download.go` imports:

```go
import "github.com/inherelab/eget/internal/cachemirror"
```

Add to `DownloadRequest`:

```go
CacheMirror cachemirror.Options
```

In `DownloadArchive`, after `completeCacheMatches` and before `client.DownloadFile`:

```go
if hit, result, err := downloadArchiveFromMirror(ctx, finalPath, req); err != nil {
	return DownloadResult{}, err
} else if hit {
	meta := downloadMeta{
		Schema:    1,
		URL:       req.URL,
		Filename:  req.Filename,
		Size:      result.Size,
		UpdatedAt: time.Now(),
	}
	if err := saveDownloadMeta(metaPath, meta); err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{Path: finalPath, FromCache: true, Size: result.Size}, nil
}
```

Add helper:

```go
func downloadArchiveFromMirror(ctx context.Context, finalPath string, req DownloadRequest) (bool, cachemirror.DownloadResult, error) {
	if !req.CacheMirror.Active() {
		return false, cachemirror.DownloadResult{}, nil
	}
	rel, err := cachemirror.RelPath(req.CacheDir, finalPath)
	if err != nil {
		return false, cachemirror.DownloadResult{}, err
	}
	key := cachemirror.KeyForRelPath(rel)
	result, err := cachemirror.DownloadToFile(ctx, req.CacheMirror, key, finalPath)
	if err != nil {
		if req.CacheMirror.Fallback {
			return false, cachemirror.DownloadResult{}, nil
		}
		return false, cachemirror.DownloadResult{}, err
	}
	return result.Hit, result, nil
}
```

Run:

```bash
go test ./internal/sdk -run "TestDownloadArchiveUsesCacheMirrorBeforeOrigin|TestDownloadArchiveUsesCompleteCacheWhenMetaMatches"
```

Expected: PASS.

- [ ] **Step 4: 写 SDK fallback 和 meta 复用测试**

Add tests:

```go
func TestDownloadArchiveCacheMirrorMissFallsBack(t *testing.T) {
	mirror := httptest.NewServer(http.NotFoundHandler())
	defer mirror.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("origin"))
	}))
	defer origin.Close()

	req := DownloadRequest{
		URL:         origin.URL + "/go.zip",
		CacheDir:    t.TempDir(),
		SDK:         "go",
		Version:     "1.22.0",
		Filename:    "go.zip",
		CacheMirror: cachemirror.Options{Enable: true, URL: mirror.URL, Fallback: true},
	}

	result, err := DownloadArchive(context.Background(), req)

	assert.NoErr(t, err)
	assert.Eq(t, int64(len("origin")), result.Size)
}

func TestDownloadArchiveCacheMirrorHitWritesReusableMeta(t *testing.T) {
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("mirror"))
	}))
	defer mirror.Close()
	req := DownloadRequest{
		URL:         mirror.URL + "/go.zip",
		CacheDir:    t.TempDir(),
		SDK:         "go",
		Version:     "1.22.0",
		Filename:    "go.zip",
		CacheMirror: cachemirror.Options{Enable: true, URL: mirror.URL, Fallback: true},
	}

	_, err := DownloadArchive(context.Background(), req)
	assert.NoErr(t, err)
	ok, meta := completeCacheMatches(sdkDownloadFinalPath(req), sdkDownloadMetaPath(req), req)
	assert.True(t, ok)
	assert.Eq(t, req.URL, meta.URL)
}
```

Run:

```bash
go test ./internal/sdk -run "CacheMirror|DownloadArchive"
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sdk/download.go internal/sdk/download_test.go internal/sdk/service.go internal/sdk/install_service.go internal/app/sdk.go internal/app/sdk_test.go
git commit -m "feat(sdk): use cache mirror before origin download"
```

## Task 6: 文档、配置说明和端到端验证

**Files:**
- Modify: `docs/config.md`
- Modify: `docs/config.zh-CN.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/TODO.md`

- [ ] **Step 1: 更新配置文档**

In `docs/config.md`, add a section near `api_cache` / `ghproxy`:

````markdown
## Cache Mirror

`[cache_mirror]` lets install/download/sdk install try a LAN `eget cache serve` instance before downloading from the original source.

```toml
[cache_mirror]
enable = true
url = "http://192.168.1.10:8686"
timeout = 5
fallback = true
```

The first mirror protocol uses a path key based on the normalized cache relative path. It can reuse old cache files already present on the mirror server. The mirror is an optimization, not a trust root; checksum verification still uses existing package verification when configured.
````

In `docs/config.zh-CN.md`, add the equivalent Chinese text:

````markdown
## Cache Mirror

`[cache_mirror]` 让 `install/download/sdk install` 在回源下载前先尝试局域网内的 `eget cache serve` 服务。

```toml
[cache_mirror]
enable = true
url = "http://192.168.1.10:8686"
timeout = 5
fallback = true
```

第一版 mirror 协议使用基于缓存相对路径的 path-key，因此可以直接复用 mirror 机器上已有的老缓存文件。mirror 只是下载优化，不是信任根；已有 checksum 配置仍会在后续流程中执行校验。
````

- [ ] **Step 2: 更新 README 简介**

Add short examples to both README files:

````markdown
Start a LAN cache server:

```bash
eget cache serve --host 0.0.0.0 --port 8686
```

Enable a client to try the cache server before origin downloads:

```toml
[cache_mirror]
enable = true
url = "http://192.168.1.10:8686"
```
````

- [ ] **Step 3: 更新 TODO**

In `docs/TODO.md`, mark completed items:

```markdown
- [ ] 增强 cache mirror 自动复用能力。
  - [x] `cache serve` 增加 path-key 下载协议，基于缓存相对路径 md5 复用现有老缓存。
  - [x] 客户端 install/download/sdk install 在回源前尝试使用局域网 cache mirror。
  - [ ] 后续 registry 化阶段再设计 source metadata、搜索和不依赖第三方 provider 的解析能力。
  - [ ] `cache serve --token` 和 manifest TTL。
```

Do not mark the parent complete because registry/token/TTL remain open.

- [ ] **Step 4: 运行局部测试**

```bash
go test ./internal/cachemirror ./internal/app/cache ./internal/config ./internal/install ./internal/sdk ./internal/app ./internal/cli
```

Expected: PASS.

- [ ] **Step 5: 运行主链路全量测试**

Because this changes MVP install/download/sdk paths, run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 6: 手动验证 path-key mirror**

Use two temporary cache dirs:

```bash
go run ./cmd/eget cache serve --host 127.0.0.1 --port 8686 --root all
```

In another shell, use a config with:

```toml
[global]
cache_dir = "D:/tmp/eget-client-cache"

[cache_mirror]
enable = true
url = "http://127.0.0.1:8686"
fallback = true
```

Run a package or direct download whose file already exists in the server cache. Expected: client prints mirror/cache usage in verbose mode, writes the file into its own cache dir, and does not fail if mirror misses.

- [ ] **Step 7: detect changes**

```bash
npx gitnexus detect-changes --repo eget
```

Expected: affected symbols limited to cache mirror helper, cache server, config parsing, install download, SDK download, docs.

- [ ] **Step 8: Commit**

```bash
git add docs/config.md docs/config.zh-CN.md README.md README.zh-CN.md docs/TODO.md
git commit -m "docs(cache): document path-key cache mirror"
```

## Final Verification Checklist

- [ ] `go test ./...` passes.
- [ ] `npx gitnexus detect-changes --repo eget` output matches expected scope.
- [ ] `cache serve /manifest.json` includes `path_key`.
- [ ] `cache serve /download/path-md5:<hash>` returns a matching cache file.
- [ ] Ordinary `install/download` uses mirror before origin when enabled.
- [ ] `sdk install` uses mirror before origin when enabled and writes reusable SDK meta.
- [ ] Mirror 404/timeout falls back when `fallback=true`.
- [ ] Mirror error stops the command when `fallback=false`.
- [ ] Existing checksum behavior remains after mirror hit.
- [ ] `docs/TODO.md` checkboxes reflect completed and remaining cache mirror work.
