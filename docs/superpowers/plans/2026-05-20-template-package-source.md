# Template Package Source 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为普通 managed package 增加 `template:<id>` 来源，使独立下载站包可以通过配置完成 latest 解析、URL 渲染、checksum 校验、安装、更新，以及受控 `run-asset` installer action。

**Architecture:** 新增 `internal/source/urltemplate` 负责 template target 解析、平台变量渲染、latest/checksum 元数据读取。配置字段从 `config.Section` 合并到 `install.Options.URLTemplate`，`install.Service.SelectFinder` 将 `template:<id>` 渲染为单个下载 URL，`InstallRunner` 在下载和校验后根据 `install_action` 选择普通提取或执行已校验 asset。`list/update` 的 latest checker 改为接收 package section，确保 `template:<id>` 能用配置字段查询 latest。

**Tech Stack:** Go, TOML/gookit config, existing `internal/client` HTTP/cache/proxy layer, existing install runner, installed store, `github.com/gookit/goutil/testutil/assert`。

---

## 文件边界

- 新增 `internal/source/urltemplate/target.go`: 解析 `template:<id>`。
- 新增 `internal/source/urltemplate/template.go`: 配置结构、变量渲染、JSON path、regex 提取。
- 新增 `internal/source/urltemplate/platform.go`: Linux libc 和 macOS Rosetta 平台修正。
- 新增 `internal/source/urltemplate/finder.go`: latest 查询、asset URL finder、checksum 解析。
- 修改 `internal/config/model.go`: 新增 package template 字段。
- 修改 `internal/config/gookit.go`: 新字段 encode/decode/set 支持。
- 修改 `internal/config/merge.go`: 合并 template 字段。
- 修改 `internal/install/options.go`: 新增 `TargetTemplate` 和 `URLTemplateOptions`。
- 修改 `internal/install/service.go`: `SelectFinder` 和 checksum verifier 支持 template source。
- 修改 `internal/install/runner.go`: 支持 `install_action = "run-asset"`。
- 修改 `internal/app/install.go`: runtime options 合并、installed store 记录。
- 修改 `internal/app/list.go`, `internal/app/update.go`: latest checker 输入升级。
- 修改 `internal/cli/wiring.go`: latest checker dispatch 支持 `template:<id>`。
- 修改文档：`README.md`, `README.zh-CN.md`, `docs/config.md`, `docs/config.zh-CN.md`, `docs/example.eget.toml`, `docs/architecture.md`。

---

## Task 1: 配置模型和 Round Trip

**Files:**
- Modify: `internal/config/model.go`
- Modify: `internal/config/gookit.go`
- Modify: `internal/config/merge.go`
- Test: `internal/config/gookit_test.go`
- Test: `internal/config/merge_test.go`

- [x] **Step 1: 新增配置 round-trip 失败测试**

在 `internal/config/gookit_test.go` 添加：

```go
func TestPackageURLTemplateFieldsRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	repo := "template:claude"
	latestURL := "https://downloads.claude.ai/claude-code-releases/latest"
	latestFormat := "text"
	urlTemplate := "https://downloads.claude.ai/claude-code-releases/{version}/{os}-{arch}{libc}/claude{ext}"
	checksumURL := "https://downloads.claude.ai/claude-code-releases/{version}/manifest.json"
	checksumFormat := "json"
	checksumPath := "platforms.{os}-{arch}{libc}.checksum"
	installAction := "run-asset"

	cfg := NewFile()
	cfg.Packages["claude"] = Section{
		Repo:                &repo,
		LatestURL:           &latestURL,
		LatestFormat:        &latestFormat,
		URLTemplate:         &urlTemplate,
		OSMap:               map[string]string{"windows": "win32", "linux": "linux", "darwin": "darwin"},
		ArchMap:             map[string]string{"amd64": "x64", "arm64": "arm64"},
		ExtMap:              map[string]string{"windows": ".exe", "linux": "", "darwin": ""},
		LibcMap:             map[string]string{"glibc": "", "musl": "-musl"},
		ChecksumURLTemplate: &checksumURL,
		ChecksumFormat:      &checksumFormat,
		ChecksumJSONPath:    &checksumPath,
		InstallAction:       &installAction,
		InstallArgs:         []string{"install", "latest"},
	}

	text, err := dumpConfigString(cfg)
	assert.NoErr(t, err)
	assert.Contains(t, text, `repo = "template:claude"`)
	assert.Contains(t, text, `install_args = ["install", "latest"]`)

	err = Save(configPath, cfg)
	assert.NoErr(t, err)
	loaded, err := LoadFile(configPath)
	assert.NoErr(t, err)

	pkg := loaded.Packages["claude"]
	assert.Eq(t, repo, *pkg.Repo)
	assert.Eq(t, latestURL, *pkg.LatestURL)
	assert.Eq(t, urlTemplate, *pkg.URLTemplate)
	assert.Eq(t, map[string]string{"windows": ".exe", "linux": "", "darwin": ""}, pkg.ExtMap)
	assert.Eq(t, map[string]string{"glibc": "", "musl": "-musl"}, pkg.LibcMap)
	assert.Eq(t, []string{"install", "latest"}, pkg.InstallArgs)
}
```

如果当前文件还没有 `path/filepath` import，需要添加。

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/config -run TestPackageURLTemplateFieldsRoundTrip
```

Expected: FAIL，缺少 `Section.LatestURL`、`Section.URLTemplate`、map 字段或 install action 字段。

- [x] **Step 3: 添加配置字段**

在 `internal/config/model.go` 的 `Section` 添加：

```go
URLTemplate         *string           `toml:"url_template" mapstructure:"url_template"`
LatestURL           *string           `toml:"latest_url" mapstructure:"latest_url"`
LatestFormat        *string           `toml:"latest_format" mapstructure:"latest_format"`
LatestJSONPath      *string           `toml:"latest_json_path" mapstructure:"latest_json_path"`
VersionRegex        *string           `toml:"version_regex" mapstructure:"version_regex"`
OSMap               map[string]string `toml:"os_map" mapstructure:"os_map"`
ArchMap             map[string]string `toml:"arch_map" mapstructure:"arch_map"`
ExtMap              map[string]string `toml:"ext_map" mapstructure:"ext_map"`
LibcMap             map[string]string `toml:"libc_map" mapstructure:"libc_map"`
ChecksumURLTemplate *string           `toml:"checksum_url_template" mapstructure:"checksum_url_template"`
ChecksumFormat      *string           `toml:"checksum_format" mapstructure:"checksum_format"`
ChecksumJSONPath    *string           `toml:"checksum_json_path" mapstructure:"checksum_json_path"`
ChecksumRegex       *string           `toml:"checksum_regex" mapstructure:"checksum_regex"`
InstallAction       *string           `toml:"install_action" mapstructure:"install_action"`
InstallArgs         []string          `toml:"install_args" mapstructure:"install_args"`
```

在 `Merged` 添加同名非指针字段；首版不为这些字段添加 CLI overrides。

- [x] **Step 4: 支持 encode/decode**

在 `internal/config/gookit.go` 的 `sectionToMap` 添加新字段输出；`install_args` 按 `[]string` 输出。更新 `normalizePathValue`：

```go
case "asset_filters", "fallbacks", "ignore_update_packages", "install_args":
	return splitAndTrim(text), true
```

- [x] **Step 5: 新增 merge 测试**

在 `internal/config/merge_test.go` 添加：

```go
func TestMergeInstallOptionsMergesURLTemplateFields(t *testing.T) {
	merged := MergeInstallOptions(
		Section{
			URLTemplate: stringPtr("global"),
			OSMap:       map[string]string{"windows": "global-win"},
		},
		Section{
			URLTemplate: stringPtr("repo"),
			OSMap:       map[string]string{"windows": "repo-win"},
		},
		Section{
			URLTemplate:   stringPtr("package"),
			LatestURL:     stringPtr("https://example.com/latest"),
			LatestFormat:  stringPtr("text"),
			OSMap:         map[string]string{"windows": "win32"},
			ArchMap:       map[string]string{"amd64": "x64"},
			ExtMap:        map[string]string{"windows": ".exe"},
			LibcMap:       map[string]string{"musl": "-musl"},
			InstallAction: stringPtr("run-asset"),
			InstallArgs:   []string{"install", "latest"},
		},
		CLIOverrides{},
	)

	assert.Eq(t, "package", merged.URLTemplate)
	assert.Eq(t, "https://example.com/latest", merged.LatestURL)
	assert.Eq(t, "text", merged.LatestFormat)
	assert.Eq(t, map[string]string{"windows": "win32"}, merged.OSMap)
	assert.Eq(t, map[string]string{"amd64": "x64"}, merged.ArchMap)
	assert.Eq(t, map[string]string{"windows": ".exe"}, merged.ExtMap)
	assert.Eq(t, map[string]string{"musl": "-musl"}, merged.LibcMap)
	assert.Eq(t, "run-asset", merged.InstallAction)
	assert.Eq(t, []string{"install", "latest"}, merged.InstallArgs)
}
```

- [x] **Step 6: 实现 merge**

在 `internal/config/merge.go` 合并 package > repo > global：

```go
merged.URLTemplate = firstString(pkg.URLTemplate, repo.URLTemplate, global.URLTemplate)
merged.LatestURL = firstString(pkg.LatestURL, repo.LatestURL, global.LatestURL)
merged.LatestFormat = firstString(pkg.LatestFormat, repo.LatestFormat, global.LatestFormat)
merged.LatestJSONPath = firstString(pkg.LatestJSONPath, repo.LatestJSONPath, global.LatestJSONPath)
merged.VersionRegex = firstString(pkg.VersionRegex, repo.VersionRegex, global.VersionRegex)
merged.ChecksumURLTemplate = firstString(pkg.ChecksumURLTemplate, repo.ChecksumURLTemplate, global.ChecksumURLTemplate)
merged.ChecksumFormat = firstString(pkg.ChecksumFormat, repo.ChecksumFormat, global.ChecksumFormat)
merged.ChecksumJSONPath = firstString(pkg.ChecksumJSONPath, repo.ChecksumJSONPath, global.ChecksumJSONPath)
merged.ChecksumRegex = firstString(pkg.ChecksumRegex, repo.ChecksumRegex, global.ChecksumRegex)
merged.InstallAction = firstString(pkg.InstallAction, repo.InstallAction, global.InstallAction)
merged.OSMap = firstStringMap(nil, pkg.OSMap, repo.OSMap, global.OSMap)
merged.ArchMap = firstStringMap(nil, pkg.ArchMap, repo.ArchMap, global.ArchMap)
merged.ExtMap = firstStringMap(nil, pkg.ExtMap, repo.ExtMap, global.ExtMap)
merged.LibcMap = firstStringMap(nil, pkg.LibcMap, repo.LibcMap, global.LibcMap)
merged.InstallArgs = firstStrings(nil, pkg.InstallArgs, repo.InstallArgs, global.InstallArgs)
```

- [x] **Step 7: 运行配置测试**

Run:

```bash
go test ./internal/config
```

Expected: PASS.

- [x] **Step 8: 提交**

```bash
git add internal/config/model.go internal/config/gookit.go internal/config/merge.go internal/config/gookit_test.go internal/config/merge_test.go
git commit -m "feat(config): add template package fields"
```

---

## Task 2: URL Template Source 包

**Files:**
- Create: `internal/source/urltemplate/target.go`
- Create: `internal/source/urltemplate/template.go`
- Create: `internal/source/urltemplate/platform.go`
- Create: `internal/source/urltemplate/finder.go`
- Test: `internal/source/urltemplate/target_test.go`
- Test: `internal/source/urltemplate/template_test.go`
- Test: `internal/source/urltemplate/finder_test.go`

- [x] **Step 1: 编写 target 解析测试**

Create `internal/source/urltemplate/target_test.go`:

```go
package urltemplate

import (
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantID  string
		wantErr string
	}{
		{name: "valid", input: "template:claude", wantID: "claude"},
		{name: "empty id", input: "template:", wantErr: "invalid template target"},
		{name: "not template", input: "owner/repo", wantErr: "invalid template target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTarget(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			assert.NoErr(t, err)
			assert.Eq(t, tt.wantID, got.ID)
			assert.Eq(t, tt.input, got.Normalized)
		})
	}
}
```

- [x] **Step 2: 实现 target 解析**

Create `internal/source/urltemplate/target.go`:

```go
package urltemplate

import (
	"fmt"
	"strings"
)

const Prefix = "template:"

type Target struct {
	ID         string
	Normalized string
}

func IsTarget(value string) bool {
	return strings.HasPrefix(value, Prefix)
}

func ParseTarget(value string) (Target, error) {
	if !IsTarget(value) {
		return Target{}, fmt.Errorf("invalid template target %q", value)
	}
	id := strings.TrimSpace(strings.TrimPrefix(value, Prefix))
	if id == "" {
		return Target{}, fmt.Errorf("invalid template target %q", value)
	}
	return Target{ID: id, Normalized: Prefix + id}, nil
}
```

- [x] **Step 3: 编写模板渲染和 metadata 解析测试**

Create `internal/source/urltemplate/template_test.go`，覆盖：

```go
func TestRenderClaudeWindowsTemplate(t *testing.T) {
	cfg := Config{
		URLTemplate: "https://downloads.claude.ai/{version}/{os}-{arch}{libc}/claude{ext}",
		OSMap:      map[string]string{"windows": "win32", "linux": "linux", "darwin": "darwin"},
		ArchMap:    map[string]string{"amd64": "x64", "arm64": "arm64"},
		ExtMap:     map[string]string{"windows": ".exe", "linux": "", "darwin": ""},
		LibcMap:    map[string]string{"glibc": "", "musl": "-musl"},
	}
	vars, err := VariablesFor(VariableInput{Name: "claude", Version: "1.2.3", GOOS: "windows", GOARCH: "amd64", Config: cfg})
	assert.NoErr(t, err)
	got, err := Render(cfg.URLTemplate, vars)
	assert.NoErr(t, err)
	assert.Eq(t, "https://downloads.claude.ai/1.2.3/win32-x64/claude.exe", got)
}

func TestRenderClaudeLinuxMuslTemplate(t *testing.T) {
	cfg := Config{
		URLTemplate: "https://downloads.claude.ai/{version}/{os}-{arch}{libc}/claude{ext}",
		OSMap:      map[string]string{"linux": "linux"},
		ArchMap:    map[string]string{"amd64": "x64"},
		ExtMap:     map[string]string{"linux": ""},
		LibcMap:    map[string]string{"glibc": "", "musl": "-musl"},
	}
	vars, err := VariablesFor(VariableInput{Name: "claude", Version: "1.2.3", GOOS: "linux", GOARCH: "amd64", Libc: "musl", Config: cfg})
	assert.NoErr(t, err)
	got, err := Render(cfg.URLTemplate, vars)
	assert.NoErr(t, err)
	assert.Eq(t, "https://downloads.claude.ai/1.2.3/linux-x64-musl/claude", got)
}

func TestParseLatestTextAndJSON(t *testing.T) {
	got, err := ParseLatest([]byte("1.2.3\n"), Config{LatestFormat: "text"})
	assert.NoErr(t, err)
	assert.Eq(t, "1.2.3", got)

	got, err = ParseLatest([]byte(`{"version":"1.2.4"}`), Config{LatestFormat: "json", LatestJSONPath: "version"})
	assert.NoErr(t, err)
	assert.Eq(t, "1.2.4", got)
}

func TestExtractChecksumJSONPathWithRenderedPath(t *testing.T) {
	vars := map[string]string{"os": "linux", "arch": "x64", "libc": "-musl"}
	path, err := Render("platforms.{os}-{arch}{libc}.checksum", vars)
	assert.NoErr(t, err)
	got, err := ExtractJSONPath([]byte(`{"platforms":{"linux-x64-musl":{"checksum":"abc123"}}}`), path)
	assert.NoErr(t, err)
	assert.Eq(t, "abc123", got)
}

func TestRenderRejectsUnknownVariable(t *testing.T) {
	_, err := Render("https://example.com/{unknown}", map[string]string{"name": "tool"})
	if err == nil {
		t.Fatal("expected unknown variable error")
	}
}
```

- [x] **Step 4: 实现 `template.go`**

实现 `Config`、`VariablesFor`、`Render`、`ParseLatest`、`ParseChecksum`、`ExtractJSONPath`、regex helper。关键要求：

- 未配置 `latest_format` 时默认 `text`。
- `latest_format = "json"` 必须设置 `latest_json_path`。
- `checksum_format = "json"` 必须设置 `checksum_json_path`。
- `{ext}` 缺失时为空字符串。
- `{libc}` 只在 Linux 且有检测结果时渲染。
- 未知变量直接报错。

- [x] **Step 5: 编写 finder 测试**

Create `internal/source/urltemplate/finder_test.go`:

```go
func TestFinderFindsRenderedURLFromLatest(t *testing.T) {
	getter := &fakeGetter{responses: map[string]string{
		"https://example.com/latest": "1.2.3",
	}}
	finder := Finder{
		Name:   "claude",
		Target: Target{ID: "claude", Normalized: "template:claude"},
		Config: Config{
			LatestURL:   "https://example.com/latest",
			URLTemplate: "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:       map[string]string{"windows": "win32"},
			ArchMap:     map[string]string{"amd64": "x64"},
			ExtMap:      map[string]string{"windows": ".exe"},
		},
		GOOS:   "windows",
		GOARCH: "amd64",
		Getter: getter,
	}

	assets, err := finder.Find()
	assert.NoErr(t, err)
	assert.Eq(t, []string{"https://example.com/1.2.3/win32-x64/claude.exe"}, assets)
	assert.Eq(t, []string{"https://example.com/latest"}, getter.requests)
	assert.Eq(t, "1.2.3", finder.Version)
}
```

测试 helper:

```go
type fakeGetter struct {
	responses map[string]string
	requests  []string
}

func (f *fakeGetter) Get(url string) (*http.Response, error) {
	f.requests = append(f.requests, url)
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(f.responses[url]))}, nil
}
```

- [x] **Step 6: 实现 finder**

Create `internal/source/urltemplate/finder.go`，提供：

- `type HTTPGetter interface { Get(url string) (*http.Response, error) }`
- `type Finder struct { Name, Target, Config, Tag, GOOS, GOARCH, Libc, Getter, Version }`
- `Find() ([]string, error)`
- `Latest() (LatestInfo, error)`
- `Vars() map[string]string`

`Find()` 逻辑：

1. `URLTemplate` 为空时报错。
2. `Tag` 非空则直接用 tag，否则请求 latest。
3. 渲染变量和 URL。
4. 保存 `Version` 和 vars，返回单个 URL。

- [x] **Step 7: 编写平台检测测试并实现**

Create `internal/source/urltemplate/platform.go`，测试：

```go
func TestEffectiveSystemUsesExplicitSystem(t *testing.T) {
	goos, goarch, libc := EffectiveSystem("linux/amd64", "darwin", "arm64", func() string { return "musl" }, func(string, string) (string, string) {
		return "darwin", "arm64"
	})
	assert.Eq(t, "linux", goos)
	assert.Eq(t, "amd64", goarch)
	assert.Eq(t, "musl", libc)
}

func TestEffectiveSystemAppliesRosettaFixOnlyForImplicitSystem(t *testing.T) {
	goos, goarch, libc := EffectiveSystem("", "darwin", "amd64", func() string { return "" }, func(string, string) (string, string) {
		return "darwin", "arm64"
	})
	assert.Eq(t, "darwin", goos)
	assert.Eq(t, "arm64", goarch)
	assert.Eq(t, "", libc)
}
```

实现 `EffectiveSystem`、`DetectLibc`、`FixDarwinRosetta`。生产检测为 best-effort，测试使用注入函数，避免平台依赖。

- [x] **Step 8: 运行 source tests**

Run:

```bash
go test ./internal/source/urltemplate
```

Expected: PASS.

- [x] **Step 9: 提交**

```bash
git add internal/source/urltemplate
git commit -m "feat(source): add template source renderer"
```

---

## Task 3: Install Finder Wiring

**Files:**
- Modify: `internal/install/options.go`
- Modify: `internal/install/service.go`
- Modify: `internal/app/install.go`
- Test: `internal/install/service_test.go`
- Test: `internal/app/install_test.go`

- [x] **Step 1: 新增 `SelectFinder` 测试**

在 `internal/install/service_test.go` 的 `TestSelectFinder` 中添加 `template target` subtest，断言：

- `SelectFinder("template:claude", opts)` 返回 tool `claude`。
- finder 请求 `latest_url`。
- finder 返回渲染后的单个 URL。

测试中使用 `TemplateGetterFactory` 注入 fake getter。

- [x] **Step 2: 实现 target kind 和 runtime options**

在 `internal/install/options.go`：

```go
TargetTemplate TargetKind = "template"
```

`DetectTargetKind` 在 GitHub URL 检测前增加：

```go
case urltemplate.IsTarget(target):
	return TargetTemplate
```

新增：

```go
type URLTemplateOptions struct {
	URLTemplate         string
	LatestURL           string
	LatestFormat        string
	LatestJSONPath      string
	VersionRegex        string
	OSMap               map[string]string
	ArchMap             map[string]string
	ExtMap              map[string]string
	LibcMap             map[string]string
	ChecksumURLTemplate string
	ChecksumFormat      string
	ChecksumJSONPath    string
	ChecksumRegex       string
	InstallAction       string
	InstallArgs         []string
	ResolvedVersion     string
	ResolvedVars        map[string]string
}
```

并在 `Options` 添加 `URLTemplate URLTemplateOptions`。

- [x] **Step 3: `SelectFinder` 支持 template**

在 `internal/install/service.go` 的 `Service` 添加：

```go
TemplateGetterFactory func(opts Options) urltemplate.HTTPGetter
```

新增 `TargetTemplate` case：

```go
case TargetTemplate:
	templateTarget, err := urltemplate.ParseTarget(target)
	if err != nil {
		return nil, "", err
	}
	getter := urltemplate.HTTPGetter(NewHTTPGetter(*opts))
	if s.TemplateGetterFactory != nil {
		getter = s.TemplateGetterFactory(*opts)
	}
	goos, goarch, libc := urltemplate.EffectiveSystem(opts.System, runtime.GOOS, runtime.GOARCH, urltemplate.DetectLibc, urltemplate.FixDarwinRosetta)
	return &urltemplate.Finder{
		Name:   templateTarget.ID,
		Target: templateTarget,
		Config: urlTemplateConfigFromOptions(opts.URLTemplate),
		Tag:    opts.Tag,
		GOOS:   goos,
		GOARCH: goarch,
		Libc:   libc,
		Getter: getter,
	}, templateTarget.ID, nil
```

- [x] **Step 4: App 层把配置合并到 runtime options**

在 `internal/app/install.go` 的 `resolveInstallOptionsWithConfig` 返回 `install.Options` 时填充 `URLTemplate`。字段来自 `merged`，map/slice 要 clone。

- [x] **Step 5: 新增 app merge 测试**

在 `internal/app/install_test.go` 添加 `TestInstallTargetMergesTemplatePackageOptions`，断言 managed package `claude` 解析到 `template:claude`，并把 `LatestURL`、`URLTemplate`、`InstallAction`、`InstallArgs` 传给 runner。

- [x] **Step 6: 运行测试**

Run:

```bash
go test ./internal/source/urltemplate ./internal/install ./internal/app
```

Expected: PASS.

- [x] **Step 7: 提交**

```bash
git add internal/install internal/app internal/source/urltemplate
git commit -m "feat(install): wire template package source"
```

---

## Task 4: Checksum Manifest Integration

**Files:**
- Modify: `internal/source/urltemplate/finder.go`
- Modify: `internal/install/service.go`
- Modify: `internal/install/runner.go`
- Test: `internal/install/service_test.go`

- [x] **Step 1: 新增 checksum resolver 测试**

在 `internal/install/service_test.go` 添加 `TestTemplateChecksumVerifierUsesRenderedManifest`。测试目标：

- fake latest 返回 `1.2.3`。
- fake manifest 返回 `{"platforms":{"win32-x64":{"checksum":"abc"}}}`。
- `SelectVerifier("", &opts)` 最终调用 `Sha256VerifierFactory("abc")`。

测试片段：

```go
var verifierValue string
svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
	verifierValue = expected
	return &fakeVerifier{name: "verify:" + expected}, nil
}
```

- [x] **Step 2: runner 保存 resolved template state**

在 `InstallRunner.Run` 的 `finder.Find()` 后：

```go
if templateFinder, ok := finder.(*urltemplate.Finder); ok {
	opts.URLTemplate.ResolvedVersion = templateFinder.Version
	opts.URLTemplate.ResolvedVars = templateFinder.Vars()
	opts.CacheVersion = templateFinder.Version
}
```

- [x] **Step 3: 实现 checksum resolver**

在 `internal/install/service.go` 的 `SelectVerifier` 中，显式 `Verify` 优先；否则当 `URLTemplate.ChecksumURLTemplate` 非空时：

1. 使用 `ResolvedVars` 渲染 `checksum_url_template`。
2. 渲染 `checksum_json_path`。
3. `GetWithOptions` 请求 manifest。
4. `urltemplate.ParseChecksum` 提取 checksum。
5. 调用 `Sha256VerifierFactory(checksum)`。

- [x] **Step 4: 显式 verify 优先测试**

新增测试：同时配置 `Verify` 和 `ChecksumURLTemplate` 时，不请求 manifest，`Sha256VerifierFactory` 收到 `Verify`。

- [x] **Step 5: 运行测试**

Run:

```bash
go test ./internal/source/urltemplate ./internal/install
```

Expected: PASS.

- [x] **Step 6: 提交**

```bash
git add internal/source/urltemplate internal/install
git commit -m "feat(install): verify template checksums"
```

---

## Task 5: Run-Asset Installer Action

**Files:**
- Modify: `internal/install/options.go`
- Modify: `internal/install/runner.go`
- Test: `internal/install/runner_test.go`

- [ ] **Step 1: 新增 run-asset 测试**

在 `internal/install/runner_test.go` 添加：

- checksum 成功后调用 fake asset runner。
- checksum 失败时不调用 asset runner。
- `install_args` 原样传递。
- result `InstallMode == "run-asset"`。

测试 runner hook：

```go
runner.AssetRunner = func(path string, args []string, stdout, stderr io.Writer) error {
	launchedPath = path
	launchedArgs = append([]string(nil), args...)
	return nil
}
```

- [ ] **Step 2: 添加常量和 hook**

在 `internal/install/options.go`：

```go
const InstallModeRunAsset = "run-asset"
const InstallActionRunAsset = "run-asset"
```

在 `InstallRunner` 添加：

```go
AssetRunner func(path string, args []string, stdout, stderr io.Writer) error
```

默认实现使用 `exec.Command(path, args...)`，stdout/stderr 直连。

- [ ] **Step 3: 实现 action branch**

在 checksum verify 成功后、选择 extractor 前：

- `InstallAction` 不是空且不是 `run-asset` 时返回错误。
- `run-asset` 没有 `Verify` 或 `ChecksumURLTemplate` 时返回错误。
- materialize asset 到 cache 或临时执行副本。
- 非 Windows 设置 executable bit。
- 输出 `Running installer asset: ...`。
- 调用 `AssetRunner`。
- 返回 `RunResult{InstallMode: InstallModeRunAsset, Version: opts.URLTemplate.ResolvedVersion}`。

不要新增 `install_cleanup`；cache 文件遵循现有 cache 语义，临时执行副本由实现自行清理。

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/install -run RunAsset
go test ./internal/install
```

Expected: PASS.

- [ ] **Step 5: 提交**

```bash
git add internal/install
git commit -m "feat(install): support run asset action"
```

---

## Task 6: App Install 记录和 Installed Store

**Files:**
- Modify: `internal/app/install.go`
- Modify: `internal/app/update.go`
- Test: `internal/app/install_test.go`
- Test: `internal/app/update_test.go`

- [ ] **Step 1: RunResult 增加 Version**

在 `internal/install/runner.go` 的 `RunResult` 增加：

```go
Version string
```

template finder resolve 后设置 result version。

- [ ] **Step 2: installed store 记录 version 和 mode**

在 `internal/app/install.go` 的 `installResolvedTarget` 中：

- 对 `TargetTemplate`，优先使用 `result.Version` 作为 `tag`。
- `entry.Version` 对 template package 也记录 selected version。
- `entry.InstallMode` 记录 `run-asset`。

- [ ] **Step 3: 记录 template options**

在 `extractOptionsMap` 中记录：

- `url_template`
- `latest_url`
- `latest_format`
- `latest_json_path`
- `version_regex`
- `os_map`
- `arch_map`
- `ext_map`
- `libc_map`
- `checksum_url_template`
- `checksum_format`
- `checksum_json_path`
- `checksum_regex`
- `install_action`
- `install_args`

- [ ] **Step 4: installed-only update 能还原 options**

在 `internal/app/update.go` 的 `optionsFromInstalledEntry` 读取上述字段，填入 `opts.URLTemplate`。

- [ ] **Step 5: 新增 app 测试**

添加：

- `TestInstallTargetRecordsTemplateVersionAndRunAssetMode`
- installed-only `template:claude` update 能从 installed options 恢复 `URLTemplate` 字段。

- [ ] **Step 6: 运行 app tests**

Run:

```bash
go test ./internal/app
```

Expected: PASS.

- [ ] **Step 7: 提交**

```bash
git add internal/app internal/install
git commit -m "feat(app): record template package installs"
```

---

## Task 7: List/Update Latest Checker 升级

**Files:**
- Modify: `internal/app/list.go`
- Modify: `internal/app/update.go`
- Modify: `internal/cli/wiring.go`
- Test: `internal/app/list_test.go`
- Test: `internal/app/update_test.go`
- Test: `internal/cli/service_test.go`

- [ ] **Step 1: 定义 target-aware latest 输入**

在 `internal/app/list.go` 添加：

```go
type LatestCheckTarget struct {
	Name       string
	Repo       string
	SourcePath string
	Package    cfgpkg.Section
}

type LatestInfoFunc func(target LatestCheckTarget) (LatestInfo, error)
```

`ListService.LatestInfo` 和 `UpdateService.LatestInfo` 改用 `LatestInfoFunc`。

- [ ] **Step 2: ListItem 携带 package section**

`ListItem` 增加：

```go
Package cfgpkg.Section
```

配置 package item 构造时保存 `pkg`；installed-only item 保持 zero value。

- [ ] **Step 3: 更新 outdated check**

`checkOutdatedItem` 调用：

```go
latest, err := latestInfo(LatestCheckTarget{
	Name:       item.Name,
	Repo:       item.Repo,
	SourcePath: item.SourcePath,
	Package:    item.Package,
})
```

同步更新现有 tests 的 callback 签名。

- [ ] **Step 4: 新增 template update 测试**

在 `internal/app/update_test.go` 添加 `TestUpdatePackageUpdatesTemplateManagedPackage`，断言 latest checker 收到 `Package.LatestURL`，并且 outdated 时调用 installer target `claude`。

- [ ] **Step 5: CLI wiring 支持 template latest**

在 `internal/cli/wiring.go` 的 latest checker dispatch 中：

- `sourceforge:*` 走现有 SourceForge latest。
- forge 走现有 Forge latest。
- `template:*` 使用 `urltemplate.LatestVersion` 或 `Finder.Latest`。
- 其他 repo 走 GitHub latest。

- [ ] **Step 6: 运行 tests**

Run:

```bash
go test ./internal/app ./internal/cli
```

Expected: PASS.

- [ ] **Step 7: 提交**

```bash
git add internal/app internal/cli
git commit -m "feat(update): check template package latest"
```

---

## Task 8: Metadata 请求和缓存边界

**Files:**
- Modify: `internal/source/urltemplate/finder.go`
- Optional: `internal/client/network.go`

- [ ] **Step 1: 固定缓存决策**

不要把任意 `latest_url` / `checksum_url_template` 自动加入 API cache 分类。原因：这些 URL 是任意站点 metadata，不是已知 provider API。

template metadata 请求必须复用现有 HTTP client，所以仍获得：

- `proxy_url`
- `disable_ssl`
- ghproxy 不生效，除非 URL 是 GitHub 下载/API 形态
- 现有 proxy notice 行为

- [ ] **Step 2: 添加代码注释**

在 `internal/source/urltemplate/finder.go` latest/checksum 请求处添加：

```go
// Template latest/checksum URLs are arbitrary site metadata. They use the
// shared HTTP client for proxy/SSL behavior, but do not force API-cache
// classification because arbitrary metadata URLs are not provider APIs.
```

- [ ] **Step 3: 运行 tests**

Run:

```bash
go test ./internal/client ./internal/source/urltemplate
```

Expected: PASS.

- [ ] **Step 4: 提交或跳过**

如果只有注释或无需代码变化，可合并到前一提交；如果单独提交：

```bash
git add internal/source/urltemplate
git commit -m "docs(source): clarify template metadata requests"
```

---

## Task 9: CLI 展示和 Smoke Tests

**Files:**
- Modify: `internal/cli/handlers.go`
- Modify: `internal/cli/service_test.go`

- [ ] **Step 1: package source 显示 template**

`packageSource` 中 `install.TargetTemplate` 返回 `"template"`。

- [ ] **Step 2: list 输出测试**

在 `internal/cli/service_test.go` 添加 installed package `template:claude`，断言 list 输出 source 为 `template`。

- [ ] **Step 3: update check wiring 测试**

添加 CLI 层 `update --check` 或 `list --outdated` 测试，断言 latest checker 收到 package section。

- [ ] **Step 4: 运行 CLI tests**

Run:

```bash
go test ./internal/cli
```

Expected: PASS.

- [ ] **Step 5: 提交**

```bash
git add internal/cli
git commit -m "feat(cli): show template package source"
```

---

## Task 10: 文档和示例

**Files:**
- Modify: `README.zh-CN.md`
- Modify: `README.md`
- Modify: `docs/config.zh-CN.md`
- Modify: `docs/config.md`
- Modify: `docs/example.eget.toml`
- Modify: `docs/architecture.md`

- [ ] **Step 1: 配置文档**

在 `docs/config.zh-CN.md` 和 `docs/config.md` 增加 Template Package Source 说明，包含 Claude Code 配置：

```toml
[packages.claude]
repo = "template:claude"
latest_url = "https://downloads.claude.ai/claude-code-releases/latest"
latest_format = "text"
url_template = "https://downloads.claude.ai/claude-code-releases/{version}/{os}-{arch}{libc}/claude{ext}"
os_map = { windows = "win32", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64" }
ext_map = { windows = ".exe", linux = "", darwin = "" }
libc_map = { glibc = "", musl = "-musl" }
checksum_url_template = "https://downloads.claude.ai/claude-code-releases/{version}/manifest.json"
checksum_format = "json"
checksum_json_path = "platforms.{os}-{arch}{libc}.checksum"
install_action = "run-asset"
install_args = ["install", "latest"]
```

明确 `run-asset` 不是通用 `post_install`，不会经过 shell。

- [ ] **Step 2: README 简要说明**

README 只放短例子和指向 config 文档的链接，不重复完整字段说明。

- [ ] **Step 3: architecture 更新**

在 `docs/architecture.md` 增加：

```text
Template sources are handled by internal/source/urltemplate. They render a configured URL from latest metadata and platform variables, then continue through the shared install pipeline.
```

并说明 `run-asset` 是执行已校验下载 asset 的 install mode。

- [ ] **Step 4: example config**

在 `docs/example.eget.toml` 增加 `[packages."claude"]` 示例。

- [ ] **Step 5: 提交 docs**

```bash
git add README.md README.zh-CN.md docs/config.md docs/config.zh-CN.md docs/example.eget.toml docs/architecture.md
git commit -m "docs add template package source"
```

---

## Task 11: 全量验证

**Files:**
- No planned source files unless verification finds issues.

- [ ] **Step 1: focused tests**

Run:

```bash
go test ./internal/source/urltemplate ./internal/config ./internal/install ./internal/app ./internal/cli ./internal/client
```

Expected: PASS.

- [ ] **Step 2: full tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: 检查工作区**

Run:

```bash
git status --short
git diff --stat
```

Expected: 只剩有意变更。不要包含无关的 `docs/TODO.md`。

- [ ] **Step 4: 验证 run-asset 覆盖**

Run:

```bash
go test ./internal/install -run RunAsset
```

Expected: PASS，且测试覆盖 checksum 成功后才调用 fake asset runner。

- [ ] **Step 5: 如有验证修复则提交**

如果 full test 后没有新变更，不创建空提交。如果有修复：

```bash
git add <fixed-files>
git commit -m "test verify template package source"
```

---

## 风险和约束

- `run-asset` 必须窄于 `post_install`：只能执行当前下载并校验通过的 asset，参数必须是数组。
- 不要把任意 template metadata URL 自动纳入 API cache。
- 不要引入 `platform_template`；平台组合直接写 `{os}-{arch}{libc}`。
- 不要引入 `install_cleanup`；缓存行为沿用现有 cache 设计。
- managed package `claude` 应按 package name 记录安装项，source identity 仍是 `template:claude`。
- 直接 URL 的 update 行为保持不变：没有 latest 发现能力。
- 完成实现后必须运行 `go test ./...`，因为改动触达 install/update 主链路。
