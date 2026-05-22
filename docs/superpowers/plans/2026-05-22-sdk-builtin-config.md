# 内置 SDK 配置模板实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标:** 新增 `eget sdk config add`，让用户可以把内置 Go、Node、JDK SDK 配置模板写入 `eget.toml`，无需手写 TOML。

**架构:** SDK 模板数据放在 `internal/sdk`，使用强类型 `cfgpkg.SDKSection` 表达。配置写入逻辑放在现有 `app.ConfigService`，CLI 通过 `sdk.config.add` 调用它。默认模板使用官方源，`--mirror` 切换为内置镜像源。

**技术栈:** Go、`gookit/gcli`、现有 `internal/config` 配置模型、现有 `internal/app.ConfigService`、现有 SDK 配置解析和 HTML index 解析。

---

## 复审结论

已复审设计文档：`docs/superpowers/specs/2026-05-22-sdk-builtin-config-design.md`。

结论：

- `eget sdk config add` 与当前 `sdk` 命令树匹配，不需要调整入口。
- 必须更新 `internal/cli/app.go` 的 `validateKnownFlags` 规则，否则 `sdk config add --all --force --mirror` 会在进入 gcli 前被未知 flag 校验拦截。
- 官方 JDK 依赖当前 HTML filename parser 解析 `openjdk-{version}_{os}-{arch}_bin.{ext}`，需要显式补回归测试。
- 设计文档无需修改，可以进入实现计划。

## 文件边界

- 新建 `internal/sdk/builtin_config.go`：内置 SDK 模板定义和查找函数。
- 新建 `internal/sdk/builtin_config_test.go`：模板查找、别名、官方/镜像 URL、JDK 关键字段测试。
- 修改 `internal/sdk/html_index_test.go`：补官方 JDK HTML 链接解析回归测试。
- 新建 `internal/app/sdk_config.go`：`ConfigService.AddSDKConfig` 和结果类型。
- 新建 `internal/app/sdk_config_test.go`：配置写入行为测试。
- 修改 `internal/cli/sdk_cmd.go`：新增 `sdk config add` 命令树和 options。
- 修改 `internal/cli/app.go`：新增 `sdk config add` 已知 flags。
- 修改 `internal/cli/handlers.go`：新增 `sdk.config.add` handler 分发和输出。
- 修改 `internal/cli/app_test.go`：CLI 参数解析测试。
- 修改 `internal/cli/service_test.go`：handler 输出测试。
- 修改 `docs/config.md`、`docs/config.zh-CN.md`、`docs/sdk-usage.md`、`README.md`、`README.zh-CN.md`：补充用户文档。

---

### Task 1: 内置 SDK 模板注册表

**Files:**
- Create: `internal/sdk/builtin_config.go`
- Create: `internal/sdk/builtin_config_test.go`
- Modify: `internal/sdk/html_index_test.go`

- [x] **Step 1: 编写失败测试：模板查找和别名解析**

创建 `internal/sdk/builtin_config_test.go`：

```go
package sdk

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestFindBuiltinConfigResolvesNamesAndAliases(t *testing.T) {
	goOfficial, ok := FindBuiltinConfig("golang", BuiltinConfigOfficial)
	assert.True(t, ok)
	assert.Eq(t, "go", goOfficial.Name)
	assert.Eq(t, BuiltinConfigOfficial, goOfficial.Source)
	assert.Eq(t, "https://go.dev/dl/", *goOfficial.Section.IndexURL)

	jdkMirror, ok := FindBuiltinConfig("java", BuiltinConfigMirror)
	assert.True(t, ok)
	assert.Eq(t, "jdk", jdkMirror.Name)
	assert.Eq(t, BuiltinConfigMirror, jdkMirror.Source)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/", *jdkMirror.Section.IndexURL)

	_, ok = FindBuiltinConfig("ruby", BuiltinConfigOfficial)
	assert.False(t, ok)
}

func TestBuiltinConfigNames(t *testing.T) {
	assert.Eq(t, []string{"go", "node", "jdk"}, BuiltinConfigNames())
}

func TestBuiltinOfficialAndMirrorTemplatesUseExpectedURLs(t *testing.T) {
	goOfficial, ok := FindBuiltinConfig("go", BuiltinConfigOfficial)
	assert.True(t, ok)
	assert.Eq(t, "https://go.dev/dl/go{version}.{os}-{arch}.{ext}", *goOfficial.Section.URLTemplate)

	goMirror, ok := FindBuiltinConfig("go", BuiltinConfigMirror)
	assert.True(t, ok)
	assert.Eq(t, "https://mirrors.aliyun.com/golang/go{version}.{os}-{arch}.{ext}", *goMirror.Section.URLTemplate)

	jdkOfficial, ok := FindBuiltinConfig("jdk", BuiltinConfigOfficial)
	assert.True(t, ok)
	assert.Nil(t, jdkOfficial.Section.URLTemplate)
	assert.Eq(t, "https://jdk.java.net/archive/", *jdkOfficial.Section.IndexURL)
	assert.Eq(t, "openjdk-{version}_{os}-{arch}_bin.{ext}", *jdkOfficial.Section.FilenamePattern)

	jdkMirror, ok := FindBuiltinConfig("jdk", BuiltinConfigMirror)
	assert.True(t, ok)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/{version}/openjdk-{version}_{os}-{arch}_bin.{ext}", *jdkMirror.Section.URLTemplate)
}
```

- [x] **Step 2: 编写失败测试：官方 JDK HTML index 解析**

在 `internal/sdk/html_index_test.go` 追加：

```go
func TestParseHTMLIndexForOfficialJDKArchiveLinks(t *testing.T) {
	body := strings.NewReader(`
<a href="https://download.java.net/java/GA/jdk21.0.2/openjdk-21.0.2_linux-x64_bin.tar.gz">jdk</a>
<a href="https://download.java.net/java/GA/jdk21.0.2/openjdk-21.0.2_windows-x64_bin.zip">jdk</a>
`)

	index, err := ParseHTMLIndex(body, HTMLParseOptions{
		SDK:             "jdk",
		SourceURL:       "https://jdk.java.net/archive/",
		FilenamePattern: "openjdk-{version}_{os}-{arch}_bin.{ext}",
	})
	if err != nil {
		t.Fatalf("parse jdk html index: %v", err)
	}

	assert.Eq(t, 1, len(index.Items))
	assert.Eq(t, "21.0.2", index.Items[0].Version)
	linuxFile := indexFileByOS(t, index.Items[0].Files, "linux")
	assert.Eq(t, "x64", linuxFile.Arch)
	assert.Eq(t, "tar.gz", linuxFile.Ext)
	assert.Eq(t, "https://download.java.net/java/GA/jdk21.0.2/openjdk-21.0.2_linux-x64_bin.tar.gz", linuxFile.URL)
}
```

- [x] **Step 3: 运行测试确认失败**

Run:

```bash
go test ./internal/sdk -run 'TestFindBuiltinConfig|TestBuiltinConfig|TestParseHTMLIndexForOfficialJDKArchiveLinks' -count=1
```

Expected:

- 编译失败，提示 `BuiltinConfigOfficial`、`BuiltinConfigMirror`、`FindBuiltinConfig`、`BuiltinConfigNames` 未定义。
- 如果执行到 HTML parser 测试，该测试应基于现有 parser 通过。

- [x] **Step 4: 实现内置模板注册表**

创建 `internal/sdk/builtin_config.go`：

```go
package sdk

import cfgpkg "github.com/inherelab/eget/internal/config"

type BuiltinConfigSource string

const (
	BuiltinConfigOfficial BuiltinConfigSource = "official"
	BuiltinConfigMirror   BuiltinConfigSource = "mirror"
)

type BuiltinConfig struct {
	Name    string
	Aliases []string
	Source  BuiltinConfigSource
	Section cfgpkg.SDKSection
}

func BuiltinConfigs(source BuiltinConfigSource) []BuiltinConfig {
	switch source {
	case BuiltinConfigMirror:
		return cloneBuiltinConfigs(builtinMirrorConfigs())
	default:
		return cloneBuiltinConfigs(builtinOfficialConfigs())
	}
}

func FindBuiltinConfig(name string, source BuiltinConfigSource) (BuiltinConfig, bool) {
	for _, item := range BuiltinConfigs(source) {
		if item.Name == name {
			return item, true
		}
		for _, alias := range item.Aliases {
			if alias == name {
				return item, true
			}
		}
	}
	return BuiltinConfig{}, false
}

func BuiltinConfigNames() []string {
	return []string{"go", "node", "jdk"}
}
```

在同文件继续加入官方模板：

```go
func builtinOfficialConfigs() []BuiltinConfig {
	return []BuiltinConfig{
		{
			Name:    "go",
			Aliases: []string{"golang"},
			Source:  BuiltinConfigOfficial,
			Section: cfgpkg.SDKSection{
				Aliases:         []string{"golang"},
				Target:          sdkStringPtr("gosdk/go{version}"),
				URLTemplate:     sdkStringPtr("https://go.dev/dl/go{version}.{os}-{arch}.{ext}"),
				IndexURL:        sdkStringPtr("https://go.dev/dl/"),
				IndexFormat:     sdkStringPtr("html"),
				FilenamePattern: sdkStringPtr("go{version}.{os}-{arch}.{ext}"),
				StripComponents: sdkIntPtr(1),
				ExtMap:          map[string]string{"windows": "zip", "linux": "tar.gz", "darwin": "tar.gz"},
			},
		},
		{
			Name:    "node",
			Aliases: []string{"nodejs"},
			Source:  BuiltinConfigOfficial,
			Section: cfgpkg.SDKSection{
				Aliases:         []string{"nodejs"},
				Target:          sdkStringPtr("nodejs/node{version}"),
				URLTemplate:     sdkStringPtr("https://nodejs.org/dist/v{version}/node-v{version}-{os}-{arch}.{ext}"),
				IndexURL:        sdkStringPtr("https://nodejs.org/dist/"),
				IndexFormat:     sdkStringPtr("html"),
				FilenamePattern: sdkStringPtr("node-v{version}-{os}-{arch}.{ext}"),
				StripComponents: sdkIntPtr(1),
				OSMap:           map[string]string{"windows": "win", "linux": "linux", "darwin": "darwin"},
				ArchMap:         map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"},
				ExtMap:          map[string]string{"windows": "zip", "linux": "tar.xz", "darwin": "tar.gz"},
			},
		},
		{
			Name:    "jdk",
			Aliases: []string{"java"},
			Source:  BuiltinConfigOfficial,
			Section: cfgpkg.SDKSection{
				Aliases:         []string{"java"},
				Target:          sdkStringPtr("jdk/openjdk-{version}"),
				IndexURL:        sdkStringPtr("https://jdk.java.net/archive/"),
				IndexFormat:     sdkStringPtr("html"),
				FilenamePattern: sdkStringPtr("openjdk-{version}_{os}-{arch}_bin.{ext}"),
				StripComponents: sdkIntPtr(1),
				ArchMap:         map[string]string{"amd64": "x64", "arm64": "aarch64"},
				OSMap:           map[string]string{"darwin": "macos"},
				ExtMap:          map[string]string{"windows": "zip", "linux": "tar.gz", "darwin": "tar.gz"},
			},
		},
	}
}
```

在同文件继续加入镜像模板和 clone helper。注意：`internal/sdk/config.go` 已有同包 `cloneStringMap`，这里直接复用，不要重复定义。

```go
func builtinMirrorConfigs() []BuiltinConfig {
	items := builtinOfficialConfigs()
	items[0].Source = BuiltinConfigMirror
	items[0].Section.URLTemplate = sdkStringPtr("https://mirrors.aliyun.com/golang/go{version}.{os}-{arch}.{ext}")
	items[0].Section.IndexURL = sdkStringPtr("https://mirrors.aliyun.com/golang/")

	items[1].Source = BuiltinConfigMirror
	items[1].Section.URLTemplate = sdkStringPtr("https://mirrors.aliyun.com/nodejs-release/v{version}/node-v{version}-{os}-{arch}.{ext}")
	items[1].Section.IndexURL = sdkStringPtr("https://mirrors.aliyun.com/nodejs-release/")
	items[1].Section.IndexPathPrefix = sdkStringPtr("/nodejs-release/")

	items[2].Source = BuiltinConfigMirror
	items[2].Section.URLTemplate = sdkStringPtr("https://mirrors.huaweicloud.com/openjdk/{version}/openjdk-{version}_{os}-{arch}_bin.{ext}")
	items[2].Section.IndexURL = sdkStringPtr("https://mirrors.huaweicloud.com/openjdk/")

	return items
}

func cloneBuiltinConfigs(items []BuiltinConfig) []BuiltinConfig {
	cloned := make([]BuiltinConfig, len(items))
	for i, item := range items {
		cloned[i] = BuiltinConfig{
			Name:    item.Name,
			Aliases: append([]string(nil), item.Aliases...),
			Source:  item.Source,
			Section: cloneSDKSection(item.Section),
		}
	}
	return cloned
}

func cloneSDKSection(section cfgpkg.SDKSection) cfgpkg.SDKSection {
	return cfgpkg.SDKSection{
		Aliases:         append([]string(nil), section.Aliases...),
		Target:          cloneBuiltinStringPtr(section.Target),
		URLTemplate:     cloneBuiltinStringPtr(section.URLTemplate),
		IndexURL:        cloneBuiltinStringPtr(section.IndexURL),
		IndexFormat:     cloneBuiltinStringPtr(section.IndexFormat),
		IndexParser:     cloneBuiltinStringPtr(section.IndexParser),
		IndexPathPrefix: cloneBuiltinStringPtr(section.IndexPathPrefix),
		FilenamePattern: cloneBuiltinStringPtr(section.FilenamePattern),
		StripComponents: cloneBuiltinIntPtr(section.StripComponents),
		OSMap:           cloneStringMap(section.OSMap),
		ArchMap:         cloneStringMap(section.ArchMap),
		ExtMap:          cloneStringMap(section.ExtMap),
	}
}

func cloneBuiltinStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBuiltinIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func sdkStringPtr(value string) *string {
	return &value
}

func sdkIntPtr(value int) *int {
	return &value
}
```

- [x] **Step 5: 运行 SDK 测试确认通过**

Run:

```bash
go test ./internal/sdk -run 'TestFindBuiltinConfig|TestBuiltinConfig|TestParseHTMLIndexForOfficialJDKArchiveLinks' -count=1
```

Expected: PASS.

- [x] **Step 6: 提交 Task 1**

Run:

```bash
git add internal/sdk/builtin_config.go internal/sdk/builtin_config_test.go internal/sdk/html_index_test.go
git commit -m "feat: add builtin sdk config templates"
```

---

### Task 2: 配置写入服务

**Files:**
- Create: `internal/app/sdk_config.go`
- Create: `internal/app/sdk_config_test.go`

- [x] **Step 1: 编写失败测试：ConfigService 写入 SDK 配置**

创建 `internal/app/sdk_config_test.go`：

```go
package app

import (
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
)

func TestAddSDKConfigAddsOfficialTemplate(t *testing.T) {
	cfg := cfgpkg.NewFile()
	saved := false
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error {
			saved = true
			return nil
		},
	}

	result, err := svc.AddSDKConfig(SDKConfigAddOptions{Name: "jdk"})
	assert.NoErr(t, err)
	assert.True(t, saved)
	assert.Eq(t, 1, len(result.Items))
	assert.Eq(t, "jdk", result.Items[0].Name)
	assert.Eq(t, SDKConfigActionAdded, result.Items[0].Action)
	assert.Eq(t, "https://jdk.java.net/archive/", *cfg.SDK["jdk"].IndexURL)
	assert.Nil(t, cfg.SDK["jdk"].URLTemplate)
}

func TestAddSDKConfigAddsMirrorTemplateByAlias(t *testing.T) {
	cfg := cfgpkg.NewFile()
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error { return nil },
	}

	result, err := svc.AddSDKConfig(SDKConfigAddOptions{Name: "java", Mirror: true})
	assert.NoErr(t, err)
	assert.Eq(t, "jdk", result.Items[0].Name)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/", *cfg.SDK["jdk"].IndexURL)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/{version}/openjdk-{version}_{os}-{arch}_bin.{ext}", *cfg.SDK["jdk"].URLTemplate)
}

func TestAddSDKConfigRejectsExistingWithoutForce(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.SDK["jdk"] = cfgpkg.SDKSection{Target: appStringPtr("custom")}
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error {
			t.Fatal("save should not be called")
			return nil
		},
	}

	_, err := svc.AddSDKConfig(SDKConfigAddOptions{Name: "jdk"})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.Eq(t, "custom", *cfg.SDK["jdk"].Target)
}

func TestAddSDKConfigForceUpdatesExisting(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.SDK["jdk"] = cfgpkg.SDKSection{Target: appStringPtr("custom")}
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error { return nil },
	}

	result, err := svc.AddSDKConfig(SDKConfigAddOptions{Name: "jdk", Force: true, Mirror: true})
	assert.NoErr(t, err)
	assert.Eq(t, SDKConfigActionUpdated, result.Items[0].Action)
	assert.Eq(t, "jdk/openjdk-{version}", *cfg.SDK["jdk"].Target)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/", *cfg.SDK["jdk"].IndexURL)
}

func TestAddSDKConfigAllSkipsExistingAndAddsMissing(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.SDK["go"] = cfgpkg.SDKSection{Target: appStringPtr("custom-go")}
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error { return nil },
	}

	result, err := svc.AddSDKConfig(SDKConfigAddOptions{All: true, Mirror: true})
	assert.NoErr(t, err)
	assert.Eq(t, 3, len(result.Items))
	assert.Eq(t, SDKConfigActionSkipped, result.Items[0].Action)
	assert.Eq(t, "custom-go", *cfg.SDK["go"].Target)
	assert.Eq(t, "https://mirrors.aliyun.com/nodejs-release/", *cfg.SDK["node"].IndexURL)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/", *cfg.SDK["jdk"].IndexURL)
}

func TestAddSDKConfigValidatesInput(t *testing.T) {
	cfg := cfgpkg.NewFile()
	svc := ConfigService{Load: func() (*cfgpkg.File, error) { return cfg, nil }}

	_, err := svc.AddSDKConfig(SDKConfigAddOptions{})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "requires exactly one")

	_, err = svc.AddSDKConfig(SDKConfigAddOptions{Name: "jdk", All: true})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "requires exactly one")

	_, err = svc.AddSDKConfig(SDKConfigAddOptions{Name: "ruby"})
	assert.Err(t, err)
	assert.True(t, strings.Contains(err.Error(), "available: go, node, jdk"))
}

func appStringPtr(value string) *string {
	return &value
}
```

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/app -run TestAddSDKConfig -count=1
```

Expected: 编译失败，提示 `AddSDKConfig`、`SDKConfigAddOptions`、`SDKConfigActionAdded` 等未定义。

- [x] **Step 3: 实现 ConfigService.AddSDKConfig**

创建 `internal/app/sdk_config.go`：

```go
package app

import (
	"fmt"
	"strings"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/sdk"
)

const (
	SDKConfigActionAdded   = "added"
	SDKConfigActionUpdated = "updated"
	SDKConfigActionSkipped = "skipped"
)

type SDKConfigAddOptions struct {
	Name   string
	All    bool
	Force  bool
	Mirror bool
}

type SDKConfigAddResult struct {
	Items []SDKConfigAddItem
}

type SDKConfigAddItem struct {
	Name   string
	Action string
	Reason string
	Source string
}

func (s ConfigService) AddSDKConfig(opts SDKConfigAddOptions) (SDKConfigAddResult, error) {
	if (strings.TrimSpace(opts.Name) == "") == !opts.All {
		return SDKConfigAddResult{}, fmt.Errorf("sdk config add requires exactly one of <name> or --all")
	}
	source := sdk.BuiltinConfigOfficial
	if opts.Mirror {
		source = sdk.BuiltinConfigMirror
	}

	cfg, err := s.load()
	if err != nil {
		return SDKConfigAddResult{}, err
	}
	if cfg.SDK == nil {
		cfg.SDK = make(map[string]cfgpkg.SDKSection)
	}

	var builtins []sdk.BuiltinConfig
	if opts.All {
		builtins = sdk.BuiltinConfigs(source)
	} else {
		builtin, ok := sdk.FindBuiltinConfig(opts.Name, source)
		if !ok {
			return SDKConfigAddResult{}, fmt.Errorf("unknown built-in SDK config %q; available: %s", opts.Name, strings.Join(sdk.BuiltinConfigNames(), ", "))
		}
		builtins = []sdk.BuiltinConfig{builtin}
	}

	result := SDKConfigAddResult{Items: make([]SDKConfigAddItem, 0, len(builtins))}
	changed := false
	for _, builtin := range builtins {
		if _, exists := cfg.SDK[builtin.Name]; exists && !opts.Force {
			if !opts.All {
				return SDKConfigAddResult{}, fmt.Errorf("sdk config %s already exists, use --force to overwrite", builtin.Name)
			}
			result.Items = append(result.Items, SDKConfigAddItem{Name: builtin.Name, Action: SDKConfigActionSkipped, Reason: "already exists", Source: string(source)})
			continue
		}
		action := SDKConfigActionAdded
		if _, exists := cfg.SDK[builtin.Name]; exists {
			action = SDKConfigActionUpdated
		}
		cfg.SDK[builtin.Name] = builtin.Section
		result.Items = append(result.Items, SDKConfigAddItem{Name: builtin.Name, Action: action, Source: string(source)})
		changed = true
	}
	if changed {
		if err := s.save(cfg); err != nil {
			return SDKConfigAddResult{}, err
		}
	}
	return result, nil
}
```

- [x] **Step 4: 运行 app 测试确认通过**

Run:

```bash
go test ./internal/app -run TestAddSDKConfig -count=1
```

Expected: PASS.

- [x] **Step 5: 提交 Task 2**

Run:

```bash
git add internal/app/sdk_config.go internal/app/sdk_config_test.go
git commit -m "feat: add sdk config writer service"
```

---

### Task 3: CLI 命令解析

**Files:**
- Modify: `internal/cli/sdk_cmd.go`
- Modify: `internal/cli/app.go`
- Modify: `internal/cli/app_test.go`

- [x] **Step 1: 编写失败测试：CLI 参数解析**

在 `internal/cli/app_test.go` 追加：

```go
func TestMain_SDKConfigAddRoutesAndBindsOptions(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"sdk", "config", "add", "--mirror", "--force", "java"})
	assert.NoErr(t, err)
	assert.Eq(t, 1, len(calls))
	assert.Eq(t, "sdk.config.add", calls[0].name)
	opts, ok := calls[0].options.(*SDKConfigOptions)
	assert.True(t, ok)
	assert.Eq(t, "add", opts.Action)
	assert.Eq(t, "java", opts.Name)
	assert.True(t, opts.Mirror)
	assert.True(t, opts.Force)
	assert.False(t, opts.All)
}

func TestMain_SDKConfigAddAllRoutesAndBindsOptions(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"sdk", "config", "add", "--all", "--mirror"})
	assert.NoErr(t, err)
	assert.Eq(t, 1, len(calls))
	opts, ok := calls[0].options.(*SDKConfigOptions)
	assert.True(t, ok)
	assert.True(t, opts.All)
	assert.True(t, opts.Mirror)
	assert.Eq(t, "", opts.Name)
}

func TestMain_SDKConfigAddRejectsNameAndAll(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(func(string, any) error {
		t.Fatal("handler should not run")
		return nil
	}, &stdout, &stderr).RunWithArgs([]string{"sdk", "config", "add", "--all", "jdk"})

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "requires exactly one")
}
```

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/cli -run 'TestMain_SDKConfigAdd' -count=1
```

Expected: 编译失败或运行失败，因为 `SDKConfigOptions` 和 `sdk config add` 命令尚未定义。

- [x] **Step 3: 增加 SDK config 命令树**

修改 `internal/cli/sdk_cmd.go`。

在 SDK option structs 附近新增：

```go
type SDKConfigOptions struct {
	Action string
	Name   string
	All    bool
	Force  bool
	Mirror bool
}
```

在 `newSDKCmd` 中创建并 reset：

```go
configOpts := &SDKConfigOptions{}
```

在 `cmd.Subs` 加入：

```go
newSDKConfigCmd(configOpts, handler),
```

在 resetter 加入：

```go
*configOpts = SDKConfigOptions{}
```

新增 builder：

```go
func newSDKConfigCmd(opts *SDKConfigOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("config", "Manage SDK config templates")
	cmd.Aliases = []string{"cfg"}
	cmd.Subs = []*gcli.Command{
		newSDKConfigAddCmd(opts, handler),
	}
	return cmd
}

func newSDKConfigAddCmd(opts *SDKConfigOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("add", "Add built-in SDK config template")
	cmd.Config = func(c *gcli.Command) {
		c.BoolOpt(&opts.All, "all", "a", false, "Add all built-in SDK configs")
		c.BoolOpt(&opts.Force, "force", "f", false, "Overwrite existing SDK config")
		c.BoolOpt(&opts.Mirror, "mirror", "m", false, "Use built-in mirror source instead of official source")
		c.AddArg("name", "Built-in SDK name or alias", false)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Action = "add"
		opts.Name = c.Arg("name").String()
		if err := validateNoFlagArgs(append([]string{opts.Name}, args...)); err != nil {
			return err
		}
		if (opts.Name == "") == !opts.All {
			return fmt.Errorf("sdk config add requires exactly one of <name> or --all")
		}
		snapshot := *opts
		return handler("sdk.config.add", &snapshot)
	}
	return cmd
}
```

在 `newSDKCmd` help 示例里补：

```text
  eget sdk config add jdk --mirror
  eget sdk config add --all
```

- [x] **Step 4: 更新已知 flags 校验**

修改 `internal/cli/app.go` 的 `commandFlagSpecs`。在 `"sdk"` 的 `subs` 里加入：

```go
"config": {
	subs: map[string]flagSpec{
		"add": {
			bools: setOf("all", "a", "force", "f", "mirror", "m"),
		},
	},
},
"cfg": {
	subs: map[string]flagSpec{
		"add": {
			bools: setOf("all", "a", "force", "f", "mirror", "m"),
		},
	},
},
```

- [x] **Step 5: 运行 CLI 解析测试确认通过**

Run:

```bash
go test ./internal/cli -run 'TestMain_SDKConfigAdd' -count=1
```

Expected: PASS.

- [x] **Step 6: 提交 Task 3**

Run:

```bash
git add internal/cli/sdk_cmd.go internal/cli/app.go internal/cli/app_test.go
git commit -m "feat: add sdk config cli command"
```

---

### Task 4: CLI handler 和输出

**Files:**
- Modify: `internal/cli/handlers.go`
- Modify: `internal/cli/service_test.go`

- [x] **Step 1: 编写失败测试：handler 输出**

在 `internal/cli/service_test.go` 追加：

```go
func TestHandleSDKConfigAddPrintsResult(t *testing.T) {
	cfg := cfgpkg.NewFile()
	svc := &cliService{
		cfgService: app.ConfigService{
			Load: func() (*cfgpkg.File, error) { return cfg, nil },
			Save: func(path string, file *cfgpkg.File) error { return nil },
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("sdk.config.add", &SDKConfigOptions{Action: "add", Name: "jdk", Mirror: true})
	assert.NoErr(t, err)

	got := ccolor.ClearCode(out.String())
	assert.Contains(t, got, "Added SDK config: jdk (mirror)")
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/", *cfg.SDK["jdk"].IndexURL)
}

func TestHandleSDKConfigAddAllPrintsSkippedAndAdded(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.SDK["go"] = cfgpkg.SDKSection{Target: cliStringPtr("custom-go")}
	svc := &cliService{
		cfgService: app.ConfigService{
			Load: func() (*cfgpkg.File, error) { return cfg, nil },
			Save: func(path string, file *cfgpkg.File) error { return nil },
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("sdk.config.add", &SDKConfigOptions{Action: "add", All: true, Mirror: true})
	assert.NoErr(t, err)

	got := ccolor.ClearCode(out.String())
	assert.Contains(t, got, "Skipped SDK config: go already exists")
	assert.Contains(t, got, "Added SDK config: node (mirror)")
	assert.Contains(t, got, "Added SDK config: jdk (mirror)")
}
```

如果 `internal/cli/service_test.go` 没有字符串指针 helper，在测试 helpers 附近增加：

```go
func cliStringPtr(value string) *string {
	return &value
}
```

- [x] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/cli -run 'TestHandleSDKConfigAdd' -count=1
```

Expected: 失败，原因是 `sdk.config.add` 还没有在 `cliService.handle` 中路由。

- [x] **Step 3: 增加 handler 路由和输出**

修改 `internal/cli/handlers.go`。

在 `handle` switch 中加入：

```go
case "sdk.config.add":
	opts := options.(*SDKConfigOptions)
	return s.handleSDKConfig(opts)
```

在 `handleSDKIndex` 附近新增：

```go
func (s *cliService) handleSDKConfig(opts *SDKConfigOptions) error {
	if opts == nil || opts.Action != "add" {
		return fmt.Errorf("sdk config action is required")
	}
	result, err := s.cfgService.AddSDKConfig(app.SDKConfigAddOptions{
		Name:   opts.Name,
		All:    opts.All,
		Force:  opts.Force,
		Mirror: opts.Mirror,
	})
	if err != nil {
		return err
	}
	for _, item := range result.Items {
		source := item.Source
		if source == "" {
			source = "official"
		}
		switch item.Action {
		case app.SDKConfigActionAdded:
			ccolor.Successf("✓ Added SDK config: %s (%s)\n", item.Name, source)
		case app.SDKConfigActionUpdated:
			ccolor.Successf("✓ Updated SDK config: %s (%s)\n", item.Name, source)
		case app.SDKConfigActionSkipped:
			ccolor.Infof("- Skipped SDK config: %s %s\n", item.Name, item.Reason)
		}
	}
	return nil
}
```

- [x] **Step 4: 运行 handler 测试确认通过**

Run:

```bash
go test ./internal/cli -run 'TestHandleSDKConfigAdd' -count=1
```

Expected: PASS.

- [x] **Step 5: 提交 Task 4**

Run:

```bash
git add internal/cli/handlers.go internal/cli/service_test.go
git commit -m "feat: wire sdk config add handler"
```

---

### Task 5: 文档和端到端验证

**Files:**
- Modify: `docs/config.md`
- Modify: `docs/config.zh-CN.md`
- Modify: `docs/sdk-usage.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

- [ ] **Step 1: 更新文档**

在 `docs/sdk-usage.md` 增加一节：

````markdown
## 内置 SDK 配置模板

可以通过 `eget sdk config add` 写入内置 SDK 配置模板，避免手写 `[sdk.<name>]`。

默认使用官方源：

```bash
eget sdk config add go
eget sdk config add node
eget sdk config add jdk
eget sdk config add --all
```

使用内置镜像源：

```bash
eget sdk config add jdk --mirror
eget sdk config add --all --mirror
```

已存在的 SDK 配置默认不会覆盖。需要覆盖时使用：

```bash
eget sdk config add jdk --force
```
````

在 `docs/config.md`、`docs/config.zh-CN.md`、`README.md`、`README.zh-CN.md` 的 SDK 配置示例附近补充简短说明。

英文文档使用：

````markdown
You can also write built-in SDK templates with:

```bash
eget sdk config add --all
eget sdk config add --all --mirror
```
````

中文文档使用：

````markdown
也可以通过内置模板快速写入 SDK 配置：

```bash
eget sdk config add --all
eget sdk config add --all --mirror
```
````

- [ ] **Step 2: 运行完整测试**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: 使用临时配置做命令冒烟测试**

Run:

```powershell
$env:EGET_CONFIG = Join-Path (New-Item -ItemType Directory -Force .tmp-sdk-config-test).FullName "eget.toml"
go run ./cmd/eget config init
go run ./cmd/eget sdk config add jdk --mirror
go run ./cmd/eget config get sdk.jdk.index_url
```

Expected final output contains:

```text
https://mirrors.huaweicloud.com/openjdk/
```

Run:

```powershell
go run ./cmd/eget sdk index build jdk
```

Expected output contains:

```text
✓ Refreshed SDK index: jdk
```

- [ ] **Step 4: 清理临时目录**

Run:

```powershell
$target = Resolve-Path .tmp-sdk-config-test
if (-not ($target.Path.StartsWith((Resolve-Path .).Path))) { throw "unexpected temp path: $target" }
Remove-Item -LiteralPath $target.Path -Recurse -Force
```

- [ ] **Step 5: 提交 Task 5**

Run:

```bash
git add docs/config.md docs/config.zh-CN.md docs/sdk-usage.md README.md README.zh-CN.md
git commit -m "docs: document builtin sdk config command"
```

---

## 最终验证

- [ ] Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] Run:

```bash
git status --short
```

Expected: 只剩实现前就存在的无关未提交文件；除非用户明确要求，不要把它们纳入提交。

## 执行选项

计划已保存到 `docs/superpowers/plans/2026-05-22-sdk-builtin-config.md`。两个执行选项：

1. Subagent-Driven（推荐）：每个任务派发一个新 subagent，任务之间由主会话审查，迭代更快。
2. Inline Execution：在当前会话按计划执行，阶段性检查。

请选择执行方式。
