# pkg_templates 实施计划

> **给 agentic workers:** 必须使用子技能：推荐 `superpowers:subagent-driven-development`，也可以使用 `superpowers:executing-plans` 按任务执行本计划。步骤使用 checkbox (`- [ ]`) 跟踪进度。

**目标:** 增加 `pkg_templates`，让一个配置好的 package 模板可以通过规范 target `pkg-template:<template>:<package>` 和 CLI 短别名 `mydev:markview` 支持多个内部工具。

**架构:** 模板配置解析放在 config/app 层完成，然后继续交给现有 `urltemplate.Finder` 执行下载解析。新增一个小的 `internal/source/pkgtemplate` 包，只负责规范 target 解析和短别名解析。不新增下载器，也不实现 registry 服务。

**技术栈:** Go、gookit config/TOML loader、现有 `internal/config.Section`、现有 `internal/source/urltemplate`、现有 install/update/list/show app service，以及 `github.com/gookit/goutil/testutil/assert`。

---

## 方案复审

方案方向可行，但有一个关键实现约束：`latest_url` 当前会直接进入 `urltemplate.Finder.Latest()` 发起 HTTP 请求，请求前不会自动渲染模板变量。因此 `pkg_templates` 必须在创建 install options 前，先渲染 template section 中允许使用 `{name}` 的字符串字段，否则 finder 会请求包含字面量 `{name}` 的 URL。

实现边界保持保守：

- `internal/source/pkgtemplate` 只解析和规范化 target。
- `internal/config` 只负责加载、输出和合并 `PkgTemplates`。
- `internal/app` 负责解析短别名，并把 template section 展开成 package install options。
- `internal/install` 只需要识别规范 `pkg-template:` target，并转交给现有 `urltemplate.Finder`。

本实现会修改超过 3 个逻辑文件。按项目规则，执行代码变更前必须再次向用户确认实施范围。

## 文件地图

- Create: `internal/source/pkgtemplate/target.go`
  - 规范 target `pkg-template:<template>:<package>` 解析。
  - 基于已配置模板名解析短别名 `<template>:<package>`。
- Create: `internal/source/pkgtemplate/target_test.go`
  - parser 和 alias resolution 测试。
- Modify: `internal/config/model.go`
  - 给 `File` 增加 `PkgTemplates map[string]Section`。
- Modify: `internal/config/loader.go`
  - 初始化 `PkgTemplates`。
- Modify: `internal/config/gookit.go`
  - 加载/输出 `[pkg_templates]`、保留 root key、支持 path get/set。
- Modify: `internal/config/gookit_test.go`
  - `[pkg_templates.mydev]` round-trip 测试。
- Modify: `internal/install/options.go`
  - 增加 `TargetPkgTemplate` 识别。
- Modify: `internal/install/service.go`
  - 为规范 pkg-template target 选择 `urltemplate.Finder`。
- Modify: `internal/install/service_finder_test.go`
  - `pkg-template:mydev:markview` finder 测试。
- Modify: `internal/app/config.go`
  - `AddPackage` 和短别名/规范 pkg-template target 的 name inference。
- Modify: `internal/app/install_resolve.go`
  - 把已配置 package 和原始短别名解析为规范 pkg-template run target，并合并 URL template options。
- Modify: `internal/app/install_record.go`
  - 将 `TargetPkgTemplate` 视为 managed target 和 source-backed version target。
- Modify: `internal/app/install_config_test.go`
  - 已配置 package 和短别名 pkg-template 的 install option resolution 测试。
- Modify: `internal/app/install_add_test.go`
  - `install --add` 写入轻量 repo 引用。
- Modify: `internal/app/update_package_test.go`, `internal/app/list_outdated_test.go`, `internal/app/update_candidates_test.go`
  - 确保 latest check 收到已渲染的 template package 数据。
- Modify: `internal/cli/wiring.go`
  - latest checker 使用现有 URL template latest 流程处理 `pkg-template:`。
- Modify: `internal/cli/handlers.go`
  - 增加 `install --add mydev:markview` 输出名称推导。
- Modify: `internal/cli/install_handler_test.go`
  - CLI 短别名和输出测试。
- Modify: `docs/config.md`, `docs/config.zh-CN.md`, `docs/example.eget.toml`, `README.md`, `README.zh-CN.md`, `docs/TODO.md`
  - 实现完成后更新用户文档和任务跟踪。

## Task 0: Execution Scope Confirmation

**Files:**
- Read: `docs/superpowers/specs/2026-06-18-pkg-templates-design.md`
- Read: `docs/superpowers/plans/2026-06-18-pkg-templates.md`

- [x] **Step 1: Confirm implementation scope**

Send this exact confirmation request before editing Go implementation files:

```text
本实现会修改超过 3 个逻辑文件，涉及 config、install、app、cli、文档和测试。确认后我会按计划分阶段实现并在每个阶段提交。是否继续实施？
```

- [x] **Step 2: Verify clean implementation staging discipline**

Run:

```bash
git status --short
```

Expected: the existing unrelated dirty files may still appear. Do not stage unrelated files. Each task stages only files listed in that task.

## Task 1: Config Model And TOML Round Trip

**Files:**
- Modify: `internal/config/model.go`
- Modify: `internal/config/loader.go`
- Modify: `internal/config/gookit.go`
- Test: `internal/config/gookit_test.go`

- [x] **Step 1: Write failing config round-trip test**

Add this test to `internal/config/gookit_test.go` near `TestPackageURLTemplateFieldsRoundTrip`:

```go
func TestPkgTemplatesSectionRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	latestURL := "http://mydev.lan/tools/{name}/latest.yaml"
	latestFormat := "yaml"
	urlTemplate := "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"
	checksumURL := "http://mydev.lan/tools/{name}/{version}/checksums.json"
	checksumFormat := "json"
	checksumPath := "platforms.{os}-{arch}.checksum"

	cfg := NewFile()
	cfg.PkgTemplates["mydev"] = Section{
		LatestURL:           &latestURL,
		LatestFormat:        &latestFormat,
		URLTemplate:         &urlTemplate,
		OSMap:               map[string]string{"windows": "win", "linux": "linux"},
		ArchMap:             map[string]string{"amd64": "x64"},
		ExtMap:              map[string]string{"windows": ".exe", "linux": ""},
		ChecksumURLTemplate: &checksumURL,
		ChecksumFormat:      &checksumFormat,
		ChecksumJSONPath:    &checksumPath,
	}

	text, err := dumpConfigString(cfg)
	assert.NoErr(t, err)
	assert.Contains(t, text, "[pkg_templates.mydev]")
	assert.Contains(t, text, `latest_url = "http://mydev.lan/tools/{name}/latest.yaml"`)

	assert.NoErr(t, Save(configPath, cfg))
	loaded, err := LoadFile(configPath)
	assert.NoErr(t, err)
	template := loaded.PkgTemplates["mydev"]
	assert.Eq(t, latestURL, *template.LatestURL)
	assert.Eq(t, latestFormat, *template.LatestFormat)
	assert.Eq(t, urlTemplate, *template.URLTemplate)
	assert.Eq(t, map[string]string{"windows": "win", "linux": "linux"}, template.OSMap)
	assert.Eq(t, map[string]string{"amd64": "x64"}, template.ArchMap)
	assert.Eq(t, map[string]string{"windows": ".exe", "linux": ""}, template.ExtMap)
	assert.Eq(t, checksumURL, *template.ChecksumURLTemplate)
	assert.Eq(t, checksumFormat, *template.ChecksumFormat)
	assert.Eq(t, checksumPath, *template.ChecksumJSONPath)
}
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/config -run TestPkgTemplatesSectionRoundTrip -count=1
```

Expected: compile failure because `File.PkgTemplates` does not exist.

- [x] **Step 3: Add config model field and initialization**

In `internal/config/model.go`, add to `File`:

```go
PkgTemplates map[string]Section `toml:"pkg_templates" mapstructure:"pkg_templates"`
```

Place it near `Packages` because it is a package-related config section:

```go
Packages    map[string]Section    `toml:"packages" mapstructure:"packages"`
PkgTemplates map[string]Section   `toml:"pkg_templates" mapstructure:"pkg_templates"`
SDK         map[string]SDKSection `toml:"sdk" mapstructure:"sdk"`
```

Run `gofmt` later; alignment will be handled by the formatter.

In `internal/config/loader.go`, initialize:

```go
cfg.PkgTemplates = make(map[string]Section)
```

- [x] **Step 4: Load and dump `pkg_templates`**

In `internal/config/gookit.go`, update `decodeConfigFile` after packages:

```go
if err := cfg.MapOnExists("pkg_templates", &conf.PkgTemplates); err != nil {
	return nil, err
}
```

Update `encodeConfigFile` data:

```go
"pkg_templates": map[string]any{},
```

Then add:

```go
for name, section := range file.PkgTemplates {
	data["pkg_templates"].(map[string]any)[name] = sectionToMap(section)
}
```

Update `isReservedConfigRootKey`:

```go
case "global", "http_proxy", "api_cache", "ghproxy", "cache_mirror", "packages", "pkg_templates", "sdk":
	return true
```

- [x] **Step 5: Run config test**

Run:

```bash
gofmt -w internal/config/model.go internal/config/loader.go internal/config/gookit.go internal/config/gookit_test.go
go test ./internal/config -run 'TestPkgTemplatesSectionRoundTrip|TestSaveAndLoadRoundTrip|TestDumpConfigStringKeepsLegacyRepoSections' -count=1
```

Expected: PASS.

- [x] **Step 6: Commit config model**

Run:

```bash
git add internal/config/model.go internal/config/loader.go internal/config/gookit.go internal/config/gookit_test.go
git commit -m "feat(config): add pkg_templates section"
```

## Task 2: pkg-template Target Parser

**Files:**
- Create: `internal/source/pkgtemplate/target.go`
- Create: `internal/source/pkgtemplate/target_test.go`

- [ ] **Step 1: Write parser tests**

Create `internal/source/pkgtemplate/target_test.go`:

```go
package pkgtemplate

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantTemplate string
		wantPackage  string
		wantErr      string
	}{
		{name: "valid", input: "pkg-template:mydev:markview", wantTemplate: "mydev", wantPackage: "markview"},
		{name: "empty template", input: "pkg-template::markview", wantErr: "invalid pkg-template target"},
		{name: "empty package", input: "pkg-template:mydev:", wantErr: "invalid pkg-template target"},
		{name: "too many parts", input: "pkg-template:mydev:markview:extra", wantErr: "invalid pkg-template target"},
		{name: "wrong prefix", input: "template:markview", wantErr: "invalid pkg-template target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTarget(tt.input)
			if tt.wantErr != "" {
				assert.Err(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoErr(t, err)
			assert.Eq(t, tt.wantTemplate, got.Template)
			assert.Eq(t, tt.wantPackage, got.Package)
			assert.Eq(t, tt.input, got.Normalized)
		})
	}
}

func TestResolveAlias(t *testing.T) {
	templates := map[string]struct{}{"mydev": {}}
	tests := []struct {
		name    string
		input   string
		want    string
		wantOK  bool
	}{
		{name: "configured alias", input: "mydev:markview", want: "pkg-template:mydev:markview", wantOK: true},
		{name: "canonical", input: "pkg-template:mydev:markview", want: "pkg-template:mydev:markview", wantOK: true},
		{name: "unknown alias", input: "other:markview", wantOK: false},
		{name: "known provider sourceforge", input: "sourceforge:winmerge/stable", wantOK: false},
		{name: "known provider gitlab", input: "gitlab:owner/repo", wantOK: false},
		{name: "known provider template", input: "template:markview", wantOK: false},
		{name: "repo target", input: "owner/repo", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResolveAlias(tt.input, templates)
			assert.Eq(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Eq(t, tt.want, got)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/source/pkgtemplate -count=1
```

Expected: package directory has no implementation.

- [ ] **Step 3: Implement parser**

Create `internal/source/pkgtemplate/target.go`:

```go
package pkgtemplate

import (
	"fmt"
	"strings"
)

const Prefix = "pkg-template:"

type Target struct {
	Template   string
	Package    string
	Normalized string
}

func IsTarget(value string) bool {
	return strings.HasPrefix(value, Prefix)
}

func ParseTarget(value string) (Target, error) {
	if !IsTarget(value) {
		return Target{}, fmt.Errorf("invalid pkg-template target %q", value)
	}
	rest := strings.TrimPrefix(value, Prefix)
	parts := strings.Split(rest, ":")
	if len(parts) != 2 {
		return Target{}, fmt.Errorf("invalid pkg-template target %q", value)
	}
	template := strings.TrimSpace(parts[0])
	pkg := strings.TrimSpace(parts[1])
	if template == "" || pkg == "" {
		return Target{}, fmt.Errorf("invalid pkg-template target %q", value)
	}
	return Target{
		Template:   template,
		Package:    pkg,
		Normalized: Prefix + template + ":" + pkg,
	}, nil
}

func ResolveAlias(value string, templates map[string]struct{}) (string, bool) {
	if IsTarget(value) {
		target, err := ParseTarget(value)
		if err != nil {
			return "", false
		}
		return target.Normalized, true
	}
	prefix, name, ok := strings.Cut(value, ":")
	if !ok || prefix == "" || name == "" {
		return "", false
	}
	if isKnownPrefix(prefix) {
		return "", false
	}
	if _, ok := templates[prefix]; !ok {
		return "", false
	}
	return Prefix + prefix + ":" + name, true
}

func isKnownPrefix(prefix string) bool {
	switch prefix {
	case "sourceforge", "gitlab", "gitea", "forgejo", "template", "http", "https", "file":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run parser tests**

Run:

```bash
gofmt -w internal/source/pkgtemplate/target.go internal/source/pkgtemplate/target_test.go
go test ./internal/source/pkgtemplate -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit parser**

Run:

```bash
git add internal/source/pkgtemplate/target.go internal/source/pkgtemplate/target_test.go
git commit -m "feat(source): parse pkg-template targets"
```

## Task 3: Install Layer Finder Support

**Files:**
- Modify: `internal/install/options.go`
- Modify: `internal/install/service.go`
- Test: `internal/install/service_finder_test.go`

- [ ] **Step 1: Write finder test**

Add to `internal/install/service_finder_test.go`:

```go
func TestSelectFinderForPkgTemplateTarget(t *testing.T) {
	svc := NewDefaultService(nil, nil)
	opts := Options{
		URLTemplate: URLTemplateOptions{
			LatestURL:   "http://mydev.lan/tools/markview/latest.yaml",
			URLTemplate: "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}",
		},
	}

	finder, tool, err := svc.SelectFinder("pkg-template:mydev:markview", &opts)

	assert.NoErr(t, err)
	assert.Eq(t, "markview", tool)
	got, ok := finder.(*urltemplate.Finder)
	if !ok {
		t.Fatalf("finder type = %T, want *urltemplate.Finder", finder)
	}
	assert.Eq(t, "markview", got.Name)
	assert.Eq(t, "http://mydev.lan/tools/markview/latest.yaml", got.Config.LatestURL)
	assert.Eq(t, "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}", got.Config.URLTemplate)
}
```

If imports are missing, add:

```go
"github.com/inherelab/eget/internal/source/urltemplate"
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/install -run TestSelectFinderForPkgTemplateTarget -count=1
```

Expected: invalid argument or unknown target kind.

- [ ] **Step 3: Add target kind detection**

In `internal/install/options.go`, import:

```go
"github.com/inherelab/eget/internal/source/pkgtemplate"
```

Add target kind:

```go
TargetPkgTemplate TargetKind = "pkg_template"
```

Update `DetectTargetKind` before `TargetTemplate`:

```go
case pkgtemplate.IsTarget(target):
	return TargetPkgTemplate
```

Update `TargetKindDisplayName`:

```go
case TargetPkgTemplate:
	return "pkg-template"
```

- [ ] **Step 4: Select URL template finder for pkg-template**

In `internal/install/service.go`, import:

```go
"github.com/inherelab/eget/internal/source/pkgtemplate"
```

Add a `case TargetPkgTemplate` next to `TargetTemplate`:

```go
case TargetPkgTemplate:
	templateTarget, err := pkgtemplate.ParseTarget(target)
	if err != nil {
		return nil, "", err
	}
	getter := urltemplate.HTTPGetter(NewHTTPGetter(*opts))
	if s.TemplateGetterFactory != nil {
		getter = s.TemplateGetterFactory(*opts)
	}
	goos, goarch, libc := urltemplate.EffectiveSystem(opts.System, runtime.GOOS, runtime.GOARCH, urltemplate.DetectLibc, urltemplate.FixDarwinRosetta)
	return &urltemplate.Finder{
		Name:   templateTarget.Package,
		Config: urlTemplateConfigFromOptions(opts.URLTemplate),
		Tag:    opts.Tag,
		GOOS:   goos,
		GOARCH: goarch,
		Libc:   libc,
		Getter: getter,
	}, templateTarget.Package, nil
```

- [ ] **Step 5: Run install tests**

Run:

```bash
gofmt -w internal/install/options.go internal/install/service.go internal/install/service_finder_test.go
go test ./internal/install -run 'TestSelectFinderForPkgTemplateTarget|TestSelectFinderForTemplateTarget' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit install finder support**

Run:

```bash
git add internal/install/options.go internal/install/service.go internal/install/service_finder_test.go
git commit -m "feat(install): support pkg-template targets"
```

## Task 4: App Resolution And Template Expansion

**Files:**
- Modify: `internal/app/install_resolve.go`
- Test: `internal/app/install_config_test.go`

- [ ] **Step 1: Write install resolution tests**

Add to `internal/app/install_config_test.go`:

```go
func TestInstallTargetResolvesPkgTemplateShortAlias(t *testing.T) {
	cfg := mustLoadFromString(t, `
[pkg_templates.mydev]
latest_url = "http://mydev.lan/tools/{name}/latest.yaml"
latest_format = "yaml"
url_template = "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"
os_map = { windows = "win" }
arch_map = { amd64 = "x64" }
ext_map = { windows = ".exe" }
`)
	runner := &fakeRunner{result: RunResult{URL: "http://mydev.lan/tools/markview/markview-win-x64.exe", Tool: "markview", ExtractedFiles: []string{"./markview.exe"}}}
	svc := Service{Runner: runner, LoadConfig: func() (*cfgpkg.File, error) { return cfg, nil }}

	_, err := svc.InstallTarget("mydev:markview", install.Options{})

	assert.NoErr(t, err)
	assert.Eq(t, "pkg-template:mydev:markview", runner.target)
	assert.Eq(t, "http://mydev.lan/tools/markview/latest.yaml", runner.opts.URLTemplate.LatestURL)
	assert.Eq(t, "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}", runner.opts.URLTemplate.URLTemplate)
	assert.Eq(t, map[string]string{"windows": "win"}, runner.opts.URLTemplate.OSMap)
	assert.Eq(t, map[string]string{"amd64": "x64"}, runner.opts.URLTemplate.ArchMap)
	assert.Eq(t, map[string]string{"windows": ".exe"}, runner.opts.URLTemplate.ExtMap)
}

func TestInstallTargetResolvesConfiguredPkgTemplatePackageWithOverride(t *testing.T) {
	cfg := mustLoadFromString(t, `
[global]
target = "~/global-bin"

[pkg_templates.mydev]
latest_url = "http://mydev.lan/tools/{name}/latest.yaml"
latest_format = "yaml"
url_template = "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"
os_map = { windows = "win" }
arch_map = { amd64 = "x64" }
ext_map = { windows = ".exe" }

[packages.markview]
repo = "pkg-template:mydev:markview"
desc = "Markdown preview"
target = "~/package-bin"
url_template = "http://override/{name}-{version}{ext}"
ext_map = { windows = ".zip" }
`)
	runner := &fakeRunner{result: RunResult{URL: "http://mydev.lan/tools/markview/markview-windows-amd64.zip", Tool: "markview", ExtractedFiles: []string{"./markview.exe"}}}
	svc := Service{Runner: runner, LoadConfig: func() (*cfgpkg.File, error) { return cfg, nil }}

	_, err := svc.InstallTarget("markview", install.Options{})

	assert.NoErr(t, err)
	assert.Eq(t, "pkg-template:mydev:markview", runner.target)
	assert.Eq(t, "http://mydev.lan/tools/markview/latest.yaml", runner.opts.URLTemplate.LatestURL)
	assert.Eq(t, "http://override/{name}-{version}{ext}", runner.opts.URLTemplate.URLTemplate)
	assert.Eq(t, map[string]string{"windows": "win"}, runner.opts.URLTemplate.OSMap)
	assert.Eq(t, map[string]string{"amd64": "x64"}, runner.opts.URLTemplate.ArchMap)
	assert.Eq(t, map[string]string{"windows": ".zip"}, runner.opts.URLTemplate.ExtMap)
	assert.Contains(t, runner.opts.Output, "package-bin")
}

func TestInstallTargetShortAliasUsesConfiguredPkgTemplateOverride(t *testing.T) {
	cfg := mustLoadFromString(t, `
[pkg_templates.mydev]
latest_url = "http://mydev.lan/tools/{name}/latest.yaml"
url_template = "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"
ext_map = { windows = ".exe" }

[packages.markview]
repo = "pkg-template:mydev:markview"
ext_map = { windows = ".zip" }
`)
	runner := &fakeRunner{result: RunResult{URL: "http://mydev.lan/tools/markview/markview-windows-amd64.zip", Tool: "markview", ExtractedFiles: []string{"./markview.exe"}}}
	svc := Service{Runner: runner, LoadConfig: func() (*cfgpkg.File, error) { return cfg, nil }}

	_, err := svc.InstallTarget("mydev:markview", install.Options{})

	assert.NoErr(t, err)
	assert.Eq(t, "pkg-template:mydev:markview", runner.target)
	assert.Eq(t, map[string]string{"windows": ".zip"}, runner.opts.URLTemplate.ExtMap)
}

func TestInstallTargetShortAliasUsesCustomNamedPkgTemplatePackage(t *testing.T) {
	cfg := mustLoadFromString(t, `
[pkg_templates.mydev]
latest_url = "http://mydev.lan/tools/{name}/latest.yaml"
url_template = "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"

[packages.mv]
repo = "pkg-template:mydev:markview"
target = "~/custom-bin"
`)
	runner := &fakeRunner{result: RunResult{URL: "http://mydev.lan/tools/markview/markview-windows-amd64", Tool: "markview", ExtractedFiles: []string{"./markview"}}}
	svc := Service{Runner: runner, LoadConfig: func() (*cfgpkg.File, error) { return cfg, nil }}

	_, err := svc.InstallTarget("mydev:markview", install.Options{})

	assert.NoErr(t, err)
	assert.Eq(t, "pkg-template:mydev:markview", runner.target)
	assert.Contains(t, runner.opts.Output, "custom-bin")
}
```

- [ ] **Step 2: Run install resolution tests to verify they fail**

Run:

```bash
go test ./internal/app -run 'TestInstallTargetResolvesPkgTemplateShortAlias|TestInstallTargetResolvesConfiguredPkgTemplatePackageWithOverride|TestInstallTargetShortAliasUsesConfiguredPkgTemplateOverride|TestInstallTargetShortAliasUsesCustomNamedPkgTemplatePackage' -count=1
```

Expected: target resolution fails or does not render `latest_url`.

- [ ] **Step 3: Implement app resolution helpers**

In `internal/app/install_resolve.go`, add imports:

```go
"sort"

"github.com/inherelab/eget/internal/source/pkgtemplate"
"github.com/inherelab/eget/internal/source/urltemplate"
```

Add helper:

```go
func configuredTemplateNames(cfg *cfgpkg.File) map[string]struct{} {
	names := make(map[string]struct{}, len(cfg.PkgTemplates))
	for name := range cfg.PkgTemplates {
		names[name] = struct{}{}
	}
	return names
}
```

Before package lookup in `resolveInstallRequestWithConfig`, normalize raw short alias:

```go
if normalized, ok := pkgtemplate.ResolveAlias(target, configuredTemplateNames(cfg)); ok {
	target = normalized
}
```

After alias normalization and before the direct package-name lookup, map canonical pkg-template targets back to a configured package when one exists:

```go
if pkgName, ok := configuredPkgTemplatePackageName(cfg, target); ok {
	target = pkgName
}
```

Implement:

```go
func configuredPkgTemplatePackageName(cfg *cfgpkg.File, target string) (string, bool) {
	parsed, err := pkgtemplate.ParseTarget(target)
	if err != nil {
		return "", false
	}
	if pkg, ok := cfg.Packages[parsed.Package]; ok && util.DerefString(pkg.Repo) == parsed.Normalized {
		return parsed.Package, true
	}

	names := make([]string, 0, len(cfg.Packages))
	for name := range cfg.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if util.DerefString(cfg.Packages[name].Repo) == parsed.Normalized {
			return name, true
		}
	}
	return "", false
}
```

这确保 `eget install mydev:markview` 和 `eget install pkg-template:mydev:markview` 在 package 已经加入配置后，仍然会使用已配置 package 的覆盖字段。优先匹配精确的 `[packages.<package>]`，再按名称排序扫描所有 configured packages，这样 `[packages.mv] repo = "pkg-template:mydev:markview"` 这类自定义 package 名也能工作。

修改 `resolveInstallOptionsWithConfig`，让它接收显式的中间层 section：

```go
func (s Service) resolveInstallOptionsWithConfig(cfg *cfgpkg.File, target string, source cfgpkg.Section, pkg cfgpkg.Section, cli install.Options, preferCacheDir bool) (install.Options, error)
```

Inside that function, remove the local `repoSection := cfg.Repos[repoKey]` lookup. Use the passed `source` as the middle layer for proxy resolution and option merging:

```go
proxy := cfgpkg.ResolveHTTPProxy(cfg, cfgpkg.ProxyResolveOptions{
	NoProxy:     cli.NoProxy,
	EnvNoProxy:  os.Getenv("NO_PROXY"),
	OverrideURL: cli.ProxyURL,
	PackageURL:  util.DerefString(pkg.ProxyURL),
	RepoURL:     util.DerefString(source.ProxyURL),
})
source.ProxyURL = nil
pkg.ProxyURL = nil
```

Then merge with existing precedence:

```go
merged := cfgpkg.MergeInstallOptions(global, source, pkg, cfgpkg.CLIOverrides{
	// keep the existing override list unchanged
})
```

When target is a configured package, resolve the source section before merging:

```go
source, err := resolveInstallSourceSection(cfg, repo)
if err != nil {
	return "", "", install.Options{}, err
}
opts, err := s.resolveInstallOptionsWithConfig(cfg, repo, source, pkg, cli, preferCacheDir)
```

For raw target path, normalize short aliases, set a default package name for canonical pkg-template targets, resolve the source section, then merge:

```go
target, pkg = resolveRawPkgTemplateTarget(cfg, target, pkg)
source, err := resolveInstallSourceSection(cfg, target)
if err != nil {
	return "", "", install.Options{}, err
}
opts, err := s.resolveInstallOptionsWithConfig(cfg, target, source, pkg, cli, preferCacheDir)
```

Implement:

```go
func resolveInstallSourceSection(cfg *cfgpkg.File, repo string) (cfgpkg.Section, error) {
	target, err := pkgtemplate.ParseTarget(repo)
	if err != nil {
		if repoKey, normErr := install.NormalizeRepoTarget(repo); normErr == nil {
			return cfg.Repos[repoKey], nil
		}
		return cfgpkg.Section{}, nil
	}
	template, ok := cfg.PkgTemplates[target.Template]
	if !ok {
		return cfgpkg.Section{}, fmt.Errorf("pkg template %q is not configured", target.Template)
	}
	if util.DerefString(template.LatestURL) == "" {
		return cfgpkg.Section{}, fmt.Errorf("pkg template %q for package %q has no latest_url", target.Template, target.Package)
	}
	if util.DerefString(template.URLTemplate) == "" {
		return cfgpkg.Section{}, fmt.Errorf("pkg template %q for package %q has no url_template", target.Template, target.Package)
	}
	return renderPkgTemplateSection(template, target.Package)
}

func resolveRawPkgTemplateTarget(cfg *cfgpkg.File, target string, pkg cfgpkg.Section) (string, cfgpkg.Section) {
	if normalized, ok := pkgtemplate.ResolveAlias(target, configuredTemplateNames(cfg)); ok {
		target = normalized
	}
	if parsed, err := pkgtemplate.ParseTarget(target); err == nil && pkg.Name == nil {
		pkg.Name = util.StringPtr(parsed.Package)
	}
	return target, pkg
}
```

Implement rendering only for string fields that can be requested before `Finder.Find()` resolves version/platform:

```go
func renderPkgTemplateSection(section cfgpkg.Section, name string) (cfgpkg.Section, error) {
	vars := map[string]string{"name": name}
	render := func(ptr *string) (*string, error) {
		if ptr == nil {
			return nil, nil
		}
		value, err := urltemplate.Render(*ptr, vars)
		if err != nil {
			return nil, err
		}
		return &value, nil
	}
	var err error
	if section.LatestURL, err = render(section.LatestURL); err != nil {
		return cfgpkg.Section{}, err
	}
	if section.LatestJSONPath, err = render(section.LatestJSONPath); err != nil {
		return cfgpkg.Section{}, err
	}
	return section, nil
}
```

Do not render `url_template` at this stage; `urltemplate.Finder` already renders it with `{name}`, `{version}`, `{os}`, `{arch}`, `{ext}`, and `{libc}`.

Update the other `resolveInstallOptionsWithConfig` call sites by passing the source section:

```go
source, err := resolveInstallSourceSection(cfg, target)
if err != nil {
	return install.Options{}, err
}
return s.resolveInstallOptionsWithConfig(cfg, target, source, cfgpkg.Section{}, cli, preferCacheDir)
```

- [ ] **Step 4: Run focused tests**

Run:

```bash
gofmt -w internal/app/install_resolve.go internal/app/install_config_test.go
go test ./internal/app -run 'TestInstallTargetResolvesPkgTemplateShortAlias|TestInstallTargetResolvesConfiguredPkgTemplatePackageWithOverride|TestInstallTargetShortAliasUsesConfiguredPkgTemplateOverride|TestInstallTargetShortAliasUsesCustomNamedPkgTemplatePackage|TestInstallTargetMergesTemplatePackageOptions' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit app resolution**

Run:

```bash
git add internal/app/install_resolve.go internal/app/install_config_test.go
git commit -m "feat(app): resolve pkg-template packages"
```

## Task 5: Add And install --add Lightweight References

**Files:**
- Modify: `internal/app/config.go`
- Modify: `internal/app/install.go`
- Modify: `internal/app/install_record.go`
- Test: `internal/app/add_test.go`
- Test: `internal/app/install_add_test.go`
- Modify: `internal/cli/handlers.go`
- Test: `internal/cli/install_handler_test.go`

- [ ] **Step 1: Write add config test**

Add to `internal/app/add_test.go`:

```go
func TestAddPackageWritesPkgTemplateReference(t *testing.T) {
	var saved *cfgpkg.File
	cfg := cfgpkg.NewFile()
	cfg.PkgTemplates["mydev"] = cfgpkg.Section{
		LatestURL:   util.StringPtr("http://mydev.lan/tools/{name}/latest.yaml"),
		URLTemplate: util.StringPtr("http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"),
	}
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error {
			saved = file
			return nil
		},
	}

	err := svc.AddPackage("mydev:markview", "", install.Options{})

	assert.NoErr(t, err)
	pkg := saved.Packages["markview"]
	assert.Eq(t, "pkg-template:mydev:markview", *pkg.Repo)
	assert.Eq(t, "markview", *pkg.Name)
	assert.Nil(t, pkg.LatestURL)
	assert.Nil(t, pkg.URLTemplate)
}
```

Adjust imports if needed:

```go
cfgpkg "github.com/inherelab/eget/internal/config"
"github.com/inherelab/eget/internal/install"
"github.com/inherelab/eget/internal/util"
```

- [ ] **Step 2: Write install --add test**

Add to `internal/app/install_add_test.go`:

```go
func TestInstallTargetWithAddRecordsPkgTemplatePackage(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:            "http://mydev.lan/tools/markview/markview-windows-amd64.exe",
			Tool:           "markview",
			ExtractedFiles: []string{"./markview.exe"},
		},
	}
	config := &fakeConfigRecorder{}
	cfg := cfgpkg.NewFile()
	cfg.PkgTemplates["mydev"] = cfgpkg.Section{
		LatestURL:   util.StringPtr("http://mydev.lan/tools/{name}/latest.yaml"),
		URLTemplate: util.StringPtr("http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"),
	}
	svc := Service{
		Runner: runner,
		Config: config,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}

	_, err := svc.InstallTarget("mydev:markview", install.Options{}, InstallExtras{AddToConfig: true, PackageOpts: install.Options{}})

	assert.NoErr(t, err)
	assert.Eq(t, "pkg-template:mydev:markview", runner.target)
	assert.Eq(t, "pkg-template:mydev:markview", config.repo)
	assert.Eq(t, "markview", config.name)
	assert.Eq(t, "markview", config.opts.Name)
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./internal/app -run 'TestAddPackageWritesPkgTemplateReference|TestInstallTargetWithAddRecordsPkgTemplatePackage' -count=1
```

Expected: name inference or managed target validation fails.

- [ ] **Step 4: Implement AddPackage short alias support**

In `internal/app/config.go`, import `pkgtemplate`.

Inside `ConfigService.AddPackage`, after loading `cfg`, normalize short alias before `ResolvePackageConfig`:

```go
if normalized, ok := pkgtemplate.ResolveAlias(repo, configuredTemplateNames(cfg)); ok {
	repo = normalized
}
```

In `ResolvePackageConfig`, add canonical pkg-template handling before forge/github fallback:

```go
if pkgTarget, pkgErr := pkgtemplate.ParseTarget(repo); pkgErr == nil {
	repo = pkgTarget.Normalized
	if name == "" {
		name = pkgTarget.Package
	}
}
```

This requires importing `internal/source/pkgtemplate`.

- [ ] **Step 5: Allow pkg-template as managed config target**

In `internal/app/install_record.go`, update `isManagedConfigTarget`:

```go
case install.TargetRepo, install.TargetGitHubURL, install.TargetSourceForge, install.TargetForge, install.TargetTemplate, install.TargetPkgTemplate:
	return true
```

Update template checks:

```go
isTemplate := install.DetectTargetKind(runTarget) == install.TargetTemplate || install.DetectTargetKind(runTarget) == install.TargetPkgTemplate
```

Update `shouldFetchReleaseInfo`:

```go
if kind := install.DetectTargetKind(repo); kind == install.TargetTemplate || kind == install.TargetPkgTemplate {
	return false
}
```

- [ ] **Step 6: Name inference before install**

In `internal/app/install.go`, keep the existing early `inferAddPackageName` call for normal repo targets. Then, after `resolveInstallRequest` returns `recordTarget`, fill missing `install --add` package metadata from the resolved record target.

Add:

```go
if len(extras) > 0 && extras[0].AddToConfig && extras[0].PackageName == "" {
	extras[0].PackageName = recordTarget
	extras[0].PackageOpts.Name = recordTarget
}
```

Place it after `resolveInstallRequest` and before `installResolvedTarget`.

- [ ] **Step 7: CLI add output name inference**

In `internal/app/config.go`, add this helper near `ResolvePackageName`:

```go
func ResolvePackageNameWithConfig(cfg *cfgpkg.File, repo, name string) (string, error)
```

Implement it by resolving short aliases with `pkgtemplate.ResolveAlias(repo, configuredTemplateNames(cfg))`, then calling `ResolvePackageConfig`.

In `internal/cli/handlers.go`, after `cfgService.AddPackage` succeeds, use `ResolvePackageNameWithConfig` when plain `app.ResolvePackageName` fails. Load config via `s.cfgService.Load` if it is set; otherwise use `cfgpkg.Load()`.

Add CLI test in `internal/cli/install_handler_test.go`:

```go
func TestHandleAddPrintsPkgTemplateAliasName(t *testing.T) {
	var saved *cfgpkg.File
	cfg := cfgpkg.NewFile()
	cfg.PkgTemplates["mydev"] = cfgpkg.Section{
		LatestURL:   util.StringPtr("http://mydev.lan/tools/{name}/latest.yaml"),
		URLTemplate: util.StringPtr("http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"),
	}
	svc := &cliService{
		cfgService: app.ConfigService{
			Load: func() (*cfgpkg.File, error) { return cfg, nil },
			Save: func(path string, file *cfgpkg.File) error {
				saved = file
				return nil
			},
		},
	}
	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("add", &AddOptions{Target: "mydev:markview"})

	assert.NoErr(t, err)
	assert.Eq(t, "pkg-template:mydev:markview", *saved.Packages["markview"].Repo)
	assert.Contains(t, out.String(), "Added package config: markview -> mydev:markview")
}
```

- [ ] **Step 8: Run focused tests**

Run:

```bash
gofmt -w internal/app/config.go internal/app/install.go internal/app/install_record.go internal/app/add_test.go internal/app/install_add_test.go internal/cli/handlers.go internal/cli/install_handler_test.go
go test ./internal/app -run 'TestAddPackageWritesPkgTemplateReference|TestInstallTargetWithAddRecordsPkgTemplatePackage|TestInstallTargetWithAddRecordsManagedPackage' -count=1
go test ./internal/cli -run 'TestHandleAddPrintsPkgTemplateAliasName|TestHandleAddPrintsInferredPackageName' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit add support**

Run:

```bash
git add internal/app/config.go internal/app/install.go internal/app/install_record.go internal/app/add_test.go internal/app/install_add_test.go internal/cli/handlers.go internal/cli/install_handler_test.go
git commit -m "feat(app): add pkg-template package references"
```

## Task 6: Latest Checks, Update, List, And Show

**Files:**
- Modify: `internal/cli/wiring.go`
- Modify: `internal/app/list.go`
- Modify: `internal/app/update_target.go`
- Modify: `internal/app/show.go`
- Test: `internal/app/list_outdated_test.go`
- Test: `internal/app/update_candidates_test.go`
- Test: `internal/app/update_package_test.go`
- Test: `internal/app/show_test.go`

- [ ] **Step 1: Write latest-check tests**

Add to `internal/app/list_outdated_test.go`:

```go
func TestListOutdatedPackagesChecksPkgTemplateRepo(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.PkgTemplates["mydev"] = cfgpkg.Section{
		LatestURL:   util.StringPtr("http://mydev.lan/tools/{name}/latest.yaml"),
		URLTemplate: util.StringPtr("http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"),
	}
	cfg.Packages["markview"] = cfgpkg.Section{Repo: util.StringPtr("pkg-template:mydev:markview")}
	installed := &storepkg.Config{Installed: map[string]storepkg.Entry{
		"markview": {Repo: "pkg-template:mydev:markview", Target: "pkg-template:mydev:markview", Tag: "1.0.0"},
	}}
	var got LatestCheckTarget
	svc := ListService{
		LoadConfig: func() (*cfgpkg.File, error) { return cfg, nil },
		LoadInstalled: func() (*storepkg.Config, error) { return installed, nil },
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			got = target
			return LatestInfo{Tag: "1.1.0"}, nil
		},
	}

	items, failures, checked, err := svc.ListOutdatedPackages()

	assert.NoErr(t, err)
	assert.Eq(t, 0, len(failures))
	assert.Eq(t, 1, checked)
	assert.Eq(t, 1, len(items))
	assert.Eq(t, "pkg-template:mydev:markview", got.Repo)
	assert.Eq(t, "http://mydev.lan/tools/markview/latest.yaml", *got.Package.LatestURL)
}
```

Add similar target-specific update candidate test to `internal/app/update_candidates_test.go`:

```go
func TestListUpdateCandidatesChecksPkgTemplateTarget(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.PkgTemplates["mydev"] = cfgpkg.Section{
		LatestURL:   util.StringPtr("http://mydev.lan/tools/{name}/latest.yaml"),
		URLTemplate: util.StringPtr("http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"),
	}
	cfg.Packages["markview"] = cfgpkg.Section{Repo: util.StringPtr("pkg-template:mydev:markview")}
	installed := &storepkg.Config{Installed: map[string]storepkg.Entry{
		"markview": {Repo: "pkg-template:mydev:markview", Target: "pkg-template:mydev:markview", Tag: "1.0.0"},
	}}
	var got LatestCheckTarget
	svc := UpdateService{
		LoadConfig: func() (*cfgpkg.File, error) { return cfg, nil },
		LoadInstalled: func() (*storepkg.Config, error) { return installed, nil },
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			got = target
			return LatestInfo{Tag: "1.1.0"}, nil
		},
	}

	items, failures, checked, err := svc.ListUpdateCandidatesForTargets([]string{"markview"})

	assert.NoErr(t, err)
	assert.Eq(t, 0, len(failures))
	assert.Eq(t, 1, checked)
	assert.Eq(t, 1, len(items))
	assert.Eq(t, "pkg-template:mydev:markview", got.Repo)
	assert.Eq(t, "http://mydev.lan/tools/markview/latest.yaml", *got.Package.LatestURL)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/app -run 'TestListOutdatedPackagesChecksPkgTemplateRepo|TestListUpdateCandidatesChecksPkgTemplateTarget' -count=1
```

Expected: latest target package section does not include rendered template fields.

- [ ] **Step 3: Enrich list/update items with pkg-template section**

In `internal/app/list.go`, enrich configured list items immediately after they are built from `cfg.Packages`, before writing them into `byName`. Add this helper:

```go
func resolveListItemPackageTemplate(cfg *cfgpkg.File, item ListItem) ListItem {
	source, err := resolveInstallSourceSection(cfg, item.Repo)
	if err != nil {
		return item
	}
	if util.DerefString(source.URLTemplate) == "" && util.DerefString(source.LatestURL) == "" {
		return item
	}
	item.Package = latestCheckSectionFromSourceAndPackage(source, item.Package)
	return item
}

func latestCheckSectionFromSourceAndPackage(source, pkg cfgpkg.Section) cfgpkg.Section {
	merged := cfgpkg.MergeInstallOptions(cfgpkg.Section{}, source, pkg, cfgpkg.CLIOverrides{})
	section := pkg
	section.LatestURL = stringPtrIfNotEmpty(merged.LatestURL)
	section.LatestFormat = stringPtrIfNotEmpty(merged.LatestFormat)
	section.LatestJSONPath = stringPtrIfNotEmpty(merged.LatestJSONPath)
	section.VersionRegex = stringPtrIfNotEmpty(merged.VersionRegex)
	section.URLTemplate = stringPtrIfNotEmpty(merged.URLTemplate)
	section.OSMap = util.CloneStringMap(merged.OSMap)
	section.ArchMap = util.CloneStringMap(merged.ArchMap)
	section.ExtMap = util.CloneStringMap(merged.ExtMap)
	section.LibcMap = util.CloneStringMap(merged.LibcMap)
	section.ChecksumURLTemplate = stringPtrIfNotEmpty(merged.ChecksumURLTemplate)
	section.ChecksumFormat = stringPtrIfNotEmpty(merged.ChecksumFormat)
	section.ChecksumJSONPath = stringPtrIfNotEmpty(merged.ChecksumJSONPath)
	section.ChecksumRegex = stringPtrIfNotEmpty(merged.ChecksumRegex)
	section.InstallAction = stringPtrIfNotEmpty(merged.InstallAction)
	section.InstallArgs = append([]string(nil), merged.InstallArgs...)
	return section
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return util.StringPtr(value)
}
```

This helper intentionally uses the same package-private `resolveInstallSourceSection` from `install_resolve.go`, because `list.go` is in the same `app` package. It also reuses `cfgpkg.MergeInstallOptions` so latest/list/update precedence stays aligned with install precedence.

When building each configured list item, call:

```go
item = resolveListItemPackageTemplate(cfg, item)
```

不要在 `findUpdateTarget` 或 `ListUpdateCandidatesForTargets` 里重复做 enrichment。这些路径已经通过 `ListService.ListPackages()` 解析 target，因此 `ListPackages` 里的单点 enrichment 是 list、update-all 和指定 target update 检查的共同来源。

- [ ] **Step 4: CLI latest checker handles pkg-template**

In `internal/cli/wiring.go`, import `pkgtemplate` and add before `urltemplate.ParseTarget`:

```go
if pkgTarget, err := pkgtemplate.ParseTarget(repo); err == nil {
	finder := urltemplate.Finder{
		Name:   pkgTarget.Package,
		Config: urlTemplateConfigFromSection(target.Package),
		Getter: client.NewHTTPGetter(defaultClientOpts),
	}
	info, err := finder.Latest()
	if err != nil {
		return app.LatestInfo{}, err
	}
	return app.LatestInfo{Tag: info.Version, PublishedAt: info.PublishedAt}, nil
}
```

Keep existing `template:` behavior unchanged.

- [ ] **Step 5: Show displays lightweight repo**

If `show` already displays `pkg-template:mydev:markview` through existing repo fields, only add a regression test in `internal/app/show_test.go`:

```go
func TestShowPackageDisplaysPkgTemplateRepo(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.Packages["markview"] = cfgpkg.Section{
		Repo: util.StringPtr("pkg-template:mydev:markview"),
		Desc: util.StringPtr("Markdown preview"),
	}
	svc := ShowService{
		LoadConfig: func() (*cfgpkg.File, error) { return cfg, nil },
		LoadInstalled: func() (*storepkg.Config, error) {
			return &storepkg.Config{Installed: map[string]storepkg.Entry{}}, nil
		},
	}

	got, err := svc.ShowPackage("markview")

	assert.NoErr(t, err)
	assert.Eq(t, "markview", got.Name)
	assert.Eq(t, "pkg-template:mydev:markview", got.Repo)
	assert.Eq(t, "Markdown preview", got.Desc)
	assert.Eq(t, "", got.RepoURL)
}
```

- [ ] **Step 6: Run focused app/cli tests**

Run:

```bash
gofmt -w internal/cli/wiring.go internal/app/list.go internal/app/update_target.go internal/app/show.go internal/app/list_outdated_test.go internal/app/update_candidates_test.go internal/app/update_package_test.go internal/app/show_test.go
go test ./internal/app -run 'PkgTemplate|TemplatePackage|ListOutdatedPackagesChecksPkgTemplateRepo|ListUpdateCandidatesChecksPkgTemplateTarget' -count=1
go test ./internal/cli -run 'Template|PkgTemplate' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit latest/update/show support**

Run:

```bash
git add internal/cli/wiring.go internal/app/list.go internal/app/update_target.go internal/app/show.go internal/app/list_outdated_test.go internal/app/update_candidates_test.go internal/app/update_package_test.go internal/app/show_test.go
git commit -m "feat(update): check pkg-template packages"
```

## Task 7: Documentation And Examples

**Files:**
- Modify: `docs/config.md`
- Modify: `docs/config.zh-CN.md`
- Modify: `docs/example.eget.toml`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/TODO.md`
- Modify: `docs/superpowers/specs/2026-06-18-pkg-templates-design.md`

- [ ] **Step 1: Update config docs**

In `docs/config.zh-CN.md`, add `[pkg_templates.<name>]` to the config section list near `[packages.<name>]`.

Add a section after “Template Package Source”:

````markdown
### pkg_templates

`[pkg_templates.<name>]` 用于复用一组 package template 字段，适合内部工具发布规则一致、只有工具名不同的场景。

```toml
[pkg_templates.mydev]
latest_url = "http://mydev.lan/tools/{name}/latest.yaml"
latest_format = "yaml"
url_template = "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"
ext_map = { windows = ".exe", linux = "", darwin = "" }

[packages.markview]
repo = "pkg-template:mydev:markview"
```

也可以直接使用短别名：

```bash
eget add mydev:markview
eget install mydev:markview
eget install --add mydev:markview
```

短别名只在 `mydev` 匹配已配置的 `[pkg_templates.mydev]` 时生效。落盘配置保留轻量引用 `repo = "pkg-template:mydev:markview"`，不会把 URL 展开写入 package。
````

Add equivalent English content to `docs/config.md`.

- [ ] **Step 2: Update example config**

In `docs/example.eget.toml`, add:

```toml
# Reusable package template for internal tools with the same release layout.
[pkg_templates.mydev]
latest_url = "http://mydev.lan/tools/{name}/latest.yaml"
latest_format = "yaml"
url_template = "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"
ext_map = { windows = ".exe", linux = "", darwin = "" }
```

- [ ] **Step 3: Update README files**

In `README.zh-CN.md`, add a short example near the template package source section:

````markdown
如果多个内部工具使用同一发布规则，可以用 `pkg_templates` 复用配置：

```toml
[pkg_templates.mydev]
latest_url = "http://mydev.lan/tools/{name}/latest.yaml"
url_template = "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"
```

```bash
eget add mydev:markview
eget install mydev:markview
```
````

Add equivalent English content to `README.md`.

- [ ] **Step 4: Update design spec revision note**

In `docs/superpowers/specs/2026-06-18-pkg-templates-design.md`, add a short “实施确认” section:

```markdown
## 实施确认

实施计划见 [2026-06-18-pkg-templates.md](../plans/2026-06-18-pkg-templates.md)。实现时需保持 `pkg_templates` 只做本地模板复用，不扩展为 registry/index/search。
```

- [ ] **Step 5: Update tracker**

在 `docs/TODO.md` 中，将旧 registry 板块文案替换为 `pkg_templates` 文案；如果实现尚未完成，只标记设计文档和实施计划已完成。使用以下结构：

````markdown
## [ ] pkg_templates 功能

内部工具发布格式一致时，通过 `[pkg_templates.<name>]` 复用 `latest_url`、`url_template` 等 template package 字段。

```toml
[pkg_templates.mydev]
latest_url = "http://mydev.lan/tools/{name}/latest.yaml"
url_template = "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"
```

使用：

- `eget add mydev:markview`
- `eget install mydev:markview`
- `eget install --add mydev:markview`

落盘 package 保留轻量引用：

```toml
[packages.markview]
repo = "pkg-template:mydev:markview"
```

- [x] 设计文档和实施计划。
- [ ] 实现配置、安装、更新和文档主链路。
````

- [ ] **Step 6: Commit docs**

Run:

```bash
git add docs/config.md docs/config.zh-CN.md docs/example.eget.toml README.md README.zh-CN.md docs/TODO.md docs/superpowers/specs/2026-06-18-pkg-templates-design.md
git commit -m "docs: document pkg_templates usage"
```

## Task 8: Full Verification

**Files:**
- All implementation files from previous tasks

- [ ] **Step 1: Run package-focused tests**

Run:

```bash
go test ./internal/source/pkgtemplate ./internal/config ./internal/install ./internal/app ./internal/cli -count=1
```

Expected: PASS.

- [ ] **Step 2: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS. This is required because pkg_templates touches install/update/list main package flows.

- [ ] **Step 3: Run GitNexus change detection before final commit or handoff**

Run:

```bash
npx gitnexus detect-changes --repo eget
```

Expected: changed symbols and affected flows match config/pkg-template/install/update/docs. If risk is HIGH or CRITICAL, stop and report before proceeding.

- [ ] **Step 4: Review staged diff**

Run:

```bash
git status --short
git diff --stat
git diff --cached --stat
```

Expected: only intended files are modified or staged. Do not stage unrelated existing modifications such as `AGENTS.md` or `CLAUDE.md` unless they are directly part of the implementation.

- [ ] **Step 5: Final implementation commit if needed**

If previous task commits already covered every change, no extra commit is needed. If there are final verification fixes, first run `git status --short` and identify the exact intended files, then stage only those files. Example for a final app resolution fix:

```bash
git add internal/app/install_resolve.go internal/app/install_config_test.go
git commit -m "fix: polish pkg_templates integration"
```

## Self-Review

- Spec coverage: covered naming, config model, canonical repo, short alias, lightweight add, direct install, install --add, update/list outdated/show, docs, and full test verification.
- Placeholder scan: no unspecified implementation step is left as an open placeholder. The path `docs/TODO.md` is referenced as a real project tracker file, not as an incomplete placeholder.
- Type consistency: canonical target is `pkg-template:<template>:<package>`; config section is `PkgTemplates` / `[pkg_templates.<name>]`; package path is `internal/source/pkgtemplate`; target kind is `TargetPkgTemplate`.
- Scope check: this is one cohesive feature across config and package install/update flows. It is not a server registry, cache registry, package index, or search feature.
