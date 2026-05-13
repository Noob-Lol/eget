# System 7z Extractor Implementation Plan

> **给执行 Agent：** 实施本计划时必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，逐任务执行并维护下面的 checkbox 状态。

**目标：** 新增 `global.sys7z_path` 配置项，并在解压 `.7z`、`.rar`、可被 7z 打开的 installer/self-extracting archive 等格式时优先使用系统 7z，可用时从 `sys7z_path` 或 `PATH` 查找，不可用时回退现有 Go 解压实现。

**架构：** 保持现有安装主链路不变：解析目标 -> 选择 asset -> 下载 -> 校验 -> 选择 extractor -> 选择 archive 内文件 -> 写入目标路径。新增系统 7z 只作为 `Extractor` 的一个实现接入 `Service.SelectExtractor()`，继续复用现有 `Chooser`、多候选选择、`--file`、`--all` 和安全路径校验逻辑。

**技术栈：** Go 1.24.2、现有 `internal/install` extractor 接口、`os/exec`、现有 config merge 模型、现有 Go archive 解压实现、`go test ./...`。

---

## 设计约束

- 配置项命名为 `global.sys7z_path`，用于指定 7z 可执行文件路径。
- 第一版不新增 CLI flag。
- 查找优先级：
  1. `install.Options.Sys7zPath`，来自配置合并后的 `global.sys7z_path`。
  2. `PATH` 中的 `7z`、`7zz`、`7za`。
  3. 现有 Go 解压实现。
- `sys7z_path` 配置路径不可用时，不直接失败；继续尝试 `PATH`，最后回退 Go 解压。verbose 模式下记录原因。
- 只让系统 7z 接管 7z 更有优势的格式：
  - `.7z`
  - `.rar`
  - `.exe` 且 `--all`，用于 installer/self-extracting archive
  - `.msi`
  - `.cab`
  - `.iso`
- `.zip` 首版继续走现有 Go `archive/zip`，除非后续明确需要系统 7z 接管。
- `.tar`、`.tar.gz`、`.tgz`、`.tar.xz`、`.txz`、`.tar.bz2`、`.tbz`、`.tar.zst` 继续走现有 Go 流程。
- `.gz`、`.bz2`、`.xz`、`.zst` 单文件解压继续走现有 Go 流程。
- 不实现 `tar.*` 的 7z nested archive 二段解包。原因是 7z 处理 `.tar.gz` 等格式时常先暴露中间 `.tar`，需要额外临时文件和二次 list/extract，收益低且更容易改变现有默认二进制选择行为。
- 系统 7z list 和 extract 都必须重新经过现有安全路径检查，不能信任外部命令输出。
- 找不到系统 7z 时回退 Go extractor；找到系统 7z 且已经进入外部 7z 解压后，如果命令失败，应返回错误，不静默吞掉真实损坏包、密码包或权限错误。
- 这个改动涉及 MVP 安装/解压主链路，完成后必须运行 `go test ./...`。

## 当前解压链路

- `internal/install/runner.go` 的 `InstallRunner.Run()` 负责安装主流程。
- `Run()` 下载 asset 后调用 `SelectExtractorAs[Extractor](r.Service, url, tool, &opts)`。
- `internal/install/service.go` 的 `Service.SelectExtractor()` 根据 `DownloadOnly`、`ExtractFile`、`All` 创建 chooser，并调用 `ExtractorFactory`。
- `internal/install/defaults.go` 的 `NewExtractor()` 根据文件扩展名选择 Go extractor：
  - `tar.*` 使用压缩流 reader + `archive/tar`。
  - `.zip` 使用 `archive/zip`。
  - `.7z` 和 `.exe --all` 使用 `github.com/bodgit/sevenzip`。
  - `.gz` / `.bz2` / `.xz` / `.zst` 使用单文件解压。
  - 其他格式按单文件直接写出。

## 文件职责

- `internal/config/model.go`：新增 `Section.Sys7zPath` 和 `Merged.Sys7zPath`。
- `internal/config/merge.go`：合并 `sys7z_path`。
- `internal/config/gookit.go`：支持 dump/write `sys7z_path`。
- `internal/config/*_test.go`：覆盖读取、写回、`config set/get`、merge。
- `internal/install/options.go`：新增 `Options.Sys7zPath`。
- `internal/app/install.go`：把配置合并后的 `sys7z_path` 展开后写入 `install.Options.Sys7zPath`。
- `internal/install/system7z.go`：新增系统 7z 查找、格式判定、外部 7z extractor。
- `internal/install/system7z_test.go`：覆盖查找和 extractor 行为。
- `internal/install/service.go`：在 `SelectExtractor()` 里按格式和可执行文件可用性选择系统 7z extractor。
- `internal/install/service_test.go`：覆盖选择逻辑和 fallback。
- `docs/DOCS.md` / `docs/example.eget.toml` / `README.md` / `README.zh-CN.md`：补充用户文档和示例。

---

## Task 1：新增配置模型和合并字段

**文件：**
- 修改：`internal/config/model.go`
- 修改：`internal/config/merge.go`
- 修改：`internal/config/gookit.go`
- 修改：`internal/config/loader_test.go`
- 修改：`internal/config/merge_test.go`
- 修改：`internal/config/gookit_test.go`

- [x] **Step 1：编写配置读取测试**

在 `internal/config/loader_test.go` 增加测试：

```go
func TestLoadFileReadsGlobalSys7zPath(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	writeTestFile(t, configPath, `
[global]
sys7z_path = "C:/Program Files/7-Zip/7z.exe"
`)

	cfg, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	assert.Eq(t, "C:/Program Files/7-Zip/7z.exe", *cfg.Global.Sys7zPath)
}
```

- [x] **Step 2：编写配置合并测试**

在 `internal/config/merge_test.go` 增加测试：

```go
func TestMergeInstallOptionsMergesSys7zPath(t *testing.T) {
	globalPath := "C:/global/7z.exe"
	repoPath := "C:/repo/7z.exe"
	pkgPath := "C:/pkg/7z.exe"

	merged := MergeInstallOptions(
		Section{Sys7zPath: &globalPath},
		Section{},
		Section{},
		CLIOverrides{},
	)
	assert.Eq(t, globalPath, merged.Sys7zPath)

	merged = MergeInstallOptions(
		Section{Sys7zPath: &globalPath},
		Section{Sys7zPath: &repoPath},
		Section{},
		CLIOverrides{},
	)
	assert.Eq(t, repoPath, merged.Sys7zPath)

	merged = MergeInstallOptions(
		Section{Sys7zPath: &globalPath},
		Section{Sys7zPath: &repoPath},
		Section{Sys7zPath: &pkgPath},
		CLIOverrides{},
	)
	assert.Eq(t, pkgPath, merged.Sys7zPath)
}
```

- [x] **Step 3：编写 dump/config set 测试**

在 `internal/config/gookit_test.go` 增加测试：

```go
func TestDumpConfigStringIncludesSys7zPath(t *testing.T) {
	path := "C:/Program Files/7-Zip/7z.exe"
	cfg := NewFile()
	cfg.Global.Sys7zPath = &path

	text, err := dumpConfigString(cfg)
	if err != nil {
		t.Fatalf("dump config string: %v", err)
	}

	assert.Contains(t, text, `sys7z_path = "C:/Program Files/7-Zip/7z.exe"`)
}

func TestSetByPathSupportsGlobalSys7zPath(t *testing.T) {
	cfg := NewFile()

	if err := SetByPath(cfg, "global.sys7z_path", "C:/Tools/7z.exe"); err != nil {
		t.Fatalf("set global.sys7z_path: %v", err)
	}

	value, ok := GetByPath(cfg, "global.sys7z_path")
	if !ok {
		t.Fatal("expected global.sys7z_path to be set")
	}
	assert.Eq(t, "C:/Tools/7z.exe", value)
}
```

- [x] **Step 4：运行配置测试确认失败**

执行：

```bash
go test ./internal/config -run 'Sys7zPath|MergeInstallOptionsMergesSys7zPath' -v
```

预期：失败，提示字段不存在或 dump 中没有 `sys7z_path`。

- [x] **Step 5：实现配置字段**

在 `internal/config/model.go` 的 `Section` 增加：

```go
Sys7zPath *string `toml:"sys7z_path" mapstructure:"sys7z_path"`
```

在 `Merged` 增加：

```go
Sys7zPath string
```

在 `internal/config/merge.go` 的 `MergeInstallOptions()` 中增加：

```go
merged.Sys7zPath = firstString(pkg.Sys7zPath, repo.Sys7zPath, global.Sys7zPath)
```

在 `internal/config/gookit.go` 的 `sectionToMap()` 中增加：

```go
if section.Sys7zPath != nil && *section.Sys7zPath != "" {
	data["sys7z_path"] = *section.Sys7zPath
}
```

- [x] **Step 6：运行配置测试确认通过**

执行：

```bash
go test ./internal/config -run 'Sys7zPath|MergeInstallOptionsMergesSys7zPath' -v
```

预期：通过。

- [x] **Step 7：提交配置模型改动**

执行：

```bash
git add internal/config/model.go internal/config/merge.go internal/config/gookit.go internal/config/*_test.go
git commit -m "feat(config): add system 7z path"
```

---

## Task 2：把配置传递到安装选项

**文件：**
- 修改：`internal/install/options.go`
- 修改：`internal/app/install.go`
- 修改：`internal/app/install_test.go`

- [x] **Step 1：编写 app 层传递测试**

在 `internal/app/install_test.go` 增加测试：

```go
func TestResolveInstallOptionsPassesGlobalSys7zPath(t *testing.T) {
	cfg := cfgpkg.NewFile()
	path := "~/bin/7z"
	cfg.Global.Sys7zPath = &path

	svc := Service{
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}

	opts, err := svc.resolveInstallOptions("owner/repo", install.Options{}, false)
	if err != nil {
		t.Fatalf("resolve install options: %v", err)
	}

	assert.Contains(t, opts.Sys7zPath, filepath.Join("bin", "7z"))
}
```

- [x] **Step 2：运行 app 测试确认失败**

执行：

```bash
go test ./internal/app -run TestResolveInstallOptionsPassesGlobalSys7zPath -v
```

预期：失败，提示 `Sys7zPath` 字段不存在或为空。

- [x] **Step 3：实现 install options 字段**

在 `internal/install/options.go` 的 `Options` 增加：

```go
Sys7zPath string
```

- [x] **Step 4：在 app 层展开并传递路径**

在 `internal/app/install.go` 的 `resolveInstallOptionsWithConfig()` 中，在 `guiTarget` 后增加：

```go
sys7zPath, err := expandPath(merged.Sys7zPath)
if err != nil {
	return install.Options{}, err
}
```

在返回的 `install.Options` 中增加：

```go
Sys7zPath: sys7zPath,
```

- [x] **Step 5：运行 app 测试确认通过**

执行：

```bash
go test ./internal/app -run TestResolveInstallOptionsPassesGlobalSys7zPath -v
```

预期：通过。

- [x] **Step 6：提交安装选项传递改动**

执行：

```bash
git add internal/install/options.go internal/app/install.go internal/app/install_test.go
git commit -m "feat(install): pass system 7z path to runner"
```

---

## Task 3：实现系统 7z 查找和格式判定

**文件：**
- 新增：`internal/install/system7z.go`
- 新增：`internal/install/system7z_test.go`

- [x] **Step 1：编写 7z 查找测试**

新增 `internal/install/system7z_test.go`：

```go
package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestResolveSystem7zPathUsesConfiguredPath(t *testing.T) {
	tmp := t.TempDir()
	exe := filepath.Join(tmp, "custom-7z.exe")
	if err := os.WriteFile(exe, []byte("fake"), 0o755); err != nil {
		t.Fatalf("write fake 7z: %v", err)
	}

	got := resolveSystem7zPath(exe)
	assert.Eq(t, exe, got)
}

func TestResolveSystem7zPathFallsBackWhenConfiguredPathMissing(t *testing.T) {
	t.Setenv("PATH", "")

	got := resolveSystem7zPath(filepath.Join(t.TempDir(), "missing-7z.exe"))
	assert.Eq(t, "", got)
}

func TestShouldUseSystem7zForPreferredFormats(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		all      bool
		want     bool
	}{
		{name: "7z", filename: "tool.7z", want: true},
		{name: "rar", filename: "tool.rar", want: true},
		{name: "msi", filename: "setup.msi", want: true},
		{name: "cab", filename: "driver.cab", want: true},
		{name: "iso", filename: "image.iso", want: true},
		{name: "exe all", filename: "setup.exe", all: true, want: true},
		{name: "exe single", filename: "setup.exe", want: false},
		{name: "zip stays go", filename: "tool.zip", want: false},
		{name: "tar gz stays go", filename: "tool.tar.gz", want: false},
		{name: "tgz stays go", filename: "tool.tgz", want: false},
		{name: "tar xz stays go", filename: "tool.tar.xz", want: false},
		{name: "txz stays go", filename: "tool.txz", want: false},
		{name: "tar bz2 stays go", filename: "tool.tar.bz2", want: false},
		{name: "tbz stays go", filename: "tool.tbz", want: false},
		{name: "tar zst stays go", filename: "tool.tar.zst", want: false},
		{name: "single gz stays go", filename: "tool.gz", want: false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Eq(t, tt.want, shouldUseSystem7z(tt.filename, tt.all))
		})
	}
}
```

- [x] **Step 2：运行测试确认失败**

执行：

```bash
go test ./internal/install -run 'System7z|ShouldUseSystem7z' -v
```

预期：失败，提示函数不存在。

- [x] **Step 3：实现查找和格式判定**

新增 `internal/install/system7z.go`：

```go
package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var system7zCandidates = []string{"7z", "7zz", "7za"}

func resolveSystem7zPath(configured string) string {
	if configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured
		}
	}
	for _, name := range system7zCandidates {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func shouldUseSystem7z(filename string, extractAll bool) bool {
	name := strings.ToLower(filepath.Base(filename))
	switch {
	case strings.HasSuffix(name, ".7z"),
		strings.HasSuffix(name, ".rar"),
		strings.HasSuffix(name, ".msi"),
		strings.HasSuffix(name, ".cab"),
		strings.HasSuffix(name, ".iso"):
		return true
	case strings.HasSuffix(name, ".exe") && extractAll:
		return true
	default:
		return false
	}
}
```

- [x] **Step 4：运行测试确认通过**

执行：

```bash
go test ./internal/install -run 'System7z|ShouldUseSystem7z' -v
```

预期：通过。

- [x] **Step 5：提交基础能力**

执行：

```bash
git add internal/install/system7z.go internal/install/system7z_test.go
git commit -m "feat(install): detect system 7z executable"
```

---

## Task 4：接入系统 7z extractor 选择逻辑

**文件：**
- 修改：`internal/install/service.go`
- 修改：`internal/install/service_test.go`
- 修改：`internal/install/system7z.go`

- [x] **Step 1：编写选择逻辑测试**

在 `internal/install/service_test.go` 增加测试：

```go
func TestSelectExtractorUsesSystem7zForSevenZipWhenAvailable(t *testing.T) {
	svc := NewService()
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return NewFileChooser(pattern)
	}
	svc.BinaryChooserFactory = func(tool string) any {
		return NewBinaryChooser(tool)
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return fakeExtractor{name: "go:" + filename}
	}
	svc.System7zPathResolver = func(configured string) string {
		assert.Eq(t, "C:/Tools/7z.exe", configured)
		return "C:/Tools/7z.exe"
	}
	svc.System7zExtractorFactory = func(filename, tool string, chooser Chooser, exe string) Extractor {
		return fakeExtractor{name: "system7z:" + filepath.Base(filename)}
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.7z", "tool", &Options{Sys7zPath: "C:/Tools/7z.exe"})
	if err != nil {
		t.Fatalf("SelectExtractor(system 7z): %v", err)
	}

	assert.Eq(t, "system7z:tool.7z", extractor.(fakeExtractor).name)
}

func TestSelectExtractorFallsBackToGoExtractorWithoutSystem7z(t *testing.T) {
	svc := NewService()
	svc.BinaryChooserFactory = func(tool string) any {
		return NewBinaryChooser(tool)
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return fakeExtractor{name: "go:" + filename}
	}
	svc.System7zPathResolver = func(configured string) string {
		return ""
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.7z", "tool", &Options{})
	if err != nil {
		t.Fatalf("SelectExtractor(go fallback): %v", err)
	}

	assert.Eq(t, "go:tool.7z", extractor.(fakeExtractor).name)
}

func TestSelectExtractorKeepsTarGzOnGoExtractorEvenWithSystem7z(t *testing.T) {
	svc := NewService()
	svc.BinaryChooserFactory = func(tool string) any {
		return NewBinaryChooser(tool)
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return fakeExtractor{name: "go:" + filename}
	}
	svc.System7zPathResolver = func(configured string) string {
		return "C:/Tools/7z.exe"
	}
	svc.System7zExtractorFactory = func(filename, tool string, chooser Chooser, exe string) Extractor {
		return fakeExtractor{name: "system7z:" + filepath.Base(filename)}
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.tar.gz", "tool", &Options{})
	if err != nil {
		t.Fatalf("SelectExtractor(tar.gz): %v", err)
	}

	assert.Eq(t, "go:tool.tar.gz", extractor.(fakeExtractor).name)
}

func TestSelectExtractorDoesNotUseSystem7zForPureDownloadOnly(t *testing.T) {
	svc := NewService()
	svc.DownloadOnlyExtractorFactory = func(name string) any {
		return fakeExtractor{name: "download:" + name}
	}
	svc.System7zPathResolver = func(configured string) string {
		return "C:/Tools/7z.exe"
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.7z", "tool", &Options{DownloadOnly: true})
	if err != nil {
		t.Fatalf("SelectExtractor(download-only): %v", err)
	}

	assert.Eq(t, "download:tool.7z", extractor.(fakeExtractor).name)
}
```

- [x] **Step 2：运行选择测试确认失败**

执行：

```bash
go test ./internal/install -run 'SelectExtractor.*System7z|SelectExtractorKeepsTarGz|SelectExtractorDoesNotUseSystem7z' -v
```

预期：失败，提示注入字段不存在或选择逻辑未接入。

- [x] **Step 3：给 Service 增加注入点**

在 `internal/install/service.go` 的 `Service` 增加字段：

```go
System7zPathResolver      func(configured string) string
System7zExtractorFactory  func(filename, tool string, chooser Chooser, exe string) Extractor
```

在 `NewDefaultService()` 中设置默认值：

```go
System7zPathResolver: resolveSystem7zPath,
System7zExtractorFactory: func(filename, tool string, chooser Chooser, exe string) Extractor {
	return NewSystem7zExtractor(filename, tool, chooser, exe)
},
```

- [x] **Step 4：在 SelectExtractor 中接入系统 7z**

在 `Service.SelectExtractor()` 中先保留 `DownloadOnly` 分支。为 `ExtractFile`、`All`、默认 binary 三个分支创建 chooser 后，统一调用 helper：

```go
return s.newExtractor(filename, tool, chooser, opts)
```

新增 helper：

```go
func (s *Service) newExtractor(filename, tool string, chooser any, opts *Options) (any, error) {
	typedChooser, ok := chooser.(Chooser)
	if !ok {
		return nil, fmt.Errorf("unexpected chooser type %T", chooser)
	}
	if opts != nil && shouldUseSystem7z(filename, opts.All) {
		resolver := s.System7zPathResolver
		if resolver == nil {
			resolver = resolveSystem7zPath
		}
		if exe := resolver(opts.Sys7zPath); exe != "" {
			factory := s.System7zExtractorFactory
			if factory == nil {
				factory = func(filename, tool string, chooser Chooser, exe string) Extractor {
					return NewSystem7zExtractor(filename, tool, chooser, exe)
				}
			}
			return factory(filename, tool, typedChooser, exe), nil
		}
	}
	if s.ExtractorFactory == nil {
		return nil, fmt.Errorf("extractor factories are required")
	}
	return s.ExtractorFactory(filename, tool, typedChooser), nil
}
```

- [x] **Step 5：补齐临时 constructor**

在 `internal/install/system7z.go` 增加最小类型、constructor 和临时 `Extract` 方法，实际 extractor 在后续任务完善。这里必须先提供 `Extract` 方法，否则 `NewDefaultService()` 中的 `System7zExtractorFactory` 无法返回 `Extractor` 接口：

```go
type System7zExtractor struct {
	Filename string
	Tool     string
	Chooser  Chooser
	Exe      string
}

func NewSystem7zExtractor(filename, tool string, chooser Chooser, exe string) *System7zExtractor {
	return &System7zExtractor{Filename: filename, Tool: tool, Chooser: chooser, Exe: exe}
}

func (e *System7zExtractor) Extract(data []byte, multiple bool) (ExtractedFile, []ExtractedFile, error) {
	return ExtractedFile{}, nil, fmt.Errorf("system 7z extractor is not implemented")
}
```

确保 `internal/install/system7z.go` 已导入 `fmt`。

- [x] **Step 6：运行选择测试确认通过**

执行：

```bash
go test ./internal/install -run 'SelectExtractor.*System7z|SelectExtractorKeepsTarGz|SelectExtractorDoesNotUseSystem7z' -v
```

预期：通过。

- [x] **Step 7：提交选择逻辑**

执行：

```bash
git add internal/install/service.go internal/install/service_test.go internal/install/system7z.go
git commit -m "feat(install): prefer system 7z for supported archives"
```

---

## Task 5：实现系统 7z extractor

**文件：**
- 修改：`internal/install/system7z.go`
- 修改：`internal/install/system7z_test.go`

- [x] **Step 1：定义命令 runner 以便测试**

在 `internal/install/system7z.go` 增加：

```go
type system7zCommandRunner func(exe string, args ...string) ([]byte, error)

var runSystem7zCommand system7zCommandRunner = func(exe string, args ...string) ([]byte, error) {
	cmd := exec.Command(exe, args...)
	return cmd.CombinedOutput()
}
```

- [x] **Step 2：编写 `7z l -slt` 解析测试**

在 `internal/install/system7z_test.go` 增加：

```go
func TestParseSystem7zListOutput(t *testing.T) {
	output := `
Path = tool.7z
Type = 7z

----------
Path = bin/tool.exe
Size = 12
Packed Size = 8
Modified = 2026-05-13 10:00:00
Attributes = A
CRC = 12345678
Encrypted = -
Method = LZMA2
Block = 0

Path = docs/
Folder = +
Size = 0
Packed Size = 0
Attributes = D
`

	files, err := parseSystem7zListOutput([]byte(output))
	if err != nil {
		t.Fatalf("parse 7z list output: %v", err)
	}

	assert.Eq(t, 2, len(files))
	assert.Eq(t, "bin/tool.exe", files[0].Name)
	assert.False(t, files[0].Dir())
	assert.Eq(t, "docs", files[1].Name)
	assert.True(t, files[1].Dir())
}
```

- [x] **Step 3：编写路径穿越测试**

在 `internal/install/system7z_test.go` 增加：

```go
func TestParseSystem7zListOutputRejectsUnsafePath(t *testing.T) {
	output := `
Path = evil.7z
Type = 7z

----------
Path = ../evil.exe
Size = 1
`

	_, err := parseSystem7zListOutput([]byte(output))
	if err == nil {
		t.Fatal("expected unsafe archive path error")
	}
	assert.Contains(t, err.Error(), "unsafe archive path")
}
```

- [x] **Step 4：实现 list 输出解析**

在 `internal/install/system7z.go` 增加 parser：

```go
func parseSystem7zListOutput(output []byte) ([]File, error) {
	blocks := strings.Split(string(output), "\n\n")
	files := make([]File, 0)
	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		fields := map[string]string{}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || line == "----------" {
				continue
			}
			key, value, ok := strings.Cut(line, " = ")
			if !ok {
				continue
			}
			fields[key] = value
		}
		rawPath := fields["Path"]
		if rawPath == "" {
			continue
		}
		if _, ok := fields["Size"]; !ok {
			continue
		}
		name, err := safeArchiveRelativePath(rawPath)
		if err != nil {
			return nil, err
		}
		typ := TypeNormal
		if fields["Folder"] == "+" || strings.HasSuffix(rawPath, "/") || strings.HasSuffix(rawPath, `\`) {
			typ = TypeDir
		}
		files = append(files, File{Name: name, Mode: 0o666, Type: typ})
	}
	return files, nil
}
```

- [x] **Step 5：编写 extractor 行为测试**

在 `internal/install/system7z_test.go` 增加：

```go
func TestSystem7zExtractorSelectsCandidateAndExtractsFile(t *testing.T) {
	tmp := t.TempDir()
	var extractArgs []string
	origRunner := runSystem7zCommand
	defer func() { runSystem7zCommand = origRunner }()

	runSystem7zCommand = func(exe string, args ...string) ([]byte, error) {
		if args[0] == "l" {
			return []byte(`
Path = tool.7z
Type = 7z

----------
Path = bin/tool.exe
Size = 4
`), nil
		}
		extractArgs = append([]string(nil), args...)
		outDir := ""
		member := args[len(args)-1]
		for _, arg := range args {
			if strings.HasPrefix(arg, "-o") {
				outDir = strings.TrimPrefix(arg, "-o")
			}
		}
		if outDir == "" {
			t.Fatal("expected output dir")
		}
		extracted := filepath.Join(outDir, filepath.FromSlash(member))
		if err := os.MkdirAll(filepath.Dir(extracted), 0o755); err != nil {
			return nil, err
		}
		return nil, os.WriteFile(extracted, []byte("tool"), 0o755)
	}

	extractor := NewSystem7zExtractor("tool.7z", "tool", NewBinaryChooser("tool"), "7z")
	file, candidates, err := extractor.Extract([]byte("archive"), false)
	if err != nil {
		t.Fatalf("extract candidate: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected direct selected file, got candidates %#v", candidates)
	}

	out := filepath.Join(tmp, "tool.exe")
	if err := file.Extract(out); err != nil {
		t.Fatalf("extract selected file: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	assert.Eq(t, "tool", string(data))
	assert.Contains(t, strings.Join(extractArgs, " "), "bin/tool.exe")
}
```

- [x] **Step 6：实现 System7zExtractor.Extract**

实现要求：

- 把下载的 `data` 写入临时 archive 文件。
- 调用 `7z l -slt <tempArchive>` 获取成员列表。
- 用 `parseSystem7zListOutput()` 转换成 `[]File`。
- 对每个成员调用 `Chooser.Choose()`。
- 构造 `ExtractedFile`：
  - `Name` 和 `ArchiveName` 使用安全后的 archive name。
  - `mode` 用 `modeFrom(name, file.Mode)`。
  - `Extract(to string)` 创建临时目录，执行 `7z x -y -o<tempDir> <tempArchive> <archiveName>`，再把提取出来的文件或目录复制/移动到 `to`。
- `multiple == false` 且 `direct == true` 时直接返回单个文件。
- 候选数量为 1 时返回单个文件。
- 候选数量为 0 时返回 `target ... not found in archive`。
- 候选数量大于 1 时返回 candidates 和错误，让现有 runner 处理选择。

实现中需要新增 helper：

```go
func writeTempArchive(data []byte, filename string) (string, func(), error)
func copyExtractedPath(src, dst string, mode fs.FileMode) error
```

`copyExtractedPath()` 首版只需要支持普通文件和目录递归复制，路径必须来自 `safeArchiveOutputPath()` 计算结果。

- [x] **Step 7：运行 extractor 测试确认通过**

执行：

```bash
go test ./internal/install -run 'System7zExtractor|ParseSystem7zListOutput' -v
```

预期：通过。

- [x] **Step 8：提交系统 7z extractor**

执行：

```bash
git add internal/install/system7z.go internal/install/system7z_test.go
git commit -m "feat(install): extract archives with system 7z"
```

---

## Task 6：补充集成测试和文档

**文件：**
- 修改：`internal/install/service_test.go`
- 修改：`docs/DOCS.md`
- 修改：`docs/example.eget.toml`
- 修改：`README.md`
- 修改：`README.zh-CN.md`

- [x] **Step 1：补充 `.exe --all` 选择测试**

在 `internal/install/service_test.go` 增加：

```go
func TestSelectExtractorUsesSystem7zForExeExtractAll(t *testing.T) {
	svc := NewService()
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return NewFileChooser(pattern)
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return fakeExtractor{name: "go:" + filename}
	}
	svc.System7zPathResolver = func(configured string) string {
		return "C:/Tools/7z.exe"
	}
	svc.System7zExtractorFactory = func(filename, tool string, chooser Chooser, exe string) Extractor {
		return fakeExtractor{name: "system7z:" + filepath.Base(filename)}
	}

	extractor, err := svc.SelectExtractor("https://example.com/setup.exe", "setup", &Options{All: true})
	if err != nil {
		t.Fatalf("SelectExtractor(exe extract-all): %v", err)
	}

	assert.Eq(t, "system7z:setup.exe", extractor.(fakeExtractor).name)
}
```

- [x] **Step 2：运行 install 测试**

执行：

```bash
go test ./internal/install -run 'System7z|SelectExtractor.*7z|SelectExtractorKeepsTarGz|SelectExtractorUsesSystem7zForExeExtractAll' -v
```

预期：通过。

- [x] **Step 3：更新 `docs/example.eget.toml`**

在 `[global]` 示例中增加：

```toml
# Optional system 7z executable. If empty, eget searches PATH for 7z/7zz/7za.
sys7z_path = ""
```

- [x] **Step 4：更新 `docs/DOCS.md`**

在 Config Model 或目录相关语义处增加：

```markdown
- `sys7z_path`: 可选系统 7z 可执行文件路径。解压 `.7z`、`.rar`、`.msi`、`.cab`、`.iso`、以及 `--extract-all` 的 `.exe` 时，eget 会按 `global.sys7z_path` -> `PATH` 中的 `7z`/`7zz`/`7za` -> 内置 Go 解压实现的顺序选择。
```

在解压流程说明处增加：

```markdown
`.tar.gz` / `.tgz` / `.tar.xz` / `.txz` / `.tar.bz2` / `.tbz` / `.tar.zst` 继续使用内置 Go 解压流程，以保持 tar 成员选择和路径安全校验稳定。
```

- [x] **Step 5：更新 README**

在 `README.md` 和 `README.zh-CN.md` 的配置项说明中补充 `global.sys7z_path`，中文 README 使用中文说明：

```markdown
- `global.sys7z_path`: optional 7z executable path. When empty, eget searches `PATH` for `7z`, `7zz`, then `7za`.
```

```markdown
- `global.sys7z_path`：可选的 7z 可执行文件路径。为空时会从 `PATH` 依次查找 `7z`、`7zz`、`7za`。
```

- [x] **Step 6：运行文档相关测试包**

执行：

```bash
go test ./internal/config ./internal/app ./internal/install -v
```

预期：通过。

- [x] **Step 7：提交文档和集成测试**

执行：

```bash
git add internal/install/service_test.go docs/DOCS.md docs/example.eget.toml README.md README.zh-CN.md
git commit -m "docs: document system 7z configuration"
```

---

## Task 7：全量验证

**文件：**
- 不新增文件

- [ ] **Step 1：格式化 Go 文件**

执行：

```bash
gofmt -w internal/config/model.go internal/config/merge.go internal/config/gookit.go internal/install/options.go internal/app/install.go internal/install/service.go internal/install/system7z.go internal/config/*_test.go internal/app/install_test.go internal/install/*_test.go
```

预期：无输出。

- [ ] **Step 2：运行全量测试**

执行：

```bash
go test ./...
```

预期：全部通过。

- [ ] **Step 3：检查工作区差异**

执行：

```bash
git status --short
git diff --stat
```

预期：只包含本计划相关文件变更，没有无关格式化或无关文件改动。

- [ ] **Step 4：提交最终修正**

如果 Task 7 中有格式化或修正改动，执行：

```bash
git add .
git commit -m "test: verify system 7z extractor"
```

如果没有新增改动，则不需要提交。

---

## 风险和取舍

- 系统 7z 的输出格式在不同版本间可能有差异。首版解析 `7z l -slt` 的稳定字段：`Path`、`Size`、`Folder`。
- `.zip` 当前 Go 实现已经稳定，首版不接管，避免引入外部命令带来的行为差异。
- `tar.*` 当前 Go 流程更适合项目现有的二进制选择、`--file`、`--all` 和路径安全逻辑，首版不接管。
- `.rar` 没有 Go fallback 支持时，如果系统 7z 不存在，仍会按现有单文件逻辑保存原文件；这是当前 fallback 能力边界。后续如需对 `.rar` 缺少系统 7z 时直接提示错误，需要单独设计。
- 外部 7z 已进入解压后失败时返回错误，不再回退 Go extractor，避免隐藏真实 archive 或权限问题。

## Self-Review

- Spec coverage：`global.sys7z_path`、`PATH` 查找、系统 7z 优先、无系统 7z 回退 Go extractor、`tar.*` 继续走现有流程、文档和全量测试均有任务覆盖。
- Placeholder scan：没有 `TBD` / `TODO` / “后续实现” 等占位步骤；每个实现任务都包含目标文件、测试和命令。
- Type consistency：`Sys7zPath` 在 `config.Section`、`config.Merged`、`install.Options` 中命名一致；`System7zExtractor`、`resolveSystem7zPath`、`shouldUseSystem7z`、`NewSystem7zExtractor` 在使用前定义。
