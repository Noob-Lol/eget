# gcli 迁移实现计划

> **给执行该计划的 agent：** 需要使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 逐任务执行。本计划使用 checkbox（`- [ ]`）跟踪进度。

**目标：** 将 `eget` CLI 从 `github.com/gookit/goutil/cflag/capp` 迁移到 `github.com/gookit/gcli/v3`，保持现有命令行为基本不变，并为后续 `sdk` 多层级命令做准备。

**架构：** 保持当前 CLI 到 app 层的边界稳定：`Main -> newApp -> RunWithArgs -> CommandHandler -> cliService.handle`。本次只替换 CLI 框架和命令构造代码，不实现 SDK 命令。

**技术栈：** Go 1.25、`github.com/gookit/gcli/v3`、现有 `internal/cli` option structs、现有 `go test` 测试套件。

---

## 参考文档

主要设计文档：

```text
docs/superpowers/specs/2026-05-15-gcli-migration-design.md
```

后续 SDK 设计文档：

```text
docs/superpowers/specs/2026-05-14-sdk-download-design.md
```

## 文件结构

需要修改：

```text
go.mod
go.sum
internal/cli/app.go
internal/cli/install_cmd.go
internal/cli/download_cmd.go
internal/cli/add_cmd.go
internal/cli/uninstall_cmd.go
internal/cli/list_cmd.go
internal/cli/update_cmd.go
internal/cli/config_cmd.go
internal/cli/query_cmd.go
internal/cli/search_cmd.go
internal/cli/app_test.go
internal/cli/search_cmd_test.go
```

只有当重复绑定代码明显变多时才新增：

```text
internal/cli/gcli_helpers.go
```

不要修改：

```text
internal/app/*
internal/install/*
internal/client/*
```

service 和业务行为必须保持不变。

## 命令等价矩阵

| 命令 | 别名 | 需要保持的关键行为 |
| --- | --- | --- |
| `install` | `i`, `ins` | 多 target、逗号拆分、`--all`、`--batch`、支持 trailing flag |
| `download` | `dl` | 单 target、`--chunk`、不支持 `--gui` |
| `add` | 无 | install-like flags，只写配置 |
| `uninstall` | 以当前源码为准 | 单 target |
| `list` | 以当前源码为准 | `--outdated`、`--all/-a`、`--gui`、`--info/-i` |
| `update` | 以当前源码为准 | 多 target、`--check`、`--all`、`--batch` |
| `config` | 以当前源码为准 | action/key/value 位置参数行为 |
| `query` | `q` | `--action/-a`、`--tag/-t`、`--limit/-l`、`--json/-j`、`--prerelease/-p` |
| `search` | 无 | keyword + extra conditions，多次运行不泄漏状态 |

## 任务 1：引入 `gcli` 依赖

**文件：**

- 修改：`go.mod`
- 修改：`go.sum`

- [x] **步骤 1：添加依赖**

运行：

```bash
go get github.com/gookit/gcli/v3@v3.3.1
```

预期：

```text
go.mod 包含 github.com/gookit/gcli/v3 v3.3.1
go.sum 包含 gcli 校验信息
```

- [x] **步骤 2：迁移代码前确认现有测试仍通过**

运行：

```bash
go test ./internal/cli
```

预期：

```text
ok   github.com/inherelab/eget/internal/cli
```

- [x] **步骤 3：提交依赖变更**

运行：

```bash
git add go.mod go.sum
git commit -m "chore(cli): add gcli dependency"
```

预期：

```text
只提交 go.mod/go.sum 变更
```

## 任务 2：迁移 App 壳

**文件：**

- 修改：`internal/cli/app.go`
- 测试：`internal/cli/app_test.go`

- [x] **步骤 1：更新 import 和 App 类型**

将 `internal/cli/app.go` 的 import 从：

```go
import (
    "errors"
    "fmt"
    "io"
    "time"

    "github.com/gookit/goutil/cflag/capp"
    "github.com/gookit/goutil/x/ccolor"
)
```

改为：

```go
import (
    "errors"
    "fmt"
    "io"
    "time"

    "github.com/gookit/gcli/v3"
    "github.com/gookit/goutil/x/ccolor"
)
```

将：

```go
type App struct {
    inner     *capp.App
    resetters []func()
    verbose   *bool
}
```

改为：

```go
type App struct {
    inner     *gcli.App
    resetters []func()
    verbose   *bool
}
```

- [x] **步骤 2：替换 app 构造逻辑**

重写 `newApp`，使用 `gcli.NewApp()` 或 `gcli v3.3.1` 中等价的 app 构造方式。

必须保持：

```text
inner.Name = "eget"
inner.Desc 包含 "Easy install and download tools from GitHub"
inner.Version = version
help 输出到 stdout
gcli 支持的 error/output writer 指向 stderr
```

不要手动注册 `--version` 或 `-V`，`gcli` 已经内置。只需要从现有包变量设置版本信息：

```go
version
gitHash
buildTime
```

如果 `gcli` 支持额外版本字段，展示 commit 和 build date。如果只支持单个 version string，则使用：

```text
<version> (<gitHash>, <buildTime>)
```

- [x] **步骤 3：重新绑定 verbose**

注册 `--verbose` 和 `-v` bool option，写入现有 `verbose` 变量。

保留：

```go
func (a *App) Verbose() bool {
    return a.verbose != nil && *a.verbose
}
```

- [x] **步骤 4：更新 add 和 RunWithArgs**

将：

```go
func (a *App) add(cmd *capp.Cmd, reset func()) {
    a.inner.Add(cmd)
    a.resetters = append(a.resetters, reset)
}
```

改为 `gcli.Command` 版本：

```go
func (a *App) add(cmd *gcli.Command, reset func()) {
    a.inner.Add(cmd)
    a.resetters = append(a.resetters, reset)
}
```

保留 `RunWithArgs` 的 reset 行为：

```go
for _, reset := range a.resetters {
    reset()
}
if a.verbose != nil {
    *a.verbose = false
}
return a.inner.RunWithArgs(args)
```

如果 `gcli.App` 使用 `Run(args)` 而不是 `RunWithArgs(args)`，就在 `App.RunWithArgs` 内部做一层适配，让外部测试继续使用当前方法。

- [x] **步骤 5：移除 capp 专用校验签名**

删除：

```go
func validateNoTrailingFlags(cmd *capp.Cmd) error
```

历史实现曾保留通用函数：

```go
func validateNoFlagArgs(args []string) error {
    for _, arg := range args {
        if len(arg) > 0 && arg[0] == '-' {
            return fmt.Errorf("flags must appear before arguments: %s", arg)
        }
    }
    return nil
}
```

现版本允许 trailing flag，`validateNoFlagArgs` 仅作为命令回调里的剩余参数兜底。

- [x] **步骤 6：运行 App 壳测试**

运行：

```bash
go test ./internal/cli -run "TestMain_NoSubcommandReturnsErrorAndHelp|TestSetBuildInfoCompactsBuildTime"
```

预期：

```text
ok   github.com/inherelab/eget/internal/cli
```

如果 help 不再包含 `Usage:`，但包含 `Commands:` 或 `Available Commands`，则把 `TestMain_NoSubcommandReturnsErrorAndHelp` 改为接受两类输出：

```go
help := stdout.String()
if !strings.Contains(help, "Usage:") && !strings.Contains(help, "Commands") {
    t.Fatalf("expected help output to contain usage or commands, got %q", help)
}
```

- [x] **步骤 7：提交 App 壳迁移**

运行：

```bash
git add internal/cli/app.go internal/cli/app_test.go
git commit -m "refactor(cli): migrate app shell to gcli"
```

预期：

```text
只提交 App 壳和对应测试变更
```

## 任务 3：迁移简单命令

**文件：**

- 修改：`internal/cli/list_cmd.go`
- 修改：`internal/cli/uninstall_cmd.go`
- 修改：`internal/cli/config_cmd.go`
- 修改：`internal/cli/query_cmd.go`
- 测试：`internal/cli/app_test.go`

- [x] **步骤 1：迁移 `list` 命令**

将 `capp` import 替换为：

```go
import "github.com/gookit/gcli/v3"
```

修改签名：

```go
func newListCmd(handler CommandHandler) (*gcli.Command, func())
```

构造 `gcli.Command`：

```text
Name: list
Desc: List managed packages
aliases: 与当前源码保持一致
```

绑定：

```text
--outdated / --old -> opts.Outdated
--all / -a -> opts.All
--gui -> opts.GUI
--info / -i -> opts.Info
```

在 `Func` 中复制 options 并调用：

```go
snapshot := *opts
return handler("list", &snapshot)
```

不要依赖 `c.Name` 是否为别名；固定传 `"list"` 更稳定。

- [x] **步骤 2：迁移 `uninstall` 命令**

修改签名：

```go
func newUninstallCmd(handler CommandHandler) (*gcli.Command, func())
```

绑定一个必填位置参数：

```text
target
```

在 `Func` 中：

```go
opts.Target = c.Arg("target").String()
if err := validateNoFlagArgs([]string{opts.Target}); err != nil {
    return err
}
snapshot := *opts
return handler("uninstall", &snapshot)
```

- [x] **步骤 3：迁移 `config` 命令**

修改签名：

```go
func newConfigCmd(handler CommandHandler) (*gcli.Command, func())
```

绑定位置参数：

```text
action optional
key optional
value optional
```

保持以下命令行为：

```bash
eget config init
eget config list
eget config ls
eget config get global.cache_dir
eget config set global.cache_dir ~/.cache/eget
```

- [x] **步骤 4：迁移 `query` 命令**

修改签名：

```go
func newQueryCmd(handler CommandHandler) (*gcli.Command, func())
```

绑定：

```text
--action / -a default latest
--tag / -t
--limit / -l default 10
--json / -j
--prerelease / -p
target required
```

保留别名：

```text
q
```

- [x] **步骤 5：运行定向测试**

运行：

```bash
go test ./internal/cli -run "TestMain_.*List|TestMain_.*Query|TestMain_.*Config|TestMain_.*Uninstall"
```

预期：

```text
ok   github.com/inherelab/eget/internal/cli
```

如果正则没有覆盖到相关现有测试，运行完整 CLI 包测试：

```bash
go test ./internal/cli
```

- [x] **步骤 6：提交简单命令迁移**

运行：

```bash
git add internal/cli/list_cmd.go internal/cli/uninstall_cmd.go internal/cli/config_cmd.go internal/cli/query_cmd.go internal/cli/app_test.go
git commit -m "refactor(cli): migrate simple commands to gcli"
```

预期：

```text
提交 list/uninstall/config/query 迁移
```

## 任务 4：迁移 search 命令

**文件：**

- 修改：`internal/cli/search_cmd.go`
- 修改：`internal/cli/search_cmd_test.go`
- 测试：`internal/cli/app_test.go`

- [x] **步骤 1：迁移命令构造**

修改签名：

```go
func newSearchCmd(handler CommandHandler) (*gcli.Command, func())
```

绑定：

```text
--sort
--order
--limit / -l default 10
--json / -j
keyword required
extras optional multi
```

在 `Func` 中保留当前行为：

```go
opts.Keyword = c.Arg("keyword").String()
opts.Extras = c.Arg("extras").Strings()
snapshot := *opts
snapshot.Extras = append([]string(nil), opts.Extras...)
return handler("search", &snapshot)
```

如果 `gcli` 把剩余参数暴露为另一套 API，就用该 API 保持 `search keyword language:go stars:>10` 的行为。

- [x] **步骤 2：保持状态 reset**

reset 函数必须恢复：

```go
*opts = SearchOptions{Limit: 10}
```

这用于保证 `TestApp_RunWithArgsDoesNotLeakSearchStateAcrossRuns` 继续通过。

- [x] **步骤 3：运行 search 测试**

运行：

```bash
go test ./internal/cli -run "Search|RunWithArgsDoesNotLeakSearchState"
```

预期：

```text
ok   github.com/inherelab/eget/internal/cli
```

- [x] **步骤 4：提交 search 迁移**

运行：

```bash
git add internal/cli/search_cmd.go internal/cli/search_cmd_test.go internal/cli/app_test.go
git commit -m "refactor(cli): migrate search command to gcli"
```

预期：

```text
提交 search 命令迁移
```

## 任务 5：迁移 install-like 命令

**文件：**

- 修改：`internal/cli/install_cmd.go`
- 修改：`internal/cli/download_cmd.go`
- 修改：`internal/cli/add_cmd.go`
- 测试：`internal/cli/app_test.go`

- [x] **步骤 1：迁移 `download` 命令**

修改签名：

```go
func newDownloadCmd(handler CommandHandler) (*gcli.Command, func())
```

绑定：

```text
--tag
--system
--to
--file
--asset / -a
--source
--extract-all / --ea
--quiet
--fallback-versions
--chunk default -1
target required
```

reset：

```go
*opts = DownloadOptions{ChunkConcurrency: -1}
```

在 `Func` 中对 target 位置参数调用 `validateNoFlagArgs`。

- [x] **步骤 2：迁移 `add` 命令**

修改签名：

```go
func newAddCmd(handler CommandHandler) (*gcli.Command, func())
```

绑定：

```text
--name
--tag
--system
--to
--file
--asset
--source
--extract-all / --ea
--gui
--quiet
--chunk default -1
target required
```

reset：

```go
*opts = AddOptions{ChunkConcurrency: -1}
```

- [x] **步骤 3：迁移 `install` 命令**

修改签名：

```go
func newInstallCmd(handler CommandHandler) (*gcli.Command, func())
```

绑定：

```text
--tag
--system
--to
--file
--asset / -a
--name
--source
--extract-all / --ea
--all
--gui
--quiet
--add
--fallback-versions
--chunk default -1
--batch default -1
target optional multi
```

在 `Func` 中保留 target 拆分：

```go
targetArgs := c.Arg("target").Strings()
if err := validateNoFlagArgs(targetArgs); err != nil {
    return err
}
opts.Targets = splitTargets(targetArgs)
snapshot := *opts
snapshot.Targets = append([]string(nil), opts.Targets...)
return handler("install", &snapshot)
```

reset：

```go
*opts = InstallOptions{ChunkConcurrency: -1, BatchConcurrency: -1}
```

- [x] **步骤 4：运行 install-like 测试**

运行：

```bash
go test ./internal/cli -run "Install|Download|Add|ExtractAll|GUI|Chunk|Batch|Trailing"
```

预期：

```text
ok   github.com/inherelab/eget/internal/cli
```

- [x] **步骤 5：提交 install-like 迁移**

运行：

```bash
git add internal/cli/install_cmd.go internal/cli/download_cmd.go internal/cli/add_cmd.go internal/cli/app_test.go
git commit -m "refactor(cli): migrate install commands to gcli"
```

预期：

```text
提交 install/download/add 迁移
```

## 任务 6：迁移 update 命令

**文件：**

- 修改：`internal/cli/update_cmd.go`
- 测试：`internal/cli/app_test.go`

- [x] **步骤 1：迁移命令构造**

修改签名：

```go
func newUpdateCmd(handler CommandHandler) (*gcli.Command, func())
```

绑定：

```text
--all / -A
--check
--dry-run
--interactive
--tag
--system
--to
--file
--asset / -a
--source
--quiet
--chunk default -1
--batch default -1
target optional multi
```

保留当前 `update_cmd.go` 中已有别名。

- [x] **步骤 2：保留 target 拆分和 reset**

在 `Func` 中：

```go
targetArgs := c.Arg("target").Strings()
if err := validateNoFlagArgs(targetArgs); err != nil {
    return err
}
opts.Targets = splitTargets(targetArgs)
snapshot := *opts
snapshot.Targets = append([]string(nil), opts.Targets...)
return handler("update", &snapshot)
```

reset：

```go
*opts = UpdateOptions{ChunkConcurrency: -1, BatchConcurrency: -1}
```

- [x] **步骤 3：运行 update 测试**

运行：

```bash
go test ./internal/cli -run "Update|RunWithArgsDoesNotLeakCommandState"
```

预期：

```text
ok   github.com/inherelab/eget/internal/cli
```

- [x] **步骤 4：提交 update 迁移**

运行：

```bash
git add internal/cli/update_cmd.go internal/cli/app_test.go
git commit -m "refactor(cli): migrate update command to gcli"
```

预期：

```text
提交 update 命令迁移
```

## 任务 7：调整测试以适配 gcli 输出

**文件：**

- 修改：`internal/cli/app_test.go`
- 修改：`internal/cli/search_cmd_test.go`

- [x] **步骤 1：放宽 help 输出断言**

如果测试依赖精确 help 文案，或只接受 `Usage:`，改为：

```go
help := stdout.String()
if !strings.Contains(help, "Usage") && !strings.Contains(help, "Commands") {
    t.Fatalf("expected help output to contain usage or commands, got %q", help)
}
```

继续保留：

```text
无子命令时 stdout 非空
无子命令时 stderr 为空
```

- [x] **步骤 2：新增或更新 gcli 内置 version 测试**

在 `internal/cli/app_test.go` 中新增或更新：

```go
func TestMain_VersionUsesBuildInfo(t *testing.T) {
    SetBuildInfo("v1.2.3", "abc123", "2026-05-16T10:11:12+08:00")
    var stdout bytes.Buffer
    var stderr bytes.Buffer

    err := newApp(func(string, any) error {
        t.Fatalf("handler should not run for version")
        return nil
    }, &stdout, &stderr).RunWithArgs([]string{"--version"})

    assert.NoErr(t, err)
    out := stdout.String() + stderr.String()
    assert.Contains(t, out, "v1.2.3")
}
```

如果 `gcli` 把 version 写到 stderr，该测试允许 stdout + stderr 组合检查。不要要求精确格式。

- [x] **步骤 3：运行全部 CLI 测试**

运行：

```bash
go test ./internal/cli
```

预期：

```text
ok   github.com/inherelab/eget/internal/cli
```

- [x] **步骤 4：提交测试调整**

运行：

```bash
git add internal/cli/app_test.go internal/cli/search_cmd_test.go
git commit -m "test(cli): normalize assertions for gcli"
```

预期：

```text
提交测试断言调整
```

## 任务 8：移除 `cflag/capp`

**文件：**

- 修改：`go.mod`
- 修改：`go.sum`
- 修改：任何仍 import `capp` 的 `internal/cli/*.go`

- [x] **步骤 1：查找残留 import**

运行：

```bash
rg "cflag|capp" internal cmd
```

清理前预期：

```text
如果还有输出，只应是未清理的迁移残留
```

删除所有 `github.com/gookit/goutil/cflag/capp` import 和 `capp.` 类型引用。

- [x] **步骤 2：整理模块依赖**

运行：

```bash
go mod tidy
```

预期：

```text
如果其他包仍使用 github.com/gookit/goutil，则 go.mod 保留该依赖
go.mod 不再因为 cflag/capp 保留额外依赖
```

- [x] **步骤 3：确认源码中没有 cflag/capp**

运行：

```bash
rg "cflag|capp" internal cmd
```

预期：

```text
无输出
```

- [x] **步骤 4：运行全量测试**

运行：

```bash
go test ./...
```

预期：

```text
所有 package 通过
```

- [x] **步骤 5：提交清理**

运行：

```bash
git add go.mod go.sum internal/cli
git commit -m "chore(cli): remove cflag usage"
```

预期：

```text
提交 cflag/capp 清理
```

## 任务 9：最终验证和文档核对

**文件：**

- 仅必要时修改：`docs/superpowers/specs/2026-05-14-sdk-download-design.md`
- 仅必要时修改：`docs/superpowers/specs/2026-05-15-gcli-migration-design.md`

- [x] **步骤 1：手动运行命令矩阵**

运行：

```bash
go run ./cmd/eget --version
go run ./cmd/eget list --all
go run ./cmd/eget query --action latest owner/repo
go run ./cmd/eget search --limit 2 ripgrep language:go
```

预期：

```text
--version 输出版本信息
命令能完成 CLI 解析并进入 app/service 层
依赖网络的命令允许远程查询失败，但不能因为 CLI 解析失败
```

对于依赖网络的命令，provider/network 错误可以接受；flag 或参数解析回归不可接受。

- [x] **步骤 2：确认 SDK 设计仍引用 gcli 前置条件**

运行：

```bash
rg -n "gcli|cflag|sdk install" docs/superpowers/specs/2026-05-14-sdk-download-design.md
```

预期：

```text
SDK 设计文档说明 gcli 迁移是前置条件，并禁止在 cflag 下硬编码解析嵌套命令
```

- [x] **步骤 3：运行最终测试**

运行：

```bash
go test ./...
```

预期：

```text
所有 package 通过
```

- [x] **步骤 4：如文档有变更则提交**

如果步骤 2 需要修改文档，运行：

```bash
git add docs/superpowers/specs/2026-05-14-sdk-download-design.md docs/superpowers/specs/2026-05-15-gcli-migration-design.md
git commit -m "docs(cli): align gcli migration notes"
```

文档有变更时预期：

```text
提交文档对齐变更
```

文档无变更时预期：

```text
无需提交
```

## 最终成功标准

- `go test ./internal/cli` 通过。
- `go test ./...` 通过。
- `rg "cflag|capp" internal cmd` 无输出。
- `internal/cli.Main(args, stdout, stderr)` 仍可用于测试。
- 现有命令别名仍能解析。
- 现有 option structs 仍传递给 `CommandHandler`。
- `--version` / `-V` 使用 `gcli` 内置版本处理。
- 本迁移不引入 `sdk` 命令或 SDK 下载代码。

