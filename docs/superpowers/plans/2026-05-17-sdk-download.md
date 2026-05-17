# SDK Download Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `eget` 增加 SDK 多版本下载、断点续传、解压安装、安装记录和索引缓存管理能力，首版覆盖 Go 和 Node 的可解压 SDK 包。

**Architecture:** 新增 `internal/sdk` 作为 SDK acquisition 核心包，负责 target 解析、配置合并、索引解析、可续传下载、解压安装和 `sdk.installed.json`。`internal/app/sdk.go` 只做配置路径、默认依赖和用例编排包装；`internal/cli/sdk_cmd.go` 使用 `gcli` 真正父子命令注册，不在一级命令中硬解析 `sdk install/list/remove/index`。

**Tech Stack:** Go 1.25、`github.com/gookit/gcli/v3`、`github.com/gookit/goutil/testutil/assert`、`golang.org/x/net/html`、现有 `internal/client` HTTP 能力、现有 `internal/install` archive extractor。

---

## 参考文档

主要设计文档：

```text
docs/superpowers/specs/2026-05-14-sdk-download-design.md
```

CLI 前置设计：

```text
docs/superpowers/specs/2026-05-15-gcli-migration-design.md
docs/superpowers/plans/2026-05-16-gcli-migration.md
```

## MVP 决策

为避免实现时反复扩大范围，本计划对设计文档中的不明确项做如下收敛：

| 项 | MVP 决策 |
| --- | --- |
| 命令范围 | 实现 `sdk install/list/remove/index list/show/refresh/clear` |
| target 格式 | 只支持 `name`、`name@version`、`name:version`，不支持 `go 1.21.1` |
| 多 target | `sdk install` 支持多个 target，逐个安装；首版串行执行 |
| 环境切换 | 不实现 `use/env/script/shim/shell hook`，交给 `kite xenv` |
| `pkg/sdk` | 首版不开放，仅保留 `internal/sdk`，等模型稳定后再包装 |
| HTML index | 首版只解析 `<a href>`，不解析页面脚本内嵌 JSON |
| JSON index | 支持内置 `go-json`、`node-json` 两类 parser |
| filename pattern | 支持通用 `filename_pattern`；`go`、`node` 提供内置默认 pattern |
| exact 版本 | 如果 `url_template` 足够生成 URL，不强制索引中存在该版本 |
| checksum | 首版不做强校验；索引字段可预留，但不阻塞安装 |
| 断点续传 | `.part` + `.meta.json`，续传时使用单连接 Range 追加；新下载可复用现有分片下载能力或单连接落盘 |
| `.part` 校验失败 | 删除 `.part`，保证下次重新下载成功率 |
| `strip_components` | 剥离后路径为空则跳过该目录项；如果最终没有任何文件落盘则报错 |
| 已存在安装目录 | 默认报错；`sdk install --force` 允许先安全删除再安装 |
| `sdk remove` | 必须通过安装记录和路径安全校验；MVP 不加交互确认，不加 `--yes` |
| 并发写 store | 首版不加文件锁；后续如出现并发使用再补 |
| `api_cache.enable=false` | SDK index cache 仍可写入和 `show`；安装时默认刷新远端，失败且有缓存时可降级使用旧缓存 |

## 成功标准

- `eget sdk install go@1.21.1` 可根据配置下载、断点续传、解压到目标目录，并写入 `sdk.installed.json`。
- `eget sdk install go:1.21` 可通过索引选择匹配前缀的最新 patch。
- `eget sdk install go node:20.11.1` 可串行安装多个 SDK target。
- `eget sdk list` 和 `eget sdk list --json` 可读取 `sdk.installed.json`。
- `eget sdk remove go@1.21.1` 只删除安装记录中的安全目录。
- `eget sdk index refresh/show/list/clear` 可管理解析后的索引缓存 JSON。
- Go/Node HTML index 解析有单元测试覆盖。
- 其它 SDK 只要能通过 `target`、`url_template`、`filename_pattern`、`os_map`、`arch_map`、`ext_map` 描述，也可以通过配置使用；默认示例和回归测试优先覆盖 Go/Node。
- SDK 下载断点续传有本地 HTTP server 测试覆盖。
- 现有 `install/download/update/list/config` 行为不变。
- 完成 MVP 主链路后 `go test ./...` 通过。

## 文件结构

新增：

```text
internal/sdk/model.go
internal/sdk/target.go
internal/sdk/target_test.go
internal/sdk/config.go
internal/sdk/config_test.go
internal/sdk/template.go
internal/sdk/template_test.go
internal/sdk/index.go
internal/sdk/index_test.go
internal/sdk/html_index.go
internal/sdk/html_index_test.go
internal/sdk/json_index.go
internal/sdk/json_index_test.go
internal/sdk/download.go
internal/sdk/download_test.go
internal/sdk/store.go
internal/sdk/store_test.go
internal/sdk/service.go
internal/sdk/service_test.go
internal/app/sdk.go
internal/app/sdk_test.go
internal/cli/sdk_cmd.go
```

修改：

```text
internal/config/model.go
internal/config/gookit.go
internal/config/loader.go
internal/config/loader_test.go
internal/config/gookit_test.go
internal/install/archive.go
internal/install/defaults_test.go
internal/cli/app.go
internal/cli/app_test.go
internal/cli/handlers.go
internal/cli/service.go
internal/cli/wiring.go
docs/example.eget.toml
docs/DOCS.md
README.md
README.zh-CN.md
docs/TODO.md
```

不要修改：

```text
internal/installed/*
internal/app/list.go
internal/app/update.go
internal/app/uninstall.go
```

理由：

- SDK installed store 独立于 package installed store，避免污染现有 `list/update/uninstall` 语义。
- SDK app service 独立于现有 package install service，避免把 SDK 多版本模型混入 repo/package 单版本模型。
- 现有下载和解压能力只做可复用扩展，不改变已有 CLI 行为。

## 配置格式

目标配置示例：

```toml
[global]
sdk_target = "~/sdks"
sdk_ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }

[sdk.go]
aliases = ["golang"]
target = "gosdk/go{version}"
url_template = "https://mirrors.aliyun.com/golang/go{version}.{os}-{arch}.{ext}"
index_url = "https://mirrors.aliyun.com/golang/"
index_format = "html"
filename_pattern = "go{version}.{os}-{arch}.{ext}"
strip_components = 1
ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }

[sdk.node]
aliases = ["nodejs"]
target = "nodejs/node{version}"
url_template = "https://cdn.npmmirror.com/binaries/node/v{version}/node-v{version}-{os}-{arch}.{ext}"
index_url = "https://registry.npmmirror.com/binary.html"
index_format = "html"
index_path_prefix = "/binaries/node/"
filename_pattern = "node-v{version}-{os}-{arch}.{ext}"
strip_components = 1
os_map = { windows = "win", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64", 386 = "x86" }
ext_map = { windows = "zip", linux = "tar.xz", darwin = "tar.gz" }
```

## 命令矩阵

| CLI 输入 | 行为 |
| --- | --- |
| `eget sdk` | 显示 sdk 命令帮助，不调用业务 handler |
| `eget sdk install go` | 安装 Go 最新稳定版 |
| `eget sdk install go@latest` | 安装 Go 最新稳定版 |
| `eget sdk install go:latest` | 安装 Go 最新稳定版 |
| `eget sdk install go@1.21` | 从索引选择最新 `1.21.x` |
| `eget sdk install go:1.21` | 从索引选择最新 `1.21.x` |
| `eget sdk install go@1.21.1` | 安装精确版本 |
| `eget sdk install go:1.21.1` | 安装精确版本 |
| `eget sdk install go@1.21.1 node:20.11.1` | 串行安装两个 SDK |
| `eget sdk install --force go@1.21.1` | 目标目录存在时安全删除后重装 |
| `eget sdk list` | 列出所有安装记录 |
| `eget sdk list go` | 只列出 Go 安装记录 |
| `eget sdk list --json` | JSON 输出 |
| `eget sdk remove go@1.21.1` | 删除安装记录中的 Go 版本目录和记录 |
| `eget sdk index list` | 列出索引缓存 |
| `eget sdk index show go` | 输出 Go 规范化索引 JSON |
| `eget sdk index refresh go` | 刷新 Go 索引缓存 |
| `eget sdk index refresh --all` | 刷新所有 SDK 索引缓存 |
| `eget sdk index clear go` | 删除 Go 索引缓存 |
| `eget sdk index clear --all` | 删除全部 SDK 索引缓存 |

## Task 0: Preflight Baseline

**Files:** 无代码修改。

- [x] **Step 1: 确认工作区状态**

Run:

```bash
git status --short
```

Expected:

```text
无未提交改动，或只有明确不属于本任务且不会触碰的文件
```

- [x] **Step 2: 确认 gcli 前置完成**

Run:

```bash
rg "cflag|capp" internal cmd
```

Expected:

```text
无输出
```

- [x] **Step 3: 运行基线测试**

Run:

```bash
go test ./...
```

Expected:

```text
所有 package 通过
```

## Task 1: Config Model For SDK Sections

**Files:**

- Modify: `internal/config/model.go`
- Modify: `internal/config/loader.go`
- Modify: `internal/config/gookit.go`
- Modify: `internal/config/loader_test.go`
- Modify: `internal/config/gookit_test.go`
- Modify: `docs/example.eget.toml`

- [x] **Step 1: 写失败测试，验证 SDK 配置可加载**

在 `internal/config/loader_test.go` 新增测试：

```go
func TestLoadFileReadsSDKSections(t *testing.T) {
    configPath := filepath.Join(t.TempDir(), "eget.toml")
    err := os.WriteFile(configPath, []byte(`
[global]
sdk_target = "~/sdks"
sdk_ext_map = { windows = "zip", linux = "tar.gz" }

[sdk.go]
aliases = ["golang"]
target = "gosdk/go{version}"
url_template = "https://example.com/go{version}.{os}-{arch}.{ext}"
index_url = "https://example.com/golang/"
index_format = "html"
strip_components = 1
ext_map = { windows = "zip", linux = "tar.gz" }
`), 0o644)
    if err != nil {
        t.Fatalf("write config: %v", err)
    }

    cfg, err := LoadFile(configPath)
    if err != nil {
        t.Fatalf("load config: %v", err)
    }

    assert.Eq(t, "~/sdks", util.DerefString(cfg.Global.SDKTarget))
    assert.Eq(t, "tar.gz", cfg.Global.SDKExtMap["linux"])
    got := cfg.SDK["go"]
    assert.Eq(t, []string{"golang"}, got.Aliases)
    assert.Eq(t, "gosdk/go{version}", util.DerefString(got.Target))
    assert.Eq(t, 1, util.DerefInt(got.StripComponents))
}
```

需要新增或复用 test helper；如果 `util.DerefInt` 不存在，测试中直接判断指针。

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/config -run TestLoadFileReadsSDKSections -count=1
```

Expected:

```text
FAIL，File/Section 中没有 SDK 相关字段
```

- [x] **Step 3: 增加配置模型**

在 `internal/config/model.go` 中：

```go
type Section struct {
    // existing fields...
    SDKTarget *string           `toml:"sdk_target" mapstructure:"sdk_target"`
    SDKExtMap map[string]string `toml:"sdk_ext_map" mapstructure:"sdk_ext_map"`
}

type SDKSection struct {
    Aliases         []string          `toml:"aliases" mapstructure:"aliases"`
    Target          *string           `toml:"target" mapstructure:"target"`
    URLTemplate     *string           `toml:"url_template" mapstructure:"url_template"`
    IndexURL        *string           `toml:"index_url" mapstructure:"index_url"`
    IndexFormat     *string           `toml:"index_format" mapstructure:"index_format"`
    IndexParser     *string           `toml:"index_parser" mapstructure:"index_parser"`
    IndexPathPrefix *string           `toml:"index_path_prefix" mapstructure:"index_path_prefix"`
    FilenamePattern *string           `toml:"filename_pattern" mapstructure:"filename_pattern"`
    StripComponents *int              `toml:"strip_components" mapstructure:"strip_components"`
    OSMap           map[string]string `toml:"os_map" mapstructure:"os_map"`
    ArchMap         map[string]string `toml:"arch_map" mapstructure:"arch_map"`
    ExtMap          map[string]string `toml:"ext_map" mapstructure:"ext_map"`
}

type File struct {
    // existing fields...
    SDK map[string]SDKSection `toml:"sdk" mapstructure:"sdk"`
}
```

`NewFile()` 初始化：

```go
cfg.SDK = make(map[string]SDKSection)
```

- [x] **Step 4: 更新 decode/encode reserved keys**

在 `internal/config/gookit.go`：

```go
cfg.MapOnExists("sdk", &conf.SDK)
```

`encodeConfigFile()` 的 data 增加：

```go
"sdk": map[string]any{},
```

并遍历 `file.SDK` 写入 `sdkToMap(section)`。

`isReservedConfigRootKey()` 增加 `"sdk"`。

`normalizePathValue()` 对 map 字段不需要从 CLI set 支持复杂 map；首版只保证 TOML load/save。

- [x] **Step 5: 写 round-trip 测试**

在 `internal/config/gookit_test.go` 增加 save/load round-trip 测试，覆盖：

```text
global.sdk_target
global.sdk_ext_map
sdk.go.aliases
sdk.go.ext_map
sdk.go.strip_components
```

- [x] **Step 6: 运行配置测试**

Run:

```bash
go test ./internal/config
```

Expected:

```text
PASS
```

- [x] **Step 7: 更新示例配置**

在 `docs/example.eget.toml` 增加注释掉的 SDK 配置示例，先放在文件末尾，避免影响现有用户默认配置。

- [x] **Step 8: 提交**

Run:

```bash
git add internal/config/model.go internal/config/loader.go internal/config/gookit.go internal/config/loader_test.go internal/config/gookit_test.go docs/example.eget.toml
git commit -m "feat(sdk): load sdk configuration"
```

## Task 2: SDK Target Parser And Template Resolver

**Files:**

- Create: `internal/sdk/model.go`
- Create: `internal/sdk/target.go`
- Create: `internal/sdk/target_test.go`
- Create: `internal/sdk/config.go`
- Create: `internal/sdk/config_test.go`
- Create: `internal/sdk/template.go`
- Create: `internal/sdk/template_test.go`

- [x] **Step 1: 写 target parser 失败测试**

在 `internal/sdk/target_test.go`：

```go
func TestParseTarget(t *testing.T) {
    tests := []struct {
        input   string
        name    string
        version string
        kind    VersionKind
    }{
        {"go", "go", "latest", VersionLatest},
        {"go@latest", "go", "latest", VersionLatest},
        {"go:latest", "go", "latest", VersionLatest},
        {"go@1.21", "go", "1.21", VersionPrefix},
        {"go:1.21", "go", "1.21", VersionPrefix},
        {"go@1.21.1", "go", "1.21.1", VersionExact},
        {"go:1.21.1", "go", "1.21.1", VersionExact},
    }
    for _, tt := range tests {
        t.Run(tt.input, func(t *testing.T) {
            got, err := ParseTarget(tt.input)
            Require(t, assert.Nil(t, err))
            assert.Eq(t, tt.name, got.Name)
            assert.Eq(t, tt.version, got.Version)
            assert.Eq(t, tt.kind, got.Kind)
        })
    }
}

func TestParseTargetRejectsInvalidInput(t *testing.T) {
    for _, input := range []string{"", "go@", "go:", "@1.21.1", ":1.21.1", "go 1.21.1", "go@@1.21"} {
        t.Run(input, func(t *testing.T) {
            _, err := ParseTarget(input)
            assert.Err(t, err)
        })
    }
}
```

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/sdk -run TestParseTarget -count=1
```

Expected:

```text
FAIL，internal/sdk package 或 ParseTarget 不存在
```

- [x] **Step 3: 实现 model 和 parser**

`internal/sdk/model.go`：

```go
type VersionKind string

const (
    VersionLatest VersionKind = "latest"
    VersionExact  VersionKind = "exact"
    VersionPrefix VersionKind = "prefix"
)

type Target struct {
    Raw     string
    Name    string
    Version string
    Kind    VersionKind
}
```

`internal/sdk/target.go`：

```go
func ParseTarget(input string) (Target, error)
```

规则：

- 只允许一个分隔符：`@` 或 `:`。
- 没有分隔符时版本为 `latest`。
- `latest` -> `VersionLatest`。
- `^\d+\.\d+$` -> `VersionPrefix`。
- 其他非空版本 -> `VersionExact`。
- name 不能为空，不能包含空白。

- [x] **Step 4: 写配置合并失败测试**

在 `internal/sdk/config_test.go` 覆盖：

```text
go alias golang 能解析到 go
sdk.ext_map 覆盖 global.sdk_ext_map
os_map/arch_map 转换 runtime GOOS/GOARCH
target 相对路径基于 global.sdk_target
缺少 target 报错
缺少 ext 映射报错
```

- [x] **Step 5: 实现 resolved config**

`internal/sdk/config.go` 定义：

```go
type Config struct {
    Name            string
    Aliases         []string
    TargetTemplate  string
    URLTemplate     string
    IndexURL        string
    IndexFormat     string
    IndexParser     string
    IndexPathPrefix string
    FilenamePattern string
    StripComponents int
    OSMap           map[string]string
    ArchMap         map[string]string
    ExtMap          map[string]string
}

type ResolveConfigOptions struct {
    GOOS   string
    GOARCH string
}

func ResolveConfig(file *config.File, name string, opts ResolveConfigOptions) (Config, error)
```

只解析配置，不访问网络，不创建目录。

- [x] **Step 6: 写模板测试并实现**

`internal/sdk/template_test.go` 覆盖：

```text
Go URL: go{version}.{os}-{arch}.{ext}
Node URL: node-v{version}-{os}-{arch}.{ext}
target: gosdk/go{version}
未知变量保持错误，不静默替换为空
```

`internal/sdk/template.go`：

```go
type TemplateVars struct {
    Name    string
    Version string
    OS      string
    Arch    string
    Ext     string
}

func RenderTemplate(pattern string, vars TemplateVars) (string, error)
```

- [x] **Step 7: 运行 SDK 基础测试**

Run:

```bash
go test ./internal/sdk
```

Expected:

```text
PASS
```

- [ ] **Step 8: 提交**

Run:

```bash
git add internal/sdk/model.go internal/sdk/target.go internal/sdk/target_test.go internal/sdk/config.go internal/sdk/config_test.go internal/sdk/template.go internal/sdk/template_test.go
git commit -m "feat(sdk): parse targets and resolve templates"
```

## Task 3: Index Parsers And Cache Store

**Files:**

- Modify: `internal/sdk/model.go`
- Create: `internal/sdk/html_index.go`
- Create: `internal/sdk/html_index_test.go`
- Create: `internal/sdk/json_index.go`
- Create: `internal/sdk/json_index_test.go`
- Create: `internal/sdk/index.go`
- Create: `internal/sdk/index_test.go`

- [x] **Step 1: 定义规范索引模型**

在 `internal/sdk/model.go` 增加：

```go
type Index struct {
    Schema    int         `json:"schema"`
    SDK       string      `json:"sdk"`
    SourceURL string      `json:"source_url"`
    FetchedAt time.Time   `json:"fetched_at"`
    Items     []IndexItem `json:"items"`
}

type IndexItem struct {
    Version string      `json:"version"`
    Stable  bool        `json:"stable"`
    Files   []IndexFile `json:"files"`
}

type IndexFile struct {
    OS       string `json:"os"`
    Arch     string `json:"arch"`
    Ext      string `json:"ext"`
    URL      string `json:"url"`
    Filename string `json:"filename"`
}
```

- [x] **Step 2: 写 HTML parser 失败测试**

`internal/sdk/html_index_test.go` 使用 fixture 字符串覆盖：

```html
<a href="go1.21.1.windows-amd64.zip">go1.21.1.windows-amd64.zip</a>
<a href="go1.21.1.linux-amd64.tar.gz">go1.21.1.linux-amd64.tar.gz</a>
<a href="/binaries/node/v20.11.1/node-v20.11.1-win-x64.zip">node</a>
<a href="zig-0.12.0-windows-x86_64.zip">zig</a>
```

测试：

```text
相对 URL 转绝对 URL
Go 文件名解析 version/os/arch/ext
Node 文件名解析 version/os/arch/ext
自定义 filename_pattern 解析 version/os/arch/ext
index_path_prefix 过滤无关链接
```

- [x] **Step 3: 实现 HTML parser**

`internal/sdk/html_index.go`：

```go
type HTMLParseOptions struct {
    SDK             string
    SourceURL       string
    IndexPathPrefix string
    FilenamePattern string
    Now             func() time.Time
}

func ParseHTMLIndex(body io.Reader, opts HTMLParseOptions) (Index, error)
```

要求：

- 使用 `golang.org/x/net/html` tokenizer 或 parser。
- 只提取 `<a href>`。
- 用 `url.Parse` 和 base URL resolve 相对链接。
- 如果 `FilenamePattern` 为空，按 SDK name 为 `go`、`node` 提供默认 pattern。
- 如果 `FilenamePattern` 非空，按 pattern 解析文件名，支持 `{version}`、`{os}`、`{arch}`、`{ext}` 四个捕获变量。
- 忽略无法识别的链接，不报错。
- 如果没有任何有效文件，返回清晰错误。

- [x] **Step 4: 写 JSON parser 失败测试**

`internal/sdk/json_index_test.go` 覆盖：

```text
go-json: go.dev/dl/?mode=json 格式
node-json: nodejs.org/dist/index.json 格式
非支持 parser 返回错误
```

- [x] **Step 5: 实现 JSON parser**

`internal/sdk/json_index.go`：

```go
func ParseJSONIndex(body io.Reader, parser string, opts JSONParseOptions) (Index, error)
```

只支持：

```text
go-json
node-json
```

- [x] **Step 6: 写 index cache 和版本选择测试**

`internal/sdk/index_test.go` 覆盖：

```text
SaveIndex/LoadIndex round-trip
ListCachedIndexes 返回 sdk、item count、source URL、fetched_at
SelectVersion latest 选择最大稳定版本
SelectVersion prefix 选择最大 patch
SelectVersion exact 找不到时报错
SelectFile 匹配 os/arch/ext
```

- [x] **Step 7: 实现 index cache**

`internal/sdk/index.go`：

```go
type IndexCache struct {
    Dir string
}

func (c IndexCache) Path(name string) string
func (c IndexCache) Load(name string) (Index, error)
func (c IndexCache) Save(index Index) error
func (c IndexCache) Clear(name string) error
func (c IndexCache) ClearAll() error
func (c IndexCache) List() ([]CachedIndexInfo, error)
func SelectVersion(index Index, target Target) (IndexItem, error)
func SelectFile(item IndexItem, osName, arch, ext string) (IndexFile, error)
```

版本比较：

- 使用轻量 semver parser，按数字段比较。
- 过滤 prerelease：包含 `-` 的版本视为 unstable。
- `latest` 和 prefix 默认只选 stable。

- [x] **Step 8: 运行测试**

Run:

```bash
go test ./internal/sdk -run "Index|HTML|JSON|Select" -count=1
go test ./internal/sdk
```

Expected:

```text
PASS
```

- [ ] **Step 9: 提交**

Run:

```bash
git add internal/sdk/model.go internal/sdk/html_index.go internal/sdk/html_index_test.go internal/sdk/json_index.go internal/sdk/json_index_test.go internal/sdk/index.go internal/sdk/index_test.go go.mod go.sum
git commit -m "feat(sdk): parse and cache sdk indexes"
```

如果 `golang.org/x/net/html` 已经由间接依赖存在，`go.mod/go.sum` 可能无变化；不要强行提交无关依赖变动。

## Task 4: Archive Extraction With Strip Components

**Files:**

- Modify: `internal/install/archive.go`
- Modify: `internal/install/defaults_test.go`

- [x] **Step 1: 写失败测试**

在 `internal/install/defaults_test.go` 新增：

```go
func TestArchiveExtractorExtractAllToWithOptionsStripsComponents(t *testing.T)
func TestArchiveExtractorExtractAllToWithOptionsRejectsAllSkippedEntries(t *testing.T)
```

fixture：

```text
go/bin/go
go/pkg/tool.txt
```

调用：

```go
files, err := extractor.ExtractAllToWithOptions(buf.Bytes(), root, ArchiveExtractOptions{StripComponents: 1})
```

断言：

```text
root/bin/go 存在
root/pkg/tool.txt 存在
返回文件路径不包含 go/
目录时间戳仍按已有逻辑恢复
```

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/install -run ExtractAllToWithOptions -count=1
```

Expected:

```text
FAIL，ArchiveExtractOptions 或 ExtractAllToWithOptions 不存在
```

- [x] **Step 3: 实现 options API**

在 `internal/install/archive.go`：

```go
type ArchiveExtractOptions struct {
    StripComponents int
}

func (a *ArchiveExtractor) ExtractAllToWithOptions(data []byte, output string, opts ArchiveExtractOptions) ([]string, error)
```

让现有：

```go
func (a *ArchiveExtractor) ExtractAllTo(data []byte, output string) ([]string, error)
```

内部调用：

```go
return a.ExtractAllToWithOptions(data, output, ArchiveExtractOptions{})
```

新增 helper：

```go
func stripArchivePath(name string, components int) (string, bool, error)
```

规则：

- `components <= 0` 返回原路径。
- 剥离后为空的目录项跳过。
- 剥离后仍需经过 `safeArchiveRelativePath`。
- 如果所有普通文件都被跳过，返回错误。

- [x] **Step 4: 运行 install 测试**

Run:

```bash
go test ./internal/install
```

Expected:

```text
PASS
```

- [x] **Step 5: 提交**

Run:

```bash
git add internal/install/archive.go internal/install/defaults_test.go
git commit -m "feat(sdk): support archive strip components"
```

## Task 5: Resumable SDK Downloader

**Files:**

- Create: `internal/sdk/download.go`
- Create: `internal/sdk/download_test.go`

- [x] **Step 1: 写 downloader 缓存复用测试**

`internal/sdk/download_test.go`：

```text
完整文件和 meta 匹配时，不发起 GET 请求，直接返回完整文件路径
```

使用 `httptest.Server` 或 fake getter 计数。

- [x] **Step 2: 写 Range 续传失败测试**

覆盖：

```text
.part size 小于 Content-Length
meta ETag 匹配
HEAD 返回 Accept-Ranges: bytes
GET 带 Range: bytes={partSize}-
服务端返回 206
最终完整文件内容正确
.part 被 rename 为最终文件
```

- [x] **Step 3: 写重下载测试**

覆盖：

```text
ETag 不匹配删除 .part 后完整下载
服务端不支持 Range 删除 .part 后完整下载
Range 请求返回 200 OK 时重建 .part 后完整下载
下载后 Content-Length 不匹配时报错并删除 .part
```

- [x] **Step 4: 实现模型和入口**

`internal/sdk/download.go`：

```go
type DownloadRequest struct {
    URL        string
    CacheDir   string
    SDK        string
    Version    string
    Filename   string
    ClientOpts client.Options
    Progress   func(size int64) io.Writer
}

type DownloadResult struct {
    Path       string
    FromCache  bool
    Resumed    bool
    Size       int64
    ETag       string
    Modified   string
}

func DownloadArchive(ctx context.Context, req DownloadRequest) (DownloadResult, error)
```

缓存路径：

```text
{cache_dir}/sdk-downloads/{sdk}/{version}/{filename}
{cache_dir}/sdk-downloads/{sdk}/{version}/{filename}.part
{cache_dir}/sdk-downloads/{sdk}/{version}/{filename}.meta.json
```

- [x] **Step 5: 实现 HEAD 探测和 meta**

```go
type downloadMeta struct {
    Schema       int       `json:"schema"`
    URL          string    `json:"url"`
    Filename     string    `json:"filename"`
    Size         int64     `json:"size"`
    ETag         string    `json:"etag,omitempty"`
    LastModified string    `json:"last_modified,omitempty"`
    UpdatedAt    time.Time `json:"updated_at"`
}
```

用 `client.NewHTTPGetter(req.ClientOpts)` 或 `client.GetWithOptions` 发请求，保持代理、SSL、ghproxy 等现有选项一致。

- [x] **Step 6: 实现续传和完整下载**

要求：

- 所有写入先写 `.part`。
- 续传只在 Range 条件满足时追加。
- 完整下载使用 truncate。
- 下载完成后先校验大小，再 rename。
- Windows 下 rename 目标已存在时先安全删除目标文件。

- [x] **Step 7: 运行 downloader 测试**

Run:

```bash
go test ./internal/sdk -run "Download|Resume" -count=1
go test ./internal/sdk
```

Expected:

```text
PASS
```

- [ ] **Step 8: 提交**

Run:

```bash
git add internal/sdk/download.go internal/sdk/download_test.go
git commit -m "feat(sdk): add resumable archive downloads"
```

## Task 6: SDK Installed Store

**Files:**

- Modify: `internal/sdk/model.go`
- Create: `internal/sdk/store.go`
- Create: `internal/sdk/store_test.go`

- [x] **Step 1: 写 store 失败测试**

覆盖：

```text
空文件不存在时 Load 返回 schema=1 的空 store
Record go 1.21.1 后可读取
Record go 1.22.0 不覆盖 1.21.1
Remove go 1.21.1 不影响 go 1.22.0
List 按 name/version 稳定排序
损坏 JSON 返回错误
```

- [x] **Step 2: 实现 store 模型**

`internal/sdk/model.go`：

```go
type InstalledStore struct {
    Schema    int                         `json:"schema"`
    Installed map[string]InstalledSDKNode `json:"installed"`
}

type InstalledSDKNode struct {
    Versions map[string]InstalledEntry `json:"versions"`
}

type InstalledEntry struct {
    Name            string    `json:"name"`
    Version         string    `json:"version"`
    Path            string    `json:"path"`
    URL             string    `json:"url"`
    Filename        string    `json:"filename"`
    OS              string    `json:"os"`
    Arch            string    `json:"arch"`
    Ext             string    `json:"ext"`
    InstalledAt     time.Time `json:"installed_at"`
    StripComponents int       `json:"strip_components"`
}
```

- [x] **Step 3: 实现 store**

`internal/sdk/store.go`：

```go
type Store struct {
    Path string
}

func DefaultStorePath() (string, error)
func (s Store) Load() (InstalledStore, error)
func (s Store) Save(store InstalledStore) error
func (s Store) Record(entry InstalledEntry) error
func (s Store) Remove(name, version string) (InstalledEntry, error)
func (s Store) List(name string) ([]InstalledEntry, error)
```

路径规则：

- 默认 `~/.config/eget/sdk.installed.json`。
- 如果 `XDG_CONFIG_HOME` 设置，使用 `$XDG_CONFIG_HOME/eget/sdk.installed.json`。

- [x] **Step 4: 运行 store 测试**

Run:

```bash
go test ./internal/sdk -run Store -count=1
go test ./internal/sdk
```

Expected:

```text
PASS
```

- [ ] **Step 5: 提交**

Run:

```bash
git add internal/sdk/model.go internal/sdk/store.go internal/sdk/store_test.go
git commit -m "feat(sdk): store installed sdk records"
```

## Task 7: SDK Service Install/List/Remove/Index

**Files:**

- Create: `internal/sdk/service.go`
- Create: `internal/sdk/service_test.go`

- [x] **Step 1: 定义 service 依赖接口**

`internal/sdk/service.go`：

```go
type Service struct {
    Config       *config.File
    Store        Store
    IndexCache   IndexCache
    ClientOpts   client.Options
    GOOS         string
    GOARCH       string
    Now          func() time.Time
    Downloader   func(context.Context, DownloadRequest) (DownloadResult, error)
}
```

避免 service 直接依赖 CLI，也避免在测试里真实联网。

- [x] **Step 2: 写 install service 失败测试**

使用本地 zip fixture 和 fake downloader：

```text
Install go@1.21.1 渲染 URL
调用 downloader 得到 archive path
解压到临时目录
strip_components=1 后目标目录包含 bin/go
写入 sdk.installed.json
目标目录已存在且 Force=false 报错
目标目录已存在且 Force=true 安全删除后重装
```

- [x] **Step 3: 实现 install**

```go
type InstallOptions struct {
    Force bool
}

type InstallResult struct {
    Name    string
    Version string
    Path    string
    URL     string
    Cached  bool
    Resumed bool
}

func (s Service) Install(ctx context.Context, target string, opts InstallOptions) (InstallResult, error)
func (s Service) InstallMany(ctx context.Context, targets []string, opts InstallOptions) ([]InstallResult, error)
```

实现要求：

- 目标 SDK 配置可通过 alias 匹配。
- exact + `url_template` 可不读取 index。
- latest/prefix 必须读取 index。
- 安装使用 `{sdk_target}/.eget-tmp/{name}-{version}-{timestamp}` 临时目录。
- 解压完成后 rename 到最终目录。
- 失败时清理临时目录。

- [x] **Step 4: 写 list/remove 测试**

覆盖：

```text
List 返回 store 中记录
List(name) 只返回指定 SDK
Remove 删除记录和目录
Remove 目录不存在时删除记录并返回 warning 信息
Remove 路径不在允许根目录时报错且不删除
```

- [x] **Step 5: 实现 list/remove**

```go
type RemoveResult struct {
    Name    string
    Version string
    Path    string
    Missing bool
}

func (s Service) List(name string) ([]InstalledEntry, error)
func (s Service) Remove(target string) (RemoveResult, error)
```

安全删除：

- 根据安装记录拿 path。
- 解析配置中的 SDK target 根目录。
- `filepath.Abs` + `filepath.Rel` 校验 path 在允许根内。
- 不接受 `..` 逃逸。

- [x] **Step 6: 写 index action 测试**

覆盖：

```text
IndexRefresh 通过 fake HTTP body 解析并保存 cache
IndexShow 返回已缓存 index
IndexList 返回缓存摘要
IndexClear 删除缓存
IndexRefreshAll 遍历所有配置了 index_url 的 SDK
刷新失败且旧缓存存在时返回旧缓存和 warning
刷新失败且无旧缓存时报错
```

- [x] **Step 7: 实现 index actions**

```go
func (s Service) RefreshIndex(ctx context.Context, name string) (Index, error)
func (s Service) RefreshAllIndexes(ctx context.Context) ([]Index, error)
func (s Service) ShowIndex(name string) (Index, error)
func (s Service) ListIndexes() ([]CachedIndexInfo, error)
func (s Service) ClearIndex(name string) error
func (s Service) ClearAllIndexes() error
```

- [x] **Step 8: 运行 service 测试**

Run:

```bash
go test ./internal/sdk -run Service -count=1
go test ./internal/sdk
```

Expected:

```text
PASS
```

- [ ] **Step 9: 提交**

Run:

```bash
git add internal/sdk/service.go internal/sdk/service_test.go
git commit -m "feat(sdk): install and manage sdk versions"
```

## Task 8: App Layer Wiring

**Files:**

- Create: `internal/app/sdk.go`
- Create: `internal/app/sdk_test.go`
- Modify: `internal/cli/wiring.go`
- Modify: `internal/cli/service.go`

- [x] **Step 1: 写 app service 测试**

`internal/app/sdk_test.go` 覆盖：

```text
NewSDKService 使用 cfgpkg.Load 结果
默认 index cache 目录使用 global.cache_dir/sdk-index
默认 store path 使用 sdk.DefaultStorePath
网络配置复用 global proxy/cache/disable_ssl/chunk_concurrency
```

- [x] **Step 2: 实现 app SDK service**

`internal/app/sdk.go`：

```go
type SDKService struct {
    Inner sdk.Service
}

func NewSDKService(cfg *config.File) (SDKService, error)
```

或如果现有 app 层更偏向薄 service，直接暴露：

```go
func NewDefaultSDKService(cfg *config.File) (sdk.Service, error)
```

要求：

- 复用 `applyGlobalNetworkConfig` 等价逻辑；如果该函数在 `internal/cli` 中，不要反向依赖 CLI，可在 app/sdk 中独立映射。
- 不直接依赖 `kite xenv`。

- [x] **Step 3: 接入 cliService**

`internal/cli/service.go`：

```go
type cliService struct {
    // existing fields...
    sdkService sdk.Service
}
```

`internal/cli/wiring.go` 中初始化。

- [x] **Step 4: 运行 app 和 cli 测试**

Run:

```bash
go test ./internal/app ./internal/cli
```

Expected:

```text
PASS
```

- [ ] **Step 5: 提交**

Run:

```bash
git add internal/app/sdk.go internal/app/sdk_test.go internal/cli/wiring.go internal/cli/service.go
git commit -m "feat(sdk): wire sdk service"
```

## Task 9: CLI Commands

**Files:**

- Create: `internal/cli/sdk_cmd.go`
- Modify: `internal/cli/app.go`
- Modify: `internal/cli/handlers.go`
- Modify: `internal/cli/app_test.go`
- Modify: `internal/cli/service_test.go`

- [x] **Step 1: 写 CLI 路由失败测试**

在 `internal/cli/app_test.go` 新增：

```text
sdk 显示帮助且不调用 handler
sdk install go@1.21.1 调用 handler("sdk.install", *SDKInstallOptions)
sdk install --force go@1.21.1 node:20.11.1 保留多个 target
sdk list 调用 handler("sdk.list", *SDKListOptions)
sdk list --json go 设置 JSON=true Name=go
sdk remove go@1.21.1 调用 handler("sdk.remove", *SDKRemoveOptions)
sdk index refresh go 调用 handler("sdk.index.refresh", *SDKIndexOptions)
sdk index refresh --all 设置 All=true
sdk index clear --all 设置 All=true
sdk index show go 设置 Name=go
```

- [x] **Step 2: 定义 CLI options**

`internal/cli/sdk_cmd.go`：

```go
type SDKInstallOptions struct {
    Targets []string
    Force   bool
}

type SDKListOptions struct {
    Name string
    JSON bool
}

type SDKRemoveOptions struct {
    Target string
}

type SDKIndexOptions struct {
    Action string
    Name   string
    All    bool
    JSON   bool
}
```

- [x] **Step 3: 实现 gcli 子命令**

`newSDKCmd(handler CommandHandler) (*gcli.Command, func())`：

```text
sdk
  install
  list
  remove
  index
    list
    show
    refresh
    clear
```

注意：

- `sdk install` 允许一个或多个 target。
- `sdk index refresh` 需要 `<name>` 或 `--all` 二选一。
- `sdk index clear` 需要 `<name>` 或 `--all` 二选一。
- `sdk index show` 必须有 `<name>`。

- [x] **Step 4: 注册命令**

在 `internal/cli/app.go` 注册：

```go
app.add(newSDKCmd(handler))
```

保留现有命令顺序；建议放在 `install/download` 附近或 `config` 前。

- [x] **Step 5: 实现 handlers**

`internal/cli/handlers.go`：

```go
case "sdk.install":
    return s.handleSDKInstall(opts.(*SDKInstallOptions))
case "sdk.list":
    return s.handleSDKList(opts.(*SDKListOptions))
case "sdk.remove":
    return s.handleSDKRemove(opts.(*SDKRemoveOptions))
case "sdk.index.list":
case "sdk.index.show":
case "sdk.index.refresh":
case "sdk.index.clear":
```

输出规则：

- `install`：每个 result 打印 name/version/path；如果 `Resumed` 或 `Cached`，附加简短提示。
- `list`：表格输出 `Name Version Path Installed At`。
- `list --json`：JSON 数组。
- `remove`：打印删除路径；missing 时 warning。
- `index show`：输出规范化 JSON。
- `index list`：表格输出 `Name Versions Source Updated At`。

- [x] **Step 6: 写 handler/service 测试**

在 `internal/cli/service_test.go` 使用 fake sdk service 或可注入函数，覆盖输出格式和错误路径。

如当前 `cliService` 不方便 mock，先抽 interface：

```go
type sdkService interface {
    InstallMany(context.Context, []string, sdk.InstallOptions) ([]sdk.InstallResult, error)
    List(string) ([]sdk.InstalledEntry, error)
    Remove(string) (sdk.RemoveResult, error)
    // index methods...
}
```

- [x] **Step 7: 运行 CLI 测试**

Run:

```bash
go test ./internal/cli
```

Expected:

```text
PASS
```

- [ ] **Step 8: 提交**

Run:

```bash
git add internal/cli/sdk_cmd.go internal/cli/app.go internal/cli/handlers.go internal/cli/app_test.go internal/cli/service_test.go
git commit -m "feat(sdk): add sdk cli commands"
```

## Task 10: End-To-End SDK Tests

**Files:**

- Modify: `internal/sdk/service_test.go`
- Modify: `internal/cli/service_test.go`
- Optional Create: `internal/sdk/testdata/*`

- [ ] **Step 1: 写本地 HTTP server 集成测试**

在 `internal/sdk/service_test.go` 新增：

```text
httptest server 提供 /index HTML
httptest server 提供 /go1.21.1.linux-amd64.tar.gz
service Install go@1.21.1
断言目标目录包含 bin/go
断言 sdk.installed.json 写入
```

测试 archive 可用 zip 简化；不要依赖真实外网。

- [ ] **Step 2: 写断点续传集成测试**

模拟：

```text
已有 .part 包含前半段
meta 匹配
server 支持 HEAD + Range
Install 触发 DownloadArchive 续传
```

断言：

```text
GET 请求包含 Range
完整 archive 解压成功
```

- [ ] **Step 3: 写 CLI 级 smoke 测试**

使用临时配置和 fake service，确认：

```text
eget sdk install go@1.21.1
eget sdk list --json
eget sdk index show go
```

至少覆盖 CLI 到 handler 的数据流，不需要真实下载。

- [ ] **Step 4: 运行全量测试**

Run:

```bash
go test ./...
```

Expected:

```text
PASS
```

- [ ] **Step 5: 提交**

Run:

```bash
git add internal/sdk/service_test.go internal/cli/service_test.go internal/sdk/testdata
git commit -m "test(sdk): cover sdk install workflow"
```

如果没有新增 `testdata`，不要把不存在路径加入 `git add`。

## Task 11: Documentation And TODO

**Files:**

- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/DOCS.md`
- Modify: `docs/example.eget.toml`
- Modify: `docs/TODO.md`
- Optional Modify: `docs/superpowers/specs/2026-05-14-sdk-download-design.md`

- [ ] **Step 1: 更新 README**

添加简短章节：

```text
SDK downloads
eget sdk install go@1.22.0
eget sdk install go:1.22 node:20.11.1
eget sdk list
eget sdk remove go@1.22.0
eget sdk index refresh go
```

明确：

```text
eget sdk 只负责下载和安装，不负责 use/env/PATH。
如需环境切换，配合 kite xenv tools index 和 xenv use。
```

- [ ] **Step 2: 更新中文 README**

同步中文说明，特别强调：

```text
不会修改 PATH
不会写 .xenv.toml
安装后可运行 kite xenv tools index
```

- [ ] **Step 3: 更新 docs/DOCS.md**

补充完整配置字段和命令说明。

- [ ] **Step 4: 更新 example config**

确认 `docs/example.eget.toml` 中 SDK 示例与真实实现字段一致。

- [ ] **Step 5: 更新 TODO checkbox**

在 `docs/TODO.md` 将 SDK 下载功能对应项标记为完成或拆分成已完成/后续项。

如果仍有后续项，例如 `pkg/sdk`、checksum、更多 SDK provider，新增明确后续 TODO，不要把半成品描述为完成。

- [ ] **Step 6: 运行文档相关检查**

Run:

```bash
rg -n "sdk_download_ext|download_ext" README.md README.zh-CN.md docs docs/example.eget.toml
rg -n "go 1\\.21\\.1" README.md README.zh-CN.md docs/DOCS.md docs/example.eget.toml
```

Expected:

```text
不再出现旧字段 sdk_download_ext/download_ext；用户文档不把 go 1.21.1 描述为支持格式
```

- [ ] **Step 7: 全量测试**

Run:

```bash
go test ./...
```

Expected:

```text
PASS
```

- [ ] **Step 8: 提交**

Run:

```bash
git add README.md README.zh-CN.md docs/DOCS.md docs/example.eget.toml docs/TODO.md docs/superpowers/specs/2026-05-14-sdk-download-design.md
git commit -m "docs(sdk): document sdk download commands"
```

如果 spec 没有修改，不要把它加入提交。

## Task 12: Final Verification

**Files:** 无预期代码修改。

- [ ] **Step 1: 检查工作区**

Run:

```bash
git status --short
```

Expected:

```text
无未提交改动
```

- [ ] **Step 2: 运行全量测试**

Run:

```bash
go test ./...
```

Expected:

```text
所有 package 通过
```

- [ ] **Step 3: 检查旧字段和旧格式**

Run:

```bash
rg -n "sdk_download_ext|download_ext" internal docs README.md README.zh-CN.md
rg -n "go 1\\.21\\.1" README.md README.zh-CN.md docs/DOCS.md docs/example.eget.toml
```

Expected:

```text
不出现旧字段；用户文档不把 go 1.21.1 描述为支持格式
```

- [ ] **Step 4: 检查 SDK 命令 help**

Run:

```bash
go run ./cmd/eget --help
go run ./cmd/eget sdk --help
go run ./cmd/eget sdk index --help
```

Expected:

```text
help 中能看到 sdk 及其子命令
命令运行退出码为 0
```

- [ ] **Step 5: 手动 smoke，使用本地 fixture 或临时 server**

如果已经有本地 integration helper，运行对应命令。不要在最终验证依赖真实外网 mirror。

Expected:

```text
sdk install/list/remove/index 主链路可在本地测试资源上跑通
```

## 风险和回滚点

- SDK 下载会引入落盘 downloader，不要改动现有 `client.Download()` 的行为；如果 downloader 出问题，可单独回滚 `internal/sdk/download.go` 相关 commit。
- `ArchiveExtractor.ExtractAllToWithOptions` 必须保持 `ExtractAllTo` 兼容；如果现有 install 测试失败，优先修 options wrapper，不要修改现有调用语义。
- `sdk remove` 涉及递归删除，必须依赖 store 记录和路径安全校验。任何安全校验不确定时，返回错误，不删除。
- CLI 命令接入最后做；在 `internal/sdk` service 未稳定前不要暴露半成品命令。
- 首版不开放 `pkg/sdk`，避免早期模型变成公共兼容承诺。

## 非目标

本计划不实现：

- `eget sdk use`
- `eget sdk env`
- `eget sdk script`
- shell hook
- shim 管理
- 自动修改 `PATH`
- `.xenv.toml` 写入
- `kite xenv` 命令自动调用
- SDK checksum 强校验
- 多进程安装文件锁
- 通用 provider plugin 系统
- `pkg/sdk` 公共 API
