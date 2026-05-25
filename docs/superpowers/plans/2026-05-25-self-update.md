# Self Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `eget update --self` so eget can detect, download, and replace itself from `inherelab/eget` GitHub releases.

**Architecture:** Keep self-update separate from normal package update because it updates the running executable rather than an installed-store package. Reuse the existing GitHub release discovery, asset filtering, download, archive extraction, proxy/cache, and progress behavior where it is safe, then add a small self-update service responsible for current-version checks and executable replacement.

**Tech Stack:** Go standard library, existing `internal/install` runner/service, existing `internal/source/github` release API client, current CLI command wiring under `internal/cli`.

---

## Scope

首版实现：

- `eget update --self`
- `eget update --self --check`
- 当前平台资产选择：`eget-<version>-<goos>-<goarch>.zip`
- 当前版本来自 build-time `Version`
- 最新版本来自 GitHub latest release tag
- Linux/macOS 直接替换当前 executable
- Windows 使用后台 helper/cmd 等待当前进程退出后替换 executable
- 支持现有网络配置：proxy、api cache、ghproxy、disable SSL、chunk concurrency
- 支持 `--tag` 指定版本
- 支持 `--asset` 覆盖资产过滤
- 不写普通 installed store
- 不支持降级，除非用户指定 `--tag`
- 不支持从源码自编译
- 不支持更新通过包管理器安装的 eget；只尝试替换当前 executable path，权限不足时给出明确错误

## File Structure

- Modify: `internal/cli/update_cmd.go`
  - Add `Self bool` to `UpdateOptions`.
  - Add `--self` flag.
- Modify: `internal/cli/app.go`
  - Expose current build info to app services with a small exported value or getter.
- Modify: `internal/cli/service.go`
  - Add `selfUpdateService` field to `cliService`.
- Modify: `internal/cli/wiring.go`
  - Construct `app.SelfUpdateService` with default dependencies.
- Modify: `internal/cli/handlers.go`
  - Route `update --self` before normal update paths.
  - Reject invalid combinations such as `--self --all` or `--self <target>`.
- Create: `internal/app/self_update.go`
  - Own high-level self-update flow: version check, install target/options construction, replacement.
- Create: `internal/app/self_update_test.go`
  - Unit tests for self-update behavior independent from CLI.
- Create: `internal/app/self_replace.go`
  - Cross-platform replacement interface and shared result type.
- Create: `internal/app/self_replace_default.go`
  - Non-Windows executable replacement implementation.
- Create: `internal/app/self_replace_windows.go`
  - Windows replacement implementation.
- Create: `internal/app/self_replace_windows_test.go`
  - Tests for generated Windows helper command/script behavior.
- Modify: `internal/app/main_test.go` or `internal/cli/app_test.go`
  - Add CLI flag parsing/routing tests.
- Modify: `README.md`
  - Document `eget update --self`.
- Modify: `README.zh-CN.md`
  - Document `eget update --self`.
- Modify: `docs/architecture.md`
  - Document self-update as a special update path.

## Design Details

`SelfUpdateService` should depend on small interfaces:

```go
type SelfUpdateInstaller interface {
	DownloadTarget(target string, opts install.Options) (RunResult, error)
}

type ExecutableReplacer interface {
	Replace(currentPath, replacementPath string) (SelfReplaceResult, error)
}
```

The service should call `DownloadTarget("inherelab/eget", opts)` with:

```go
install.Options{
	Tag:              opts.Tag,
	System:           runtime.GOOS + "/" + runtime.GOARCH,
	Asset:            []string{"PRE:eget-", runtime.GOOS + "-" + runtime.GOARCH, "SUF:.zip"},
	ExtractFile:      expectedExecutableName(runtime.GOOS, runtime.GOARCH),
	Output:           tempDir,
	OutputExplicit:   false,
	CacheDir:         inheritedCacheDir,
	ProxyURL:         inheritedProxyURL,
	APICacheEnabled:  inheritedAPICacheEnabled,
	APICacheDir:      inheritedAPICacheDir,
	APICacheTime:     inheritedAPICacheTime,
	GhproxyEnabled:   inheritedGhproxyEnabled,
	GhproxyHostURL:   inheritedGhproxyHostURL,
	GhproxySupportAPI: inheritedGhproxySupportAPI,
	GhproxyFallbacks: inheritedGhproxyFallbacks,
	DisableSSL:       inheritedDisableSSL,
}
```

If `--asset` is provided, use it instead of the default asset filters. If `--tag` is provided, skip the "already latest" check and install that tag.

The replacement path from `DownloadTarget` is the single extracted file. The service must validate:

- exactly one extracted file exists
- extracted file basename matches expected binary name for current platform
- replacement file exists and is not a directory
- current executable path resolves via `os.Executable()`

## Task 1: CLI Flag And Validation

**Files:**

- Modify: `internal/cli/update_cmd.go`
- Modify: `internal/cli/handlers.go`
- Test: `internal/cli/app_test.go`

- [x] **Step 1: Write failing CLI parse test**

Add a test that verifies `update --self` reaches the update handler with `Self: true` and no target.

```go
func TestUpdateSelfFlagParses(t *testing.T) {
	var got *UpdateOptions
	app := newApp(func(name string, options any) error {
		if name != "update" {
			t.Fatalf("expected update command, got %q", name)
		}
		got = options.(*UpdateOptions)
		return nil
	}, io.Discard, io.Discard)

	err := app.RunWithArgs([]string{"update", "--self"})

	assert.NoErr(t, err)
	assert.True(t, got.Self)
	assert.Eq(t, 0, len(got.Targets))
}
```

- [x] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./internal/cli -run TestUpdateSelfFlagParses -count=1
```

Expected: FAIL because `UpdateOptions.Self` does not exist or `--self` is unknown.

- [x] **Step 3: Add the flag**

Modify `internal/cli/update_cmd.go`:

```go
type UpdateOptions struct {
	All              bool
	Check            bool
	DryRun           bool
	Interactive      bool
	Self             bool
	Tag              string
	System           string
	To               string
	File             string
	Asset            string
	Source           bool
	Quiet            bool
	ChunkConcurrency int
	BatchConcurrency int
	Targets          []string
}
```

Add in command config:

```go
c.BoolOpt(&opts.Self, "self", "", false, "Update eget itself")
```

Reset:

```go
*opts = UpdateOptions{ChunkConcurrency: -1, BatchConcurrency: -1}
```

- [x] **Step 4: Add validation tests**

Add tests around `handleUpdate` using a `cliService` with fake services:

```go
func TestHandleUpdateSelfRejectsTargets(t *testing.T) {
	svc := &cliService{}
	err := svc.handleUpdate(&UpdateOptions{Self: true, Targets: []string{"junegunn/fzf"}})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "update --self cannot be used with target")
}

func TestHandleUpdateSelfRejectsAll(t *testing.T) {
	svc := &cliService{}
	err := svc.handleUpdate(&UpdateOptions{Self: true, All: true})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "update --self cannot be used with --all")
}
```

- [x] **Step 5: Implement validation skeleton**

Modify `handleUpdate` near the top:

```go
if opts.Self {
	if opts.All {
		return fmt.Errorf("update --self cannot be used with --all")
	}
	if len(opts.Targets) > 0 {
		return fmt.Errorf("update --self cannot be used with target")
	}
	if opts.BatchConcurrency > 0 {
		return fmt.Errorf("--batch can only be used with --all")
	}
	return fmt.Errorf("update --self is not implemented")
}
```

- [x] **Step 6: Run CLI tests**

Run:

```bash
go test ./internal/cli -run "TestUpdateSelfFlagParses|TestHandleUpdateSelf" -count=1
```

Expected: parse/validation tests pass; valid `update --self` returns the temporary not-implemented error added in this task.

- [x] **Step 7: Commit**

```bash
git add internal/cli/update_cmd.go internal/cli/handlers.go internal/cli/app_test.go
git commit -m "feat(update): add self update flag"
```

## Task 2: Build Info Access

**Files:**

- Modify: `internal/cli/app.go`
- Test: `internal/cli/app_test.go`

- [x] **Step 1: Write failing build info test**

```go
func TestBuildInfoReturnsConfiguredValues(t *testing.T) {
	SetBuildInfo("1.7.1", "abcdef12", "2026-05-25T10:20:30")

	info := BuildInfo()

	assert.Eq(t, "1.7.1", info.Version)
	assert.Eq(t, "abcdef12", info.GitHash)
	assert.Eq(t, "20260525-102030", info.BuildTime)
}
```

- [x] **Step 2: Run test and verify it fails**

```bash
go test ./internal/cli -run TestBuildInfoReturnsConfiguredValues -count=1
```

Expected: FAIL because `BuildInfo` does not exist.

- [x] **Step 3: Implement exported build info**

Add to `internal/cli/app.go`:

```go
type Info struct {
	Version   string
	GitHash   string
	BuildTime string
}

func BuildInfo() Info {
	return Info{Version: version, GitHash: gitHash, BuildTime: buildTime}
}
```

- [x] **Step 4: Run tests**

```bash
go test ./internal/cli -run TestBuildInfoReturnsConfiguredValues -count=1
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/cli/app.go internal/cli/app_test.go
git commit -m "feat(cli): expose build info"
```

## Task 3: Self Update Service Version Decisions

**Files:**

- Create: `internal/app/self_update.go`
- Create: `internal/app/self_update_test.go`

- [ ] **Step 1: Write failing tests for version decisions**

Tests:

```go
func TestSelfUpdateSkipsWhenLatestMatchesCurrentVersion(t *testing.T) {
	svc := SelfUpdateService{
		CurrentVersion: "1.7.1",
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			assert.Eq(t, "inherelab/eget", target.Repo)
			return LatestInfo{Tag: "v1.7.1"}, nil
		},
	}

	result, err := svc.Update(SelfUpdateOptions{CheckOnly: true})

	assert.NoErr(t, err)
	assert.False(t, result.Updated)
	assert.False(t, result.Outdated)
	assert.Eq(t, "v1.7.1", result.LatestVersion)
}

func TestSelfUpdateReportsOutdatedWhenLatestDiffers(t *testing.T) {
	svc := SelfUpdateService{
		CurrentVersion: "1.7.1",
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			return LatestInfo{Tag: "v1.7.2"}, nil
		},
	}

	result, err := svc.Update(SelfUpdateOptions{CheckOnly: true})

	assert.NoErr(t, err)
	assert.True(t, result.Outdated)
	assert.Eq(t, "1.7.1", result.CurrentVersion)
	assert.Eq(t, "v1.7.2", result.LatestVersion)
}
```

- [ ] **Step 2: Run test and verify it fails**

```bash
go test ./internal/app -run TestSelfUpdate -count=1
```

Expected: FAIL because `SelfUpdateService` is undefined.

- [ ] **Step 3: Implement minimal types and version comparison**

Create `internal/app/self_update.go`:

```go
package app

import (
	"fmt"
	"strings"

	"github.com/inherelab/eget/internal/install"
)

const SelfUpdateRepo = "inherelab/eget"

type SelfUpdateOptions struct {
	CheckOnly bool
	Tag       string
	Asset     []string
	Install   install.Options
}

type SelfUpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	Updated        bool
	Outdated       bool
	Replacement    string
	Executable     string
	Deferred       bool
}

type SelfUpdateService struct {
	CurrentVersion string
	LatestInfo     LatestInfoFunc
}

func (s SelfUpdateService) Update(opts SelfUpdateOptions) (SelfUpdateResult, error) {
	current := normalizeSelfVersion(s.CurrentVersion)
	result := SelfUpdateResult{CurrentVersion: s.CurrentVersion}
	if opts.Tag != "" {
		result.LatestVersion = opts.Tag
		result.Outdated = normalizeSelfVersion(opts.Tag) != current
		return result, nil
	}
	if s.LatestInfo == nil {
		return SelfUpdateResult{}, fmt.Errorf("latest info checker is required")
	}
	latest, err := s.LatestInfo(LatestCheckTarget{Repo: SelfUpdateRepo})
	if err != nil {
		return SelfUpdateResult{}, err
	}
	result.LatestVersion = latest.Tag
	result.Outdated = normalizeSelfVersion(latest.Tag) != current
	return result, nil
}

func normalizeSelfVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	return value
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/app -run TestSelfUpdate -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/self_update.go internal/app/self_update_test.go
git commit -m "feat(update): add self update version checks"
```

## Task 4: Self Update Download Options

**Files:**

- Modify: `internal/app/self_update.go`
- Modify: `internal/app/self_update_test.go`

- [ ] **Step 1: Write failing test for installer invocation**

```go
type fakeSelfUpdateInstaller struct {
	target string
	opts   install.Options
	result RunResult
}

func (f *fakeSelfUpdateInstaller) DownloadTarget(target string, opts install.Options) (RunResult, error) {
	f.target = target
	f.opts = opts
	return f.result, nil
}

func TestSelfUpdateDownloadsExpectedPlatformAsset(t *testing.T) {
	installer := &fakeSelfUpdateInstaller{
		result: RunResult{ExtractedFiles: []string{filepath.Join(t.TempDir(), "eget")}},
	}
	svc := SelfUpdateService{
		CurrentVersion: "1.7.1",
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			return LatestInfo{Tag: "v1.7.2"}, nil
		},
		Installer: installer,
		RuntimeGOOS: "linux",
		RuntimeGOARCH: "amd64",
		ExecutablePath: func() (string, error) {
			return filepath.Join(t.TempDir(), "eget"), nil
		},
		Replacer: fakeSelfReplacer{},
	}

	_, err := svc.Update(SelfUpdateOptions{})

	assert.NoErr(t, err)
	assert.Eq(t, SelfUpdateRepo, installer.target)
	assert.Eq(t, "linux/amd64", installer.opts.System)
	assert.Eq(t, "eget-linux-amd64", installer.opts.ExtractFile)
	assert.Eq(t, []string{"PRE:eget-", "linux-amd64", "SUF:.zip"}, installer.opts.Asset)
}
```

- [ ] **Step 2: Run test and verify it fails**

```bash
go test ./internal/app -run TestSelfUpdateDownloadsExpectedPlatformAsset -count=1
```

Expected: FAIL because installer/replacer fields and download flow do not exist.

- [ ] **Step 3: Implement installer dependency and option builder**

Add:

```go
type SelfUpdateInstaller interface {
	DownloadTarget(target string, opts install.Options) (RunResult, error)
}

type SelfUpdateService struct {
	CurrentVersion string
	LatestInfo     LatestInfoFunc
	Installer      SelfUpdateInstaller
	Replacer       ExecutableReplacer
	RuntimeGOOS    string
	RuntimeGOARCH  string
	ExecutablePath func() (string, error)
}
```

Add helpers:

```go
func selfUpdateSystem(goos, goarch string) string {
	return goos + "/" + goarch
}

func selfUpdateAssetFilters(goos, goarch string) []string {
	return []string{"PRE:eget-", goos + "-" + goarch, "SUF:.zip"}
}

func selfUpdateExtractFile(goos, goarch string) string {
	name := "eget-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}
```

Implement download after outdated check:

```go
if opts.CheckOnly || !result.Outdated {
	return result, nil
}
if s.Installer == nil {
	return SelfUpdateResult{}, fmt.Errorf("self update installer is required")
}
goos, goarch := s.runtimePlatform()
installOpts := opts.Install
installOpts.System = selfUpdateSystem(goos, goarch)
installOpts.Asset = selfUpdateAssetFilters(goos, goarch)
if len(opts.Asset) > 0 {
	installOpts.Asset = append([]string(nil), opts.Asset...)
}
installOpts.ExtractFile = selfUpdateExtractFile(goos, goarch)
downloaded, err := s.Installer.DownloadTarget(SelfUpdateRepo, installOpts)
if err != nil {
	return SelfUpdateResult{}, err
}
replacement, err := singleSelfUpdateReplacement(downloaded)
if err != nil {
	return SelfUpdateResult{}, err
}
result.Replacement = replacement
return result, nil
```

- [ ] **Step 4: Add helper tests for Windows naming**

```go
func TestSelfUpdateExtractFileUsesExeOnWindows(t *testing.T) {
	assert.Eq(t, "eget-windows-amd64.exe", selfUpdateExtractFile("windows", "amd64"))
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/app -run "TestSelfUpdate|TestSelfUpdateExtractFile" -count=1
```

Expected: PASS except replacement tests deferred to next task if replacer is still stubbed.

- [ ] **Step 6: Commit**

```bash
git add internal/app/self_update.go internal/app/self_update_test.go
git commit -m "feat(update): prepare self update download"
```

## Task 5: Executable Replacement

**Files:**

- Create: `internal/app/self_replace.go`
- Create: `internal/app/self_replace_default.go`
- Create: `internal/app/self_replace_windows.go`
- Create: `internal/app/self_replace_windows_test.go`
- Modify: `internal/app/self_update.go`
- Modify: `internal/app/self_update_test.go`

- [ ] **Step 1: Write failing non-Windows replacement test**

In `internal/app/self_update_test.go`:

```go
type fakeSelfReplacer struct {
	current     string
	replacement string
	result      SelfReplaceResult
}

func (f fakeSelfReplacer) Replace(currentPath, replacementPath string) (SelfReplaceResult, error) {
	f.current = currentPath
	f.replacement = replacementPath
	return f.result, nil
}

func TestSelfUpdateReplacesExecutable(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "eget")
	replacement := filepath.Join(dir, "download", "eget")
	assert.NoErr(t, os.MkdirAll(filepath.Dir(replacement), 0o755))
	assert.NoErr(t, os.WriteFile(replacement, []byte("new"), 0o755))
	installer := &fakeSelfUpdateInstaller{
		result: RunResult{ExtractedFiles: []string{replacement}},
	}
	replacer := &recordingSelfReplacer{}
	svc := SelfUpdateService{
		CurrentVersion: "1.7.1",
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			return LatestInfo{Tag: "v1.7.2"}, nil
		},
		Installer: installer,
		Replacer: replacer,
		RuntimeGOOS: "linux",
		RuntimeGOARCH: "amd64",
		ExecutablePath: func() (string, error) { return current, nil },
	}

	result, err := svc.Update(SelfUpdateOptions{})

	assert.NoErr(t, err)
	assert.True(t, result.Updated)
	assert.Eq(t, current, replacer.current)
	assert.Eq(t, replacement, replacer.replacement)
}
```

- [ ] **Step 2: Implement replacement interfaces**

Create `internal/app/self_replace.go`:

```go
package app

type SelfReplaceResult struct {
	Deferred bool
}

type ExecutableReplacer interface {
	Replace(currentPath, replacementPath string) (SelfReplaceResult, error)
}
```

Create `internal/app/self_replace_default.go` with `//go:build !windows`:

```go
//go:build !windows

package app

import "os"

type DefaultExecutableReplacer struct{}

func (DefaultExecutableReplacer) Replace(currentPath, replacementPath string) (SelfReplaceResult, error) {
	info, err := os.Stat(currentPath)
	if err != nil {
		return SelfReplaceResult{}, err
	}
	if err := os.Chmod(replacementPath, info.Mode()); err != nil {
		return SelfReplaceResult{}, err
	}
	backup := currentPath + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(currentPath, backup); err != nil {
		return SelfReplaceResult{}, err
	}
	if err := os.Rename(replacementPath, currentPath); err != nil {
		_ = os.Rename(backup, currentPath)
		return SelfReplaceResult{}, err
	}
	_ = os.Remove(backup)
	return SelfReplaceResult{}, nil
}
```

- [ ] **Step 3: Wire replacement into service**

In `SelfUpdateService.Update`, after `result.Replacement = replacement`:

```go
exePath, err := s.executablePath()
if err != nil {
	return SelfUpdateResult{}, err
}
replaceResult, err := s.replacer().Replace(exePath, replacement)
if err != nil {
	return SelfUpdateResult{}, err
}
result.Executable = exePath
result.Deferred = replaceResult.Deferred
result.Updated = true
return result, nil
```

Add helper methods:

```go
func (s SelfUpdateService) executablePath() (string, error) {
	if s.ExecutablePath != nil {
		return s.ExecutablePath()
	}
	return os.Executable()
}

func (s SelfUpdateService) replacer() ExecutableReplacer {
	if s.Replacer != nil {
		return s.Replacer
	}
	return DefaultExecutableReplacer{}
}
```

- [ ] **Step 4: Write Windows command generation test**

In `internal/app/self_replace_windows_test.go`:

```go
func TestWindowsSelfReplaceScriptContainsQuotedPaths(t *testing.T) {
	script := windowsSelfReplaceScript(`C:\Tools\eget.exe`, `C:\Temp\eget-new.exe`, `C:\Tools\eget.exe.old`)

	assert.Contains(t, script, `"C:\Tools\eget.exe"`)
	assert.Contains(t, script, `"C:\Temp\eget-new.exe"`)
	assert.Contains(t, script, `"C:\Tools\eget.exe.old"`)
	assert.Contains(t, script, "move /Y")
}
```

- [ ] **Step 5: Implement Windows deferred replacement**

Create `internal/app/self_replace_windows.go` with `//go:build windows`:

```go
package app

import (
	"fmt"
	"os"
	"os/exec"
)

type DefaultExecutableReplacer struct{}

func (DefaultExecutableReplacer) Replace(currentPath, replacementPath string) (SelfReplaceResult, error) {
	backup := currentPath + ".old"
	script := windowsSelfReplaceScript(currentPath, replacementPath, backup)
	scriptFile, err := os.CreateTemp("", "eget-self-update-*.cmd")
	if err != nil {
		return SelfReplaceResult{}, err
	}
	if _, err := scriptFile.WriteString(script); err != nil {
		_ = scriptFile.Close()
		return SelfReplaceResult{}, err
	}
	if err := scriptFile.Close(); err != nil {
		return SelfReplaceResult{}, err
	}
	cmd := exec.Command("cmd", "/C", "start", "", "/B", scriptFile.Name())
	if err := cmd.Start(); err != nil {
		return SelfReplaceResult{}, err
	}
	return SelfReplaceResult{Deferred: true}, nil
}

func windowsSelfReplaceScript(currentPath, replacementPath, backupPath string) string {
	return fmt.Sprintf(`@echo off
setlocal
:wait
move /Y "%[1]s" "%[3]s" >nul 2>nul
if errorlevel 1 (
  timeout /T 1 /NOBREAK >nul
  goto wait
)
move /Y "%[2]s" "%[1]s" >nul 2>nul
if errorlevel 1 (
  move /Y "%[3]s" "%[1]s" >nul 2>nul
  exit /B 1
)
del /F /Q "%[3]s" >nul 2>nul
del /F /Q "%%~f0" >nul 2>nul
`, currentPath, replacementPath, backupPath)
}
```

- [ ] **Step 6: Run app tests**

```bash
go test ./internal/app -run "TestSelfUpdate|TestWindowsSelfReplaceScript" -count=1
```

Expected: PASS on the current platform. Windows-only tests compile only on Windows if guarded by build tags.

- [ ] **Step 7: Commit**

```bash
git add internal/app/self_update.go internal/app/self_update_test.go internal/app/self_replace.go internal/app/self_replace_default.go internal/app/self_replace_windows.go internal/app/self_replace_windows_test.go
git commit -m "feat(update): replace executable during self update"
```

## Task 6: CLI Wiring And Output

**Files:**

- Modify: `internal/cli/service.go`
- Modify: `internal/cli/wiring.go`
- Modify: `internal/cli/handlers.go`
- Test: `internal/cli/service_test.go`

- [ ] **Step 1: Define CLI-facing self update interface**

In `internal/cli/service.go`:

```go
type selfUpdateCLIService interface {
	Update(app.SelfUpdateOptions) (app.SelfUpdateResult, error)
}
```

Add field:

```go
selfUpdateService selfUpdateCLIService
```

- [ ] **Step 2: Write failing handler test**

```go
type fakeSelfUpdateCLIService struct {
	opts app.SelfUpdateOptions
	result app.SelfUpdateResult
}

func (f *fakeSelfUpdateCLIService) Update(opts app.SelfUpdateOptions) (app.SelfUpdateResult, error) {
	f.opts = opts
	return f.result, nil
}

func TestHandleUpdateSelfRunsSelfUpdateService(t *testing.T) {
	fake := &fakeSelfUpdateCLIService{
		result: app.SelfUpdateResult{CurrentVersion: "1.7.1", LatestVersion: "v1.7.2", Updated: true},
	}
	svc := &cliService{selfUpdateService: fake, stderr: io.Discard}

	err := svc.handleUpdate(&UpdateOptions{Self: true, Tag: "v1.7.2", Quiet: true})

	assert.NoErr(t, err)
	assert.Eq(t, "v1.7.2", fake.opts.Tag)
	assert.True(t, fake.opts.Install.Quiet)
}
```

- [ ] **Step 3: Implement handler call**

Replace the temporary not implemented branch:

```go
if s.selfUpdateService == nil {
	return fmt.Errorf("self update service is required")
}
result, err := s.selfUpdateService.Update(app.SelfUpdateOptions{
	CheckOnly: opts.Check,
	Tag:       opts.Tag,
	Asset:     splitAssetFilters(opts.Asset),
	Install:   installOptionsFromUpdate(opts),
})
if err != nil {
	return err
}
printSelfUpdateResult(result)
return nil
```

Add `printSelfUpdateResult` in `handlers.go`:

```go
func printSelfUpdateResult(result app.SelfUpdateResult) {
	if !result.Outdated && !result.Updated {
		ccolor.Cyanf("🎉 eget is already up to date: %s\n", result.CurrentVersion)
		return
	}
	if result.Updated && result.Deferred {
		ccolor.Successf("✅ eget update downloaded. It will be replaced after this process exits: %s\n", result.LatestVersion)
		return
	}
	if result.Updated {
		ccolor.Successf("✅ eget updated: %s -> %s\n", result.CurrentVersion, result.LatestVersion)
		return
	}
	ccolor.Infof("⬆️ eget update available: %s -> %s\n", result.CurrentVersion, result.LatestVersion)
}
```

- [ ] **Step 4: Wire default service**

In `internal/cli/wiring.go`, construct:

```go
selfUpdateService := app.SelfUpdateService{
	CurrentVersion: BuildInfo().Version,
	LatestInfo:     app.DefaultLatestInfo,
	Installer:      appService,
}
```

Assign to `cliService`.

If the project uses a different existing latest-info function name, use that existing function rather than adding a duplicate.

- [ ] **Step 5: Run CLI tests**

```bash
go test ./internal/cli -run "TestHandleUpdateSelf|TestUpdateSelf" -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/service.go internal/cli/wiring.go internal/cli/handlers.go internal/cli/service_test.go
git commit -m "feat(update): wire self update command"
```

## Task 7: Documentation

**Files:**

- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/architecture.md`

- [ ] **Step 1: Update README command section**

Add near the existing update command documentation:

```markdown
# Update eget itself
eget update --self

# Check whether eget itself has a newer release
eget update --self --check
```

Explain:

```markdown
`update --self` checks `inherelab/eget` releases, downloads the matching asset for the current OS/arch, and replaces the running executable. On Windows the replacement is deferred until the current process exits.
```

- [ ] **Step 2: Update Chinese README**

Add:

```markdown
# 更新 eget 自身
eget update --self

# 仅检查 eget 自身是否有新版本
eget update --self --check
```

Explain:

```markdown
`update --self` 会检查 `inherelab/eget` release，下载当前 OS/arch 对应的 asset，并替换当前正在运行的 eget 可执行文件。Windows 下替换会延迟到当前进程退出后执行。
```

- [ ] **Step 3: Update architecture doc**

Add to update section:

```markdown
`update --self` is a special update path. It does not read or write the normal installed package store. It resolves the current executable with `os.Executable()`, downloads the matching `inherelab/eget` release asset into a temporary directory, extracts the platform binary, and replaces the current executable. Windows uses a deferred helper script because a running `.exe` cannot be overwritten.
```

- [ ] **Step 4: Commit**

```bash
git add README.md README.zh-CN.md docs/architecture.md
git commit -m "docs: document self update"
```

## Task 8: Full Verification

**Files:**

- No source edits unless tests reveal a bug.

- [ ] **Step 1: Run focused tests**

```bash
go test ./internal/app ./internal/cli ./internal/install -count=1
```

Expected: PASS.

- [ ] **Step 2: Run full test suite**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Manual smoke test with a local build**

Build:

```bash
make build
```

Run:

```bash
./eget update --self --check
```

Expected:

- If local build version matches latest release: prints already up to date.
- If local build is dev or older: prints update available or performs no mutation because `--check` is set.

- [ ] **Step 4: Manual smoke test for invalid combinations**

Run:

```bash
./eget update --self fzf
./eget update --self --all
```

Expected: both commands return validation errors.

- [ ] **Step 5: Final commit if fixes were needed**

Only if this task required changes:

```bash
git add <changed-files>
git commit -m "test(update): verify self update flow"
```

## Open Decisions Before Implementation

- Whether `dev`, `dirty`, or `unknown` current versions should always be considered outdated. Recommended: yes, unless `--check` only reports that current version is not a release build.
- Whether `--tag` should permit installing the same version. Recommended: yes, because it can repair a broken binary.
- Whether failed Unix replacement should leave `.old` backup. Recommended: restore immediately when final rename fails; remove backup only after success.

## Self Review

- Spec coverage: CLI flag, version check, asset selection, download, Unix replacement, Windows delayed replacement, docs, and tests are covered.
- Placeholder scan: no `TBD` or undefined "later" work remains; open decisions are explicit product choices before implementation.
- Type consistency: `SelfUpdateService`, `SelfUpdateOptions`, `SelfUpdateResult`, `SelfUpdateInstaller`, `ExecutableReplacer`, and `SelfReplaceResult` are consistently named across tasks.
