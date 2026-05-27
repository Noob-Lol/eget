# Cache Management Phase 1+2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现设计文档 Phase 1+2 的 `eget cache` 能力：`cache clean` 清理本机缓存，`cache serve` 启动只读内网缓存服务，并从第一期提供稳定的 manifest schema。

**Architecture:** 核心逻辑放在 `internal/app/cache` 子包，CLI 只负责参数解析、调用服务和格式化输出。缓存扫描统一输出 `Entry`，`cache clean` 和 `cache serve /manifest.json` 复用同一套分类、路径安全和 root scope 逻辑，避免后续 mirror 阶段推翻协议。

**Tech Stack:** Go, gookit/gcli, net/http, httptest, `github.com/gookit/goutil/testutil/assert`。

---

## 复审结论

本计划基于设计文档 [2026-05-26-cache-management-design.md](../specs/2026-05-26-cache-management-design.md) 复审后制定。设计文档中“Phase 1”章节只包含 `cache clean`，但文末“推荐首期实施范围”明确建议第一期同时实现 Phase 1 和 Phase 2，因此本计划把第一期定义为：

- `cache clean` MVP。
- `cache serve` 只读服务 MVP。
- manifest 使用 `schema: 1`，但不实现 `/download/{cache-key}`、`--token`、`--manifest-ttl` 和客户端自动 mirror。

实现前需要向用户再次确认，因为本计划落地会修改超过 3 个逻辑文件：`internal/app/cache/model.go`、`internal/app/cache/service.go`、`internal/app/cache/server.go`、`internal/cli/cache_cmd.go`、`internal/cli/handlers.go`、`internal/cli/service.go`、`internal/cli/wiring.go`、`internal/cli/app.go`，以及 README/TODO 文档和测试文件。

## 文件结构

- Create: `internal/app/cache/model.go`
  - 定义 `Kind`、`Entry`、`CleanOptions`、`CleanResult` 和 `ServeOptions`。
- Create: `internal/app/cache/service.go`
  - 缓存根目录解析、危险目录校验、duration 解析、缓存扫描、kind/root scope 过滤、清理候选和删除执行。
- Create: `internal/app/cache/cache_test.go`
  - 覆盖 duration、默认 kind、扫描分类、dry-run、`--all`、`sdk-index` 显式清理、危险目录拒绝和路径边界。
- Create: `internal/app/cache/server.go`
  - 只读 HTTP handler、`/healthz`、`/manifest.json`、`/files/{relpath}`、manifest DTO、路径防逃逸、目录列表开关。
- Create: `internal/app/cache/server_test.go`
  - 使用 `httptest` 覆盖 healthz、manifest、root scope、文件下载、HEAD、Range、目录列表禁用和路径逃逸。
- Create: `internal/cli/cache_cmd.go`
  - 注册顶层 `cache` 命令及 `clean`、`serve` 子命令，定义 CLI options。
- Modify: `internal/cli/app.go`
  - 在 `newApp` 中注册 `newCacheCmd(handler)`。
- Modify: `internal/cli/service.go`
  - 增加 `cacheService appcache.Service` 字段，便于 handler 调用和测试替换。
- Modify: `internal/cli/wiring.go`
  - 构造 `appcache.Service{Config: cfg, Now: time.Now}` 并注入 `cliService`。
- Modify: `internal/cli/handlers.go`
  - 增加 `case "cache.clean"` 和 `case "cache.serve"`，实现输出摘要与 HTTP 服务启动。
- Modify: `internal/cli/app_test.go`
  - 覆盖 CLI 参数绑定：`cache clean`、`cache clean --older 7d --pkg --dry-run`、`cache serve --host 127.0.0.1 --port 0 --root sdk --no-index`。
- Modify: `README.md`
  - 增加 `cache clean`、`cache serve` 简要说明。
- Modify: `README.zh-CN.md`
  - 增加中文说明。
- Modify: `docs/TODO.md`
  - 第一期开发布署完成后勾选或拆分 `cache` 任务。

## 命令范围

第一期必须支持：

```bash
eget cache clean
eget cache clean --dry-run
eget cache clean --older 7d
eget cache clean --all
eget cache clean --yes
eget cache clean --pkg
eget cache clean --api
eget cache clean --sdk
eget cache clean --sdk-index
eget cache clean --partial
eget cache serve
eget cache serve --host 127.0.0.1 --port 0 --root sdk --no-index
```

第一期不暴露：

```bash
eget cache list
eget cache status
eget cache mirror
eget cache serve --token
eget cache serve --manifest-ttl
```

## Task 1: 缓存模型、duration 和路径安全

**Files:**
- Create: `internal/app/cache/model.go`
- Create: `internal/app/cache/service.go`
- Create: `internal/app/cache/cache_test.go`

- [x] **Step 1: 写失败测试：duration 解析**

在 `internal/app/cache/cache_test.go` 增加：

```go
package cache

import (
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestParseOlderDuration(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{"minutes", "30m", 30 * time.Minute},
		{"hours", "12h", 12 * time.Hour},
		{"days", "3d", 72 * time.Hour},
		{"weeks", "1w", 7 * 24 * time.Hour},
		{"go duration", "72h", 72 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOlderDuration(tt.input)
			assert.NoErr(t, err)
			assert.Eq(t, tt.want, got)
		})
	}
}

func TestParseOlderDurationRejectsInvalidInput(t *testing.T) {
	tests := []string{"", "0", "0d", "-1d", "1mo", "abc"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseOlderDuration(input)
			assert.Err(t, err)
		})
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/app/cache -run OlderDuration -v
```

Expected: FAIL，提示 `ParseOlderDuration` 未定义。

- [x] **Step 3: 实现基础类型**

创建 `internal/app/cache/model.go`：

```go
package cache

import (
	"time"
)

type Kind string

const (
	KindPkg      Kind = "pkg"
	KindAPI      Kind = "api"
	KindSDK      Kind = "sdk"
	KindSDKIndex Kind = "sdk-index"
	KindPartial  Kind = "partial"
)

var defaultCacheCleanKinds = []Kind{
	KindPkg,
	KindAPI,
	KindSDK,
	KindPartial,
}

type Entry struct {
	Kind      Kind
	Path      string
	RelPath   string
	Size      int64
	ModTime   time.Time
	IsPartial bool
}

type CleanOptions struct {
	Older  time.Duration
	All    bool
	DryRun bool
	Yes    bool
	Kinds  []Kind
}

type CleanSkip struct {
	Path   string
	Reason string
}

type CleanResult struct {
	CacheDir     string
	MatchedFiles int
	RemovedFiles int
	MatchedSize  int64
	RemovedSize  int64
	Skipped      []CleanSkip
}

func (r CleanResult) NeedsConfirmation() bool {
	return r.MatchedFiles >= 100 || r.MatchedSize >= 1024*1024*1024
}

type ServeOptions struct {
	Host    string
	Port    int
	Root    string
	NoIndex bool
}
```

- [x] **Step 4: 实现 duration 解析**

创建 `internal/app/cache/service.go`：

```go
package cache

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func ParseOlderDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("older duration is required")
	}

	unit := value[len(value)-1]
	if unit == 'd' || unit == 'w' {
		n, err := strconv.Atoi(value[:len(value)-1])
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid older duration %q", value)
		}
		if unit == 'd' {
			return time.Duration(n) * 24 * time.Hour, nil
		}
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}

	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid older duration %q", value)
	}
	return d, nil
}
```

- [x] **Step 5: 运行测试确认通过**

Run:

```bash
go test ./internal/app/cache -run OlderDuration -v
```

Expected: PASS。

- [x] **Step 6: 写失败测试：cache dir 解析和危险目录拒绝**

追加到 `internal/app/cache/cache_test.go`：

```go
func TestServiceResolveCacheDir(t *testing.T) {
	tmp := t.TempDir()
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	service := Service{Config: cfg}

	got, err := service.ResolveCacheDir()

	assert.NoErr(t, err)
	assert.Eq(t, tmp, got)
}

func TestServiceResolveCacheDirUsesDefault(t *testing.T) {
	service := Service{Config: cfgpkg.NewFile()}

	got, err := service.ResolveCacheDir()

	assert.NoErr(t, err)
	assert.Contains(t, got, ".cache")
	assert.Contains(t, got, "eget")
}

func TestServiceRejectsDangerousCacheDir(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{"empty", ""},
		{"root", filepath.VolumeName(filepath.Clean(os.TempDir())) + string(filepath.Separator)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCacheDirForMutation(tt.dir)
			assert.Err(t, err)
		})
	}
}
```

同时在 import 中加入：

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/gookit/goutil/testutil/assert"
)
```

- [x] **Step 7: 运行测试确认失败**

Run:

```bash
go test ./internal/app/cache -run 'ResolveCacheDir|DangerousCacheDir' -v
```

Expected: FAIL，提示 `Service`、`ResolveCacheDir`、`validateCacheDirForMutation` 未定义。

- [x] **Step 8: 实现 cache dir 解析和危险目录校验**

修改 `internal/app/cache/service.go` 的 import，并追加：

```go
import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/util"
)

type Service struct {
	Config *cfgpkg.File
	Now    func() time.Time
}

func (s Service) ResolveCacheDir() (string, error) {
	cacheDir := "~/.cache/eget"
	if s.Config != nil && s.Config.Global.CacheDir != nil && strings.TrimSpace(*s.Config.Global.CacheDir) != "" {
		cacheDir = *s.Config.Global.CacheDir
	}
	expanded, err := util.Expand(cacheDir)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(expanded) == "" {
		return "", fmt.Errorf("cache dir is empty")
	}
	return filepath.Abs(expanded)
}

func validateCacheDirForMutation(cacheDir string) error {
	cacheDir = strings.TrimSpace(cacheDir)
	if cacheDir == "" {
		return fmt.Errorf("cache dir is empty")
	}

	abs, err := filepath.Abs(cacheDir)
	if err != nil {
		return err
	}
	clean := filepath.Clean(abs)
	volumeRoot := filepath.VolumeName(clean) + string(filepath.Separator)
	if clean == filepath.Clean(volumeRoot) {
		return fmt.Errorf("refuse to mutate dangerous cache dir %q", cacheDir)
	}

	home, err := util.Home()
	if err == nil {
		homeAbs, _ := filepath.Abs(home)
		if filepath.Clean(homeAbs) == clean {
			return fmt.Errorf("refuse to mutate home directory %q", cacheDir)
		}
	}

	return nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func ensurePathInDir(root, path string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q is outside cache dir", path)
	}
	return nil
}
```

如果 `os` 尚未使用，先不要引入；实际实现中以 `go test` 编译结果为准删除未使用 import。

- [x] **Step 9: 运行测试确认通过**

Run:

```bash
go test ./internal/app/cache -run 'OlderDuration|ResolveCacheDir|DangerousCacheDir' -v
```

Expected: PASS。

- [x] **Step 10: Commit**

```bash
git add internal/app/cache/model.go internal/app/cache/service.go internal/app/cache/cache_test.go
git commit -m "feat(cache): add cache model and safety helpers"
```

## Task 2: 缓存扫描和 clean 执行

**Files:**
- Modify: `internal/app/cache/service.go`
- Modify: `internal/app/cache/cache_test.go`

- [x] **Step 1: 写失败测试：扫描分类和默认 kind**

追加到 `internal/app/cache/cache_test.go`：

```go
func TestServiceScanClassifiesEntries(t *testing.T) {
	cacheDir := t.TempDir()
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg.zip"), "pkg")
	writeCacheTestFile(t, filepath.Join(cacheDir, "api-cache", "repo.json"), "{}")
	writeCacheTestFile(t, filepath.Join(cacheDir, "sdk-downloads", "go", "1.22.0", "go.zip"), "sdk")
	writeCacheTestFile(t, filepath.Join(cacheDir, "sdk-index", "go.json"), "{}")
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg.zip.part"), "partial")
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg.zip.meta.json"), "{}")

	service := Service{}
	entries, err := service.Scan(cacheDir, CacheScanOptions{Kinds: []Kind{
		KindPkg,
		KindAPI,
		KindSDK,
		KindSDKIndex,
		KindPartial,
	}})

	assert.NoErr(t, err)
	got := map[string]Kind{}
	for _, entry := range entries {
		got[entry.RelPath] = entry.Kind
	}
	assert.Eq(t, KindPkg, got["pkg.zip"])
	assert.Eq(t, KindAPI, got["api-cache/repo.json"])
	assert.Eq(t, KindSDK, got["sdk-downloads/go/1.22.0/go.zip"])
	assert.Eq(t, KindSDKIndex, got["sdk-index/go.json"])
	assert.Eq(t, KindPartial, got["pkg.zip.part"])
	assert.Eq(t, KindPartial, got["pkg.zip.meta.json"])
}

func TestServiceDefaultCleanKindsExcludeSDKIndex(t *testing.T) {
	kinds := normalizeKinds(nil)

	assert.Eq(t, []Kind{KindPkg, KindAPI, KindSDK, KindPartial}, kinds)
}

func writeCacheTestFile(t *testing.T, path, body string) {
	t.Helper()
	assert.NoErr(t, os.MkdirAll(filepath.Dir(path), 0o755))
	assert.NoErr(t, os.WriteFile(path, []byte(body), 0o644))
}
```

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/app/cache -run 'ScanClassifies|DefaultCleanKinds' -v
```

Expected: FAIL，提示 `Scan`、`CacheScanOptions` 或 `normalizeKinds` 未定义。

- [x] **Step 3: 实现扫描、分类和 kind 过滤**

在 `internal/app/cache/service.go` 追加：

```go
type CacheScanOptions struct {
	Kinds []Kind
	Root  string
}

func (s Service) Scan(cacheDir string, opts CacheScanOptions) ([]Entry, error) {
	if cacheDir == "" {
		resolved, err := s.ResolveCacheDir()
		if err != nil {
			return nil, err
		}
		cacheDir = resolved
	}

	selected := cacheKindSet(normalizeScanKinds(opts.Kinds))
	root := strings.TrimSpace(opts.Root)

	var entries []Entry
	err := filepath.WalkDir(cacheDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == cacheDir {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.Mode().IsDir() || !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		if err := ensurePathInDir(cacheDir, path); err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(cacheDir, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		kind, partial := classifyEntry(rel)
		if !selected[kind] {
			return nil
		}
		if root != "" && !cacheRootAllows(root, kind) {
			return nil
		}
		entries = append(entries, Entry{
			Kind:      kind,
			Path:      path,
			RelPath:   rel,
			Size:      info.Size(),
			ModTime:   info.ModTime(),
			IsPartial: partial,
		})
		return nil
	})
	return entries, err
}

func normalizeScanKinds(kinds []Kind) []Kind {
	if len(kinds) == 0 {
		return []Kind{KindPkg, KindAPI, KindSDK, KindSDKIndex, KindPartial}
	}
	return dedupeKinds(kinds)
}

func normalizeKinds(kinds []Kind) []Kind {
	if len(kinds) == 0 {
		return append([]Kind(nil), defaultCacheCleanKinds...)
	}
	return dedupeKinds(kinds)
}

func dedupeKinds(kinds []Kind) []Kind {
	seen := map[Kind]bool{}
	out := make([]Kind, 0, len(kinds))
	for _, kind := range kinds {
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	return out
}

func cacheKindSet(kinds []Kind) map[Kind]bool {
	set := make(map[Kind]bool, len(kinds))
	for _, kind := range kinds {
		set[kind] = true
	}
	return set
}

func classifyEntry(rel string) (Kind, bool) {
	base := path.Base(rel)
	if strings.HasSuffix(base, ".part") || strings.HasSuffix(base, ".meta.json") {
		return KindPartial, true
	}
	switch {
	case strings.HasPrefix(rel, "api-cache/"):
		return KindAPI, false
	case strings.HasPrefix(rel, "sdk-downloads/"):
		return KindSDK, false
	case strings.HasPrefix(rel, "sdk-index/"):
		return KindSDKIndex, false
	default:
		return KindPkg, false
	}
}

func cacheRootAllows(root string, kind Kind) bool {
	switch strings.TrimSpace(root) {
	case "", "all":
		return kind != KindPartial
	case "pkg":
		return kind == KindPkg
	case "api":
		return kind == KindAPI
	case "sdk":
		return kind == KindSDK
	case "sdk-index":
		return kind == KindSDKIndex
	default:
		return false
	}
}

func ValidRoot(root string) bool {
	switch strings.TrimSpace(root) {
	case "", "all", "pkg", "api", "sdk", "sdk-index":
		return true
	default:
		return false
	}
}
```

同时在 import 中加入：

```go
	"os"
	"path"
```

`os` 用于 `os.DirEntry` 和 `os.ModeSymlink`，不要误换成 `io/fs`。

- [x] **Step 4: 运行测试确认通过**

Run:

```bash
go test ./internal/app/cache -run 'ScanClassifies|DefaultCleanKinds' -v
```

Expected: PASS。

- [x] **Step 5: 写失败测试：dry-run、older、all、sdk-index 显式清理**

追加到 `internal/app/cache/cache_test.go`：

```go
func TestServicePreviewCleanDoesNotRemoveFiles(t *testing.T) {
	cacheDir := t.TempDir()
	oldFile := filepath.Join(cacheDir, "old.zip")
	writeCacheTestFile(t, oldFile, "old")
	oldTime := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	assert.NoErr(t, os.Chtimes(oldFile, oldTime, oldTime))

	service := Service{Now: func() time.Time {
		return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	}}
	result, err := service.PreviewClean(cacheDir, CleanOptions{Older: 3 * 24 * time.Hour})

	assert.NoErr(t, err)
	assert.Eq(t, 1, result.MatchedFiles)
	assert.Eq(t, 0, result.RemovedFiles)
	assert.True(t, fileExistsForTest(oldFile))
}

func TestServiceCleanRemovesOnlyOlderMatchedFiles(t *testing.T) {
	cacheDir := t.TempDir()
	oldFile := filepath.Join(cacheDir, "old.zip")
	newFile := filepath.Join(cacheDir, "new.zip")
	writeCacheTestFile(t, oldFile, "old")
	writeCacheTestFile(t, newFile, "new")
	oldTime := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	assert.NoErr(t, os.Chtimes(oldFile, oldTime, oldTime))
	assert.NoErr(t, os.Chtimes(newFile, newTime, newTime))

	service := Service{Now: func() time.Time {
		return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	}}
	result, err := service.Clean(cacheDir, CleanOptions{Older: 3 * 24 * time.Hour})

	assert.NoErr(t, err)
	assert.Eq(t, 1, result.MatchedFiles)
	assert.Eq(t, 1, result.RemovedFiles)
	assert.False(t, fileExistsForTest(oldFile))
	assert.True(t, fileExistsForTest(newFile))
}

func TestServiceCleanAllIgnoresOlder(t *testing.T) {
	cacheDir := t.TempDir()
	file := filepath.Join(cacheDir, "new.zip")
	writeCacheTestFile(t, file, "new")

	service := Service{Now: func() time.Time {
		return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	}}
	result, err := service.Clean(cacheDir, CleanOptions{Older: 3 * 24 * time.Hour, All: true})

	assert.NoErr(t, err)
	assert.Eq(t, 1, result.RemovedFiles)
	assert.False(t, fileExistsForTest(file))
}

func TestServiceCleanDoesNotRemoveSDKIndexByDefault(t *testing.T) {
	cacheDir := t.TempDir()
	indexFile := filepath.Join(cacheDir, "sdk-index", "go.json")
	writeCacheTestFile(t, indexFile, "{}")
	oldTime := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	assert.NoErr(t, os.Chtimes(indexFile, oldTime, oldTime))

	service := Service{Now: func() time.Time {
		return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	}}
	result, err := service.Clean(cacheDir, CleanOptions{Older: 3 * 24 * time.Hour})

	assert.NoErr(t, err)
	assert.Eq(t, 0, result.RemovedFiles)
	assert.True(t, fileExistsForTest(indexFile))
}

func TestServiceCleanRemovesSDKIndexWhenExplicit(t *testing.T) {
	cacheDir := t.TempDir()
	indexFile := filepath.Join(cacheDir, "sdk-index", "go.json")
	writeCacheTestFile(t, indexFile, "{}")

	service := Service{}
	result, err := service.Clean(cacheDir, CleanOptions{All: true, Kinds: []Kind{KindSDKIndex}})

	assert.NoErr(t, err)
	assert.Eq(t, 1, result.RemovedFiles)
	assert.False(t, fileExistsForTest(indexFile))
}

func TestServicePreviewCleanReportsLargeDeletionNeed(t *testing.T) {
	cacheDir := t.TempDir()
	for i := 0; i < 100; i++ {
		writeCacheTestFile(t, filepath.Join(cacheDir, fmt.Sprintf("pkg-%03d.zip", i)), "pkg")
	}

	service := Service{}
	result, err := service.PreviewClean(cacheDir, CleanOptions{All: true})

	assert.NoErr(t, err)
	assert.Eq(t, 100, result.MatchedFiles)
	assert.True(t, result.NeedsConfirmation())
	assert.True(t, fileExistsForTest(filepath.Join(cacheDir, "pkg-000.zip")))
}

func fileExistsForTest(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
```

- [x] **Step 6: 运行测试确认失败**

Run:

```bash
go test ./internal/app/cache -run 'Clean' -v
```

Expected: FAIL，提示 `Clean` 或 `PreviewClean` 未定义。

- [x] **Step 7: 实现 PreviewClean 和 Clean**

在 `internal/app/cache/service.go` 追加：

```go
func (s Service) PreviewClean(cacheDir string, opts CleanOptions) (CleanResult, error) {
	opts.DryRun = true
	return s.clean(cacheDir, opts)
}

func (s Service) Clean(cacheDir string, opts CleanOptions) (CleanResult, error) {
	opts.DryRun = false
	return s.clean(cacheDir, opts)
}

func (s Service) clean(cacheDir string, opts CleanOptions) (CleanResult, error) {
	if cacheDir == "" {
		resolved, err := s.ResolveCacheDir()
		if err != nil {
			return CleanResult{}, err
		}
		cacheDir = resolved
	}
	if err := validateCacheDirForMutation(cacheDir); err != nil {
		return CleanResult{}, err
	}
	if opts.Older == 0 {
		opts.Older = 3 * 24 * time.Hour
	}

	entries, err := s.Scan(cacheDir, CacheScanOptions{Kinds: normalizeKinds(opts.Kinds)})
	if err != nil {
		return CleanResult{}, err
	}

	cutoff := s.now().Add(-opts.Older)
	result := CleanResult{CacheDir: cacheDir}
	for _, entry := range entries {
		if !opts.All && !entry.ModTime.Before(cutoff) {
			continue
		}
		result.MatchedFiles++
		result.MatchedSize += entry.Size
		if opts.DryRun {
			continue
		}
		if err := ensurePathInDir(cacheDir, entry.Path); err != nil {
			result.Skipped = append(result.Skipped, CleanSkip{Path: entry.Path, Reason: err.Error()})
			continue
		}
		if err := os.Remove(entry.Path); err != nil {
			result.Skipped = append(result.Skipped, CleanSkip{Path: entry.Path, Reason: err.Error()})
			continue
		}
		result.RemovedFiles++
		result.RemovedSize += entry.Size
		removeEmptyParents(cacheDir, filepath.Dir(entry.Path))
	}
	return result, nil
}

func removeEmptyParents(root, dir string) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return
	}
	for {
		dirAbs, err := filepath.Abs(dir)
		if err != nil || filepath.Clean(dirAbs) == filepath.Clean(rootAbs) {
			return
		}
		if ensurePathInDir(rootAbs, dirAbs) != nil {
			return
		}
		if err := os.Remove(dirAbs); err != nil {
			return
		}
		dir = filepath.Dir(dirAbs)
	}
}
```

- [x] **Step 8: 运行 app 缓存测试**

Run:

```bash
go test ./internal/app/cache -run 'Cache|Clean' -v
```

Expected: PASS。

- [x] **Step 9: Commit**

```bash
git add internal/app/cache/model.go internal/app/cache/service.go internal/app/cache/cache_test.go
git commit -m "feat(cache): implement cache clean service"
```

## Task 3: 只读 HTTP cache server

**Files:**
- Create: `internal/app/cache/server.go`
- Create: `internal/app/cache/server_test.go`
- Modify: `internal/app/cache/model.go`

- [x] **Step 1: 写失败测试：healthz 和 manifest**

创建 `internal/app/cache/server_test.go`：

```go
package cache

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestCacheServerHealthz(t *testing.T) {
	cacheDir := t.TempDir()
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok":true`)
	assert.Contains(t, rec.Body.String(), `"name":"eget-cache"`)
}

func TestCacheServerManifest(t *testing.T) {
	cacheDir := t.TempDir()
	file := filepath.Join(cacheDir, "pkg.zip")
	assert.NoErr(t, os.WriteFile(file, []byte("pkg"), 0o644))
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "pkg.zip.part"), []byte("partial"), 0o644))
	fixed := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	service := Service{Now: func() time.Time { return fixed }}
	handler := NewHandler(service, cacheDir, ServeOptions{})
	req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	var manifest Manifest
	assert.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &manifest))
	assert.Eq(t, 1, manifest.Schema)
	assert.Eq(t, "eget-cache", manifest.Server.Name)
	assert.Eq(t, "", manifest.Cache.Root)
	assert.Eq(t, 1, len(manifest.Files))
	assert.Eq(t, "pkg", manifest.Files[0].Kind)
	assert.Eq(t, "pkg.zip", manifest.Files[0].Path)
	assert.Eq(t, "/files/pkg.zip", manifest.Files[0].URL)
	assert.Eq(t, "http://example.com", manifest.Server.BaseURL)
}
```

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/app/cache -run 'CacheServerHealthz|CacheServerManifest' -v
```

Expected: FAIL，提示 `NewHandler` 和 `Manifest` 未定义。

- [x] **Step 3: 实现 healthz 和 manifest handler**

创建 `internal/app/cache/server.go`：

```go
package cache

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"
)

type Manifest struct {
	Schema int                 `json:"schema"`
	Server ManifestServer `json:"server"`
	Cache  ManifestCache  `json:"cache"`
	Files  []ManifestFile `json:"files"`
}

type ManifestServer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	BaseURL string `json:"base_url"`
}

type ManifestCache struct {
	Root        string    `json:"root"`
	GeneratedAt time.Time `json:"generated_at"`
}

type ManifestFile struct {
	Kind    string    `json:"kind"`
	Path    string    `json:"path"`
	URL     string    `json:"url"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

type cacheHandler struct {
	service  Service
	cacheDir string
	opts     ServeOptions
}

func NewHandler(service Service, cacheDir string, opts ServeOptions) http.Handler {
	if opts.Root == "" {
		opts.Root = "all"
	}
	return cacheHandler{service: service, cacheDir: cacheDir, opts: opts}
}

func (h cacheHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/healthz":
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"name":    "eget-cache",
			"version": h.opts.Version,
		})
	case r.URL.Path == "/manifest.json":
		h.handleManifest(w, r)
	case strings.HasPrefix(r.URL.Path, "/files/"):
		h.handleFile(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h cacheHandler) handleManifest(w http.ResponseWriter, r *http.Request) {
	entries, err := h.service.Scan(h.cacheDir, CacheScanOptions{
		Root:  h.opts.Root,
		Kinds: []Kind{KindPkg, KindAPI, KindSDK, KindSDKIndex},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	files := make([]ManifestFile, 0, len(entries))
	for _, entry := range entries {
		files = append(files, ManifestFile{
			Kind:    string(entry.Kind),
			Path:    entry.RelPath,
			URL:     "/files/" + path.Clean(entry.RelPath),
			Size:    entry.Size,
			ModTime: entry.ModTime,
		})
	}
	manifest := Manifest{
		Schema: 1,
		Server: ManifestServer{
			Name:    "eget-cache",
			Version: h.opts.Version,
			BaseURL: cacheBaseURL(r),
		},
		Cache: ManifestCache{
			Root:        "",
			GeneratedAt: h.service.now(),
		},
		Files: files,
	}
	writeJSON(w, http.StatusOK, manifest)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func cacheBaseURL(r *http.Request) string {
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	if host == "" {
		return ""
	}
	return "http://" + host
}
```

- [x] **Step 4: 扩展 ServeOptions 传入版本号**

在 `internal/app/cache/model.go` 中把 `ServeOptions` 改为：

```go
type ServeOptions struct {
	Host    string
	Port    int
	Root    string
	NoIndex bool
	Version string
}
```

不要从 `internal/app` import `internal/cli`，否则会形成循环依赖；CLI 层在 Task 5 中通过 `BuildInfo().Version` 填充 `ServeOptions.Version`。

`internal/app/cache/server.go` 此时的 import 应为：

```go
import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"
)
```

- [x] **Step 5: 运行测试确认通过**

Run:

```bash
go test ./internal/app/cache -run 'CacheServerHealthz|CacheServerManifest' -v
```

Expected: PASS。

- [x] **Step 6: 写失败测试：文件下载、HEAD、Range、no-index 和路径逃逸**

追加到 `internal/app/cache/server_test.go`：

```go
func TestCacheServerFilesDownloadHeadAndRange(t *testing.T) {
	cacheDir := t.TempDir()
	file := filepath.Join(cacheDir, "sdk-downloads", "go", "1.22.0", "go.zip")
	assert.NoErr(t, os.MkdirAll(filepath.Dir(file), 0o755))
	assert.NoErr(t, os.WriteFile(file, []byte("0123456789"), 0o644))
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})

	getReq := httptest.NewRequest(http.MethodGet, "/files/sdk-downloads/go/1.22.0/go.zip", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	assert.Eq(t, http.StatusOK, getRec.Code)
	assert.Eq(t, "0123456789", getRec.Body.String())

	headReq := httptest.NewRequest(http.MethodHead, "/files/sdk-downloads/go/1.22.0/go.zip", nil)
	headRec := httptest.NewRecorder()
	handler.ServeHTTP(headRec, headReq)
	assert.Eq(t, http.StatusOK, headRec.Code)
	assert.Eq(t, "", headRec.Body.String())

	rangeReq := httptest.NewRequest(http.MethodGet, "/files/sdk-downloads/go/1.22.0/go.zip", nil)
	rangeReq.Header.Set("Range", "bytes=2-5")
	rangeRec := httptest.NewRecorder()
	handler.ServeHTTP(rangeRec, rangeReq)
	assert.Eq(t, http.StatusPartialContent, rangeRec.Code)
	assert.Eq(t, "2345", rangeRec.Body.String())
}

func TestCacheServerRejectsPathEscape(t *testing.T) {
	cacheDir := t.TempDir()
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})
	req := httptest.NewRequest(http.MethodGet, "/files/../secret.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusForbidden, rec.Code)
}

func TestCacheServerNoIndexRejectsDirectoryListing(t *testing.T) {
	cacheDir := t.TempDir()
	assert.NoErr(t, os.MkdirAll(filepath.Join(cacheDir, "sdk-downloads"), 0o755))
	handler := NewHandler(Service{}, cacheDir, ServeOptions{NoIndex: true})
	req := httptest.NewRequest(http.MethodGet, "/files/sdk-downloads/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusForbidden, rec.Code)
}

func TestCacheServerRootScopeFiltersManifest(t *testing.T) {
	cacheDir := t.TempDir()
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "pkg.zip"), []byte("pkg"), 0o644))
	assert.NoErr(t, os.MkdirAll(filepath.Join(cacheDir, "sdk-downloads"), 0o755))
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "sdk-downloads", "go.zip"), []byte("sdk"), 0o644))
	handler := NewHandler(Service{}, cacheDir, ServeOptions{Root: "sdk"})
	req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	var manifest Manifest
	assert.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &manifest))
	assert.Eq(t, 1, len(manifest.Files))
	assert.Eq(t, "sdk", manifest.Files[0].Kind)
}

func TestCacheServerRootScopeRejectsDirectFileOutsideScope(t *testing.T) {
	cacheDir := t.TempDir()
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "pkg.zip"), []byte("pkg"), 0o644))
	handler := NewHandler(Service{}, cacheDir, ServeOptions{Root: "sdk"})
	req := httptest.NewRequest(http.MethodGet, "/files/pkg.zip", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusForbidden, rec.Code)
}

func TestCacheServerRejectsPartialFiles(t *testing.T) {
	cacheDir := t.TempDir()
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "pkg.zip.part"), []byte("partial"), 0o644))
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})
	req := httptest.NewRequest(http.MethodGet, "/files/pkg.zip.part", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusForbidden, rec.Code)
}
```

- [x] **Step 7: 运行测试确认失败**

Run:

```bash
go test ./internal/app/cache -run 'CacheServerFiles|PathEscape|NoIndex|RootScope|PartialFiles' -v
```

Expected: FAIL，`/files` 尚未实现。

- [x] **Step 8: 实现文件服务**

在 `internal/app/cache/server.go` 追加：

```go
func (h cacheHandler) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/files/")
	cleanRel, err := cleanCacheRelPath(rel)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	fullPath := filepath.Join(h.cacheDir, filepath.FromSlash(cleanRel))
	if err := ensurePathInDir(h.cacheDir, fullPath); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	kind, _ := classifyEntry(cleanRel)
	if !cacheRootAllows(h.opts.Root, kind) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if info.IsDir() && h.opts.NoIndex {
		http.Error(w, "directory listing disabled", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, fullPath)
}

func cleanCacheRelPath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("empty path")
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return "", fmt.Errorf("invalid path")
		}
	}
	clean := path.Clean("/" + rel)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", fmt.Errorf("invalid path")
	}
	if strings.Contains(clean, "/../") {
		return "", fmt.Errorf("invalid path")
	}
	return clean, nil
}
```

同时补充 import：

```go
	"fmt"
	"os"
	"path/filepath"
```

- [x] **Step 9: 运行 app 测试**

Run:

```bash
go test ./internal/app/cache -run 'Cache|CacheServer' -v
```

Expected: PASS。

- [x] **Step 10: Commit**

```bash
git add internal/app/cache/model.go internal/app/cache/server.go internal/app/cache/server_test.go
git commit -m "feat(cache): add read-only cache server"
```

## Task 4: CLI 命令注册和参数绑定

**Files:**
- Create: `internal/cli/cache_cmd.go`
- Modify: `internal/cli/app.go`
- Modify: `internal/cli/app_test.go`

- [x] **Step 1: 写失败测试：cache clean 参数绑定**

追加到 `internal/cli/app_test.go`：

```go
func TestMain_CacheCleanBindsOptions(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"cache", "clean", "--older", "7d", "--pkg", "--sdk", "--dry-run", "--yes"})

	assert.NoErr(t, err)
	assert.Eq(t, 1, len(calls))
	assert.Eq(t, "cache.clean", calls[0].name)
	opts, ok := calls[0].options.(*CacheCleanOptions)
	assert.True(t, ok)
	assert.Eq(t, "7d", opts.Older)
	assert.True(t, opts.Pkg)
	assert.True(t, opts.SDK)
	assert.True(t, opts.DryRun)
	assert.True(t, opts.Yes)
}

func TestMain_CacheServeBindsOptions(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"cache", "serve", "--host", "127.0.0.1", "--port", "0", "--root", "sdk", "--no-index"})

	assert.NoErr(t, err)
	assert.Eq(t, 1, len(calls))
	assert.Eq(t, "cache.serve", calls[0].name)
	opts, ok := calls[0].options.(*CacheServeOptions)
	assert.True(t, ok)
	assert.Eq(t, "127.0.0.1", opts.Host)
	assert.Eq(t, 0, opts.Port)
	assert.Eq(t, "sdk", opts.Root)
	assert.True(t, opts.NoIndex)
}

func TestMain_CacheServeRejectsInvalidRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(func(string, any) error {
		t.Fatalf("handler should not run for invalid root")
		return nil
	}, &stdout, &stderr).RunWithArgs([]string{"cache", "serve", "--root", "bad"})

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "invalid cache root")
}
```

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/cli -run 'CacheCleanBinds|CacheServeBinds' -v
```

Expected: FAIL，`cache` 命令或 options 类型未定义。

- [x] **Step 3: 实现 cache_cmd.go**

创建 `internal/cli/cache_cmd.go`：

```go
package cli

import "github.com/gookit/gcli/v3"

type CacheCleanOptions struct {
	Older    string
	All      bool
	DryRun   bool
	Yes      bool
	Pkg      bool
	API      bool
	SDK      bool
	SDKIndex bool
	Partial  bool
}

type CacheServeOptions struct {
	Host    string
	Port    int
	Root    string
	NoIndex bool
}

func newCacheCmd(handler CommandHandler) (*gcli.Command, func()) {
	cleanOpts := &CacheCleanOptions{Older: "3d"}
	serveOpts := &CacheServeOptions{Host: "0.0.0.0", Port: 8686, Root: "all"}
	cmd := gcli.NewCommand("cache", "Manage local eget cache")
	cmd.Help = `<info>Examples</>:
  eget cache clean
  eget cache clean --dry-run --older 7d
  eget cache clean --api --all
  eget cache serve
  eget cache serve --host 127.0.0.1 --port 0 --root sdk`
	cmd.Subs = []*gcli.Command{
		newCacheCleanCmd(cleanOpts, handler),
		newCacheServeCmd(serveOpts, handler),
	}
	return cmd, func() {
		*cleanOpts = CacheCleanOptions{Older: "3d"}
		*serveOpts = CacheServeOptions{Host: "0.0.0.0", Port: 8686, Root: "all"}
	}
}

func newCacheCleanCmd(opts *CacheCleanOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("clean", "Clean local cache files")
	cmd.Config = func(c *gcli.Command) {
		c.StrOpt(&opts.Older, "older", "", "3d", "Remove files older than duration, e.g. 3d, 12h, 1w")
		c.BoolOpt(&opts.All, "all", "a", false, "Ignore older duration and remove all selected cache files")
		c.BoolOpt(&opts.DryRun, "dry-run", "", false, "Print matched files without removing them")
		c.BoolOpt(&opts.Yes, "yes", "y", false, "Skip large deletion confirmation")
		c.BoolOpt(&opts.Pkg, "pkg", "", false, "Select package/download cache")
		c.BoolOpt(&opts.API, "api", "", false, "Select API cache")
		c.BoolOpt(&opts.SDK, "sdk", "", false, "Select SDK download cache")
		c.BoolOpt(&opts.SDKIndex, "sdk-index", "", false, "Select SDK index cache")
		c.BoolOpt(&opts.Partial, "partial", "", false, "Select unfinished download state")
	}
	cmd.Func = func(_ *gcli.Command, args []string) error {
		if err := validateNoFlagArgs(args); err != nil {
			return err
		}
		snapshot := *opts
		return handler("cache.clean", &snapshot)
	}
	return cmd
}

func newCacheServeCmd(opts *CacheServeOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("serve", "Serve local cache files over read-only HTTP")
	cmd.Config = func(c *gcli.Command) {
		c.StrOpt(&opts.Host, "host", "", "0.0.0.0", "Listen host")
		c.IntOpt(&opts.Port, "port", "p", 8686, "Listen port, 0 means random free port")
		c.StrOpt(&opts.Root, "root", "", "all", "Share scope: all, pkg, api, sdk, sdk-index")
		c.BoolOpt(&opts.NoIndex, "no-index", "", false, "Disable directory listing")
	}
	cmd.Func = func(_ *gcli.Command, args []string) error {
		if err := validateNoFlagArgs(args); err != nil {
			return err
		}
		if !isValidCacheRoot(opts.Root) {
			return fmt.Errorf("invalid cache root %q: must be one of all, pkg, api, sdk, sdk-index", opts.Root)
		}
		snapshot := *opts
		return handler("cache.serve", &snapshot)
	}
	return cmd
}

func isValidCacheRoot(root string) bool {
	switch root {
	case "", "all", "pkg", "api", "sdk", "sdk-index":
		return true
	default:
		return false
	}
}
```

同时把 `internal/cli/cache_cmd.go` 的 import 改为：

```go
import (
	"fmt"

	"github.com/gookit/gcli/v3"
)
```

- [x] **Step 4: 注册顶层 cache 命令**

在 `internal/cli/app.go` 的 `newApp` 中，放在 `sdk` 后或 `config` 前：

```go
	app.add(newCacheCmd(handler))
```

- [x] **Step 5: 运行 CLI 绑定测试**

Run:

```bash
go test ./internal/cli -run 'CacheCleanBinds|CacheServeBinds' -v
```

Expected: PASS。

- [x] **Step 6: Commit**

```bash
git add internal/cli/cache_cmd.go internal/cli/app.go internal/cli/app_test.go
git commit -m "feat(cache): register cache cli commands"
```

## Task 5: CLI handler、service wiring 和 serve 启动

**Files:**
- Modify: `internal/cli/service.go`
- Modify: `internal/cli/wiring.go`
- Modify: `internal/cli/handlers.go`

- [x] **Step 1: 扩展 cliService 字段**

在 `internal/cli/service.go` 的 import 中加入 cache 子包别名：

```go
appcache "github.com/inherelab/eget/internal/app/cache"
```

并在 `cliService` 结构体中增加字段：

```go
	cacheService appcache.Service
```

- [x] **Step 2: 注入 Service**

在 `internal/cli/wiring.go` 的 import 中加入：

```go
appcache "github.com/inherelab/eget/internal/app/cache"
```

然后在 return 前创建：

```go
	cacheService := appcache.Service{Config: cfg, Now: time.Now}
```

在 `cliService{}` 字面量里增加：

```go
		cacheService:      cacheService,
```

- [x] **Step 3: 在 handler switch 中分发 cache 命令**

在 `internal/cli/handlers.go` 的 `handle` switch 中加入：

```go
	case "cache.clean":
		opts := options.(*CacheCleanOptions)
		return s.handleCacheClean(opts)
	case "cache.serve":
		opts := options.(*CacheServeOptions)
		return s.handleCacheServe(opts)
```

- [x] **Step 4: 实现 CLI options 转换**

在 `internal/cli/handlers.go` 的 import 中加入：

```go
appcache "github.com/inherelab/eget/internal/app/cache"
```

然后在文件末尾追加：

```go
func cleanOptionsFromCLI(opts *CacheCleanOptions) (appcache.CleanOptions, error) {
	older, err := appcache.ParseOlderDuration(opts.Older)
	if err != nil {
		return appcache.CleanOptions{}, err
	}
	kinds := make([]appcache.Kind, 0, 5)
	if opts.Pkg {
		kinds = append(kinds, appcache.KindPkg)
	}
	if opts.API {
		kinds = append(kinds, appcache.KindAPI)
	}
	if opts.SDK {
		kinds = append(kinds, appcache.KindSDK)
	}
	if opts.SDKIndex {
		kinds = append(kinds, appcache.KindSDKIndex)
	}
	if opts.Partial {
		kinds = append(kinds, appcache.KindPartial)
	}
	return appcache.CleanOptions{
		Older:  older,
		All:    opts.All,
		DryRun: opts.DryRun,
		Yes:    opts.Yes,
		Kinds:  kinds,
	}, nil
}

func serveOptionsFromCLI(opts *CacheServeOptions) appcache.ServeOptions {
	return appcache.ServeOptions{
		Host:    opts.Host,
		Port:    opts.Port,
		Root:    opts.Root,
		NoIndex: opts.NoIndex,
		Version: BuildInfo().Version,
	}
}
```

- [x] **Step 5: 实现 cache clean 输出**

在 `internal/cli/handlers.go` 追加：

```go
func (s *cliService) handleCacheClean(opts *CacheCleanOptions) error {
	cleanOpts, err := cleanOptionsFromCLI(opts)
	if err != nil {
		return err
	}
	preview, err := s.cacheService.PreviewClean("", cleanOpts)
	if err != nil {
		return err
	}
	if cleanOpts.DryRun {
		ccolor.Fprintln(s.stderrWriter(), "Dry run: eget cache clean")
		ccolor.Fprintf(s.stderrWriter(), " - cache dir: %s\n", preview.CacheDir)
		ccolor.Fprintf(s.stderrWriter(), " - matched files: %d\n", preview.MatchedFiles)
		ccolor.Fprintf(s.stderrWriter(), " - matched size: %s\n", formatBytes(preview.MatchedSize))
		return nil
	}
	if preview.NeedsConfirmation() && !opts.Yes {
		if !stdinIsTerminal() {
			return fmt.Errorf("cache clean matched %d files (%s); rerun with --yes to confirm", preview.MatchedFiles, formatBytes(preview.MatchedSize))
		}
		ccolor.Fprintf(s.stderrWriter(), "Cache clean matched %d files (%s)\n", preview.MatchedFiles, formatBytes(preview.MatchedSize))
		ccolor.Fprint(s.stderrWriter(), "Continue? [y/N]: ")
		confirmed, err := promptConfirmDefaultNo()
		if err != nil {
			return err
		}
		if !confirmed {
			return fmt.Errorf("cache clean cancelled")
		}
	}
	result, err := s.cacheService.Clean("", cleanOpts)
	if err != nil {
		return err
	}
	ccolor.Fprintln(s.stderrWriter(), "Cleaned eget cache")
	ccolor.Fprintf(s.stderrWriter(), " - cache dir: %s\n", result.CacheDir)
	ccolor.Fprintf(s.stderrWriter(), " - removed files: %d\n", result.RemovedFiles)
	ccolor.Fprintf(s.stderrWriter(), " - freed size: %s\n", formatBytes(result.RemovedSize))
	ccolor.Fprintf(s.stderrWriter(), " - skipped files: %d\n", len(result.Skipped))
	if len(result.Skipped) > 0 {
		ccolor.Fprintln(s.stderrWriter(), "Skipped:")
		for _, skipped := range result.Skipped {
			ccolor.Fprintf(s.stderrWriter(), " - %s: %s\n", skipped.Path, skipped.Reason)
		}
	}
	return nil
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
```

同时补充 import：

```go
	"os"
	"golang.org/x/term"
```

- [x] **Step 6: 实现 cache serve 启动**

在 `internal/cli/handlers.go` 追加：

```go
func (s *cliService) handleCacheServe(opts *CacheServeOptions) error {
	serveOpts := serveOptionsFromCLI(opts)
	if serveOpts.Host == "" {
		serveOpts.Host = "0.0.0.0"
	}
	if serveOpts.Root == "" {
		serveOpts.Root = "all"
	}
	cacheDir, err := s.cacheService.ResolveCacheDir()
	if err != nil {
		return err
	}
	handler := appcache.NewHandler(s.cacheService, cacheDir, serveOpts)
	addr := fmt.Sprintf("%s:%d", serveOpts.Host, serveOpts.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	actualAddr := listener.Addr().String()
	ccolor.Fprintf(s.stderrWriter(), "Serving eget cache on http://%s\n", actualAddr)
	ccolor.Fprintf(s.stderrWriter(), " - cache dir: %s\n", cacheDir)
	ccolor.Fprintln(s.stderrWriter(), " - read-only mode; do not expose this service to the public internet")

	server := &http.Server{Handler: handler}
	return server.Serve(listener)
}
```

同时补充 import：

```go
	"net"
	"net/http"
```

- [x] **Step 7: 编译 CLI**

Run:

```bash
go test ./internal/cli -run 'CacheCleanBinds|CacheServeBinds' -v
```

Expected: PASS。

- [x] **Step 8: Commit**

```bash
git add internal/cli/service.go internal/cli/wiring.go internal/cli/handlers.go
git commit -m "feat(cache): wire cache command handlers"
```

## Task 6: CLI handler 单测与确认交互边界

**Files:**
- Create: `internal/cli/cache_cmd_test.go`
- Modify: `internal/cli/handlers.go`

- [x] **Step 1: 写失败测试：clean 输出 dry-run 摘要**

创建 `internal/cli/cache_cmd_test.go`：

```go
package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	appcache "github.com/inherelab/eget/internal/app/cache"
	cfgpkg "github.com/inherelab/eget/internal/config"
)

func TestCliServiceHandleCacheCleanDryRun(t *testing.T) {
	tmp := t.TempDir()
	writeCLITestFile(t, filepath.Join(tmp, "old.zip"), "old")
	old := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	assert.NoErr(t, os.Chtimes(filepath.Join(tmp, "old.zip"), old, old))

	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	var stderr bytes.Buffer
	service := &cliService{
		cacheService: appcache.Service{
			Config: cfg,
			Now: func() time.Time {
				return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
			},
		},
		stderr: &stderr,
	}

	err := service.handleCacheClean(&CacheCleanOptions{Older: "3d", DryRun: true})

	assert.NoErr(t, err)
	out := stderr.String()
	assert.Contains(t, out, "Dry run: eget cache clean")
	assert.Contains(t, out, "matched files: 1")
	assert.True(t, fileExistsCLI(filepath.Join(tmp, "old.zip")))
}

func TestCliServiceHandleCacheCleanLargeDeletionRequiresYesInNonTTY(t *testing.T) {
	tmp := t.TempDir()
	for i := 0; i < 100; i++ {
		writeCLITestFile(t, filepath.Join(tmp, fmt.Sprintf("pkg-%03d.zip", i)), "pkg")
	}

	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	service := &cliService{
		cacheService: appcache.Service{Config: cfg},
		stderr:       io.Discard,
	}

	err := service.handleCacheClean(&CacheCleanOptions{Older: "3d", All: true})

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "--yes")
	assert.True(t, fileExistsCLI(filepath.Join(tmp, "pkg-000.zip")))
}

func TestCliServiceHandleCacheCleanLargeDeletionYesSkipsConfirmation(t *testing.T) {
	tmp := t.TempDir()
	for i := 0; i < 100; i++ {
		writeCLITestFile(t, filepath.Join(tmp, fmt.Sprintf("pkg-%03d.zip", i)), "pkg")
	}

	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	var stderr bytes.Buffer
	service := &cliService{
		cacheService: appcache.Service{Config: cfg},
		stderr:       &stderr,
	}

	err := service.handleCacheClean(&CacheCleanOptions{Older: "3d", All: true, Yes: true})

	assert.NoErr(t, err)
	assert.Contains(t, stderr.String(), "removed files: 100")
	assert.False(t, fileExistsCLI(filepath.Join(tmp, "pkg-000.zip")))
}
```

辅助函数：

```go
func writeCLITestFile(t *testing.T, path, body string) {
	t.Helper()
	assert.NoErr(t, os.MkdirAll(filepath.Dir(path), 0o755))
	assert.NoErr(t, os.WriteFile(path, []byte(body), 0o644))
}

func fileExistsCLI(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
```

- [x] **Step 2: 运行测试确认行为**

Run:

```bash
go test ./internal/cli -run HandleCacheCleanDryRun -v
```

Expected: PASS，并确认大批量清理在非 TTY 且未传 `--yes` 时不会删除文件。

- [x] **Step 3: Commit**

```bash
git add internal/cli/cache_cmd_test.go internal/cli/handlers.go
git commit -m "test(cache): cover cache clean cli output"
```

## Task 7: 文档更新

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/TODO.md`

- [x] **Step 1: 更新 README.md**

在命令列表或配置说明附近加入：

```markdown
### Cache Management

`eget cache clean` removes local cache files from `global.cache_dir`. By default it removes package, API, SDK download, and partial download cache files older than 3 days. SDK index cache is kept by default; use `--sdk-index` when you explicitly want to clear it.

```bash
eget cache clean
eget cache clean --dry-run --older 7d
eget cache clean --api --all
```

`eget cache serve` starts a read-only HTTP server for local cache files so machines on the same LAN can browse or download cached packages and SDK archives.

```bash
eget cache serve
eget cache serve --host 127.0.0.1 --port 0 --root sdk --no-index
```
```

- [x] **Step 2: 更新 README.zh-CN.md**

加入：

```markdown
### 缓存管理

`eget cache clean` 用于清理 `global.cache_dir` 下的本机缓存。默认清理 3 天前的 package 下载缓存、API cache、SDK 下载缓存和未完成下载状态。SDK index 默认保留；如果确认要清理 SDK index，请显式使用 `--sdk-index`。

```bash
eget cache clean
eget cache clean --dry-run --older 7d
eget cache clean --api --all
```

`eget cache serve` 会启动只读 HTTP 服务，方便同一局域网内其它机器浏览或下载本机已有的 package/SDK 缓存文件。

```bash
eget cache serve
eget cache serve --host 127.0.0.1 --port 0 --root sdk --no-index
```
```

- [x] **Step 3: 更新 docs/TODO.md**

把原 cache 条目拆分为已完成第一期和后续项：

```markdown
- [x] 新增 eget 缓存管理命令 `cache` 第一期。
  - [x] `cache clean` 清理下载文件、API cache、SDK 下载缓存和未完成下载状态，默认清理 3 天前的缓存。
  - [x] `cache serve` 启动只读内网 server，分享 package/sdk cache 文件。
- [ ] 增强 cache mirror 自动复用能力。
  - [ ] manifest 增加 source URL hash、etag、last_modified 等字段。
  - [ ] 客户端 install/download/sdk install 在回源前尝试使用局域网 cache mirror。
  - [ ] `cache serve --token` 和 manifest TTL。
```

- [x] **Step 4: Commit**

```bash
git add README.md README.zh-CN.md docs/TODO.md
git commit -m "docs(cache): document cache management commands"
```

## Task 8: 全量验证和手动冒烟

**Files:**
- No code changes expected.

- [ ] **Step 1: 运行全量测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 2: 构建 CLI**

Run:

```bash
go build ./cmd/eget
```

Expected: PASS，并在当前目录生成可执行文件或按 Go 默认输出完成。

- [ ] **Step 3: 手动验证 cache clean dry-run**

Run:

```bash
go run ./cmd/eget cache clean --dry-run
```

Expected: 输出包含：

```text
Dry run: eget cache clean
 - cache dir:
 - matched files:
 - matched size:
```

- [ ] **Step 4: 手动验证 cache serve healthz**

启动服务：

```bash
go run ./cmd/eget cache serve --host 127.0.0.1 --port 8686 --no-index
```

另开终端：

```bash
curl http://127.0.0.1:8686/healthz
curl http://127.0.0.1:8686/manifest.json
```

Expected:

```json
{"name":"eget-cache","ok":true,"version":"..."}
```

`manifest.json` 返回 `schema: 1`、`server.name: eget-cache` 和 `files` 数组。

- [ ] **Step 5: 最终 commit 或 amend**

如果 Task 8 发现修复项，单独提交：

```bash
git add .
git commit -m "fix(cache): stabilize cache management mvp"
```

如果没有修复项，不需要空提交。

## 自查

- 覆盖设计文档第一期推荐范围：`cache clean` 和只读 `cache serve`。
- 没有实现客户端自动 mirror，避免触碰普通 package、direct URL、template package、SDK download 等主链路。
- `sdk-index` 默认不清理，只在 `--sdk-index` 显式选择时清理。
- `/manifest.json` 不暴露本机绝对路径。
- `/manifest.json` 填充 `server.base_url`，后续 mirror 客户端不用猜测服务地址。
- `/files/{relpath}` 只允许访问 `cache_dir` 内路径，并支持 `HEAD` 和 Range。
- `cache clean` 大量删除触发确认阈值，非 TTY 下必须显式传 `--yes`。
- 未注册 `cache list/status/mirror` 等不可用入口。
- 未暴露 `--token`、`--manifest-ttl`，避免承诺未实现能力。
- 没有占位项；每个任务都包含测试、实现、验证和 commit 步骤。
