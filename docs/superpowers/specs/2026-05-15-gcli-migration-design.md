# gcli 迁移设计

## 目标

将 `eget` CLI 从 `github.com/gookit/goutil/cflag/capp` 迁移到 `github.com/gookit/gcli/v3`，为后续 `eget sdk install/list/remove/index` 这类多层级命令提供可靠基础。

首版目标：

- 保持现有命令、参数、别名和错误语义尽量等价。
- 保持 `internal/cli.Main(args, stdout, stderr)` 对测试和入口的调用方式稳定。
- 保持 `CommandHandler func(name string, options any) error` 的 app 层分发边界稳定。
- 迁移现有一级命令到 `gcli.Command`。
- 在迁移完成后移除 `cflag/capp` 依赖。
- 不通过手写解析 `sdk install`、`sdk list` 等字符串模拟多层级命令。

## 非目标

本迁移不实现：

- `sdk` 命令。
- SDK 下载、索引、断点续传或安装记录。
- 现有业务 service 行为调整。
- 输出文案大规模重写。
- 命令语义重命名或参数重命名。

## 背景

当前 CLI 使用 `cflag/capp`，所有命令在 `internal/cli/*_cmd.go` 中通过 `capp.NewCmd` 构造：

```text
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
```

`cflag/capp` 对当前一级命令足够，但不适合后续 `sdk install/list/remove/index` 这种真正多层级命令。继续在 `cflag/capp` 下做 SDK 会引入临时字符串分发和手写参数解析，后续维护成本高。

`kite-go` 项目中的 `kite xenv` 已使用 `gookit/gcli/v3` 实现父子命令，形态和本项目后续需求匹配：

```go
var ToolsCmd = &gcli.Command{
    Name: "tools",
    Subs: []*gcli.Command{
        ToolsInstallCmd(),
        ToolsListCmd(),
        ToolsIndexCmd(),
    },
}
```

## 迁移原则

- 先迁移框架，再新增能力。
- 每个命令保持独立文件，沿用现有 `InstallOptions`、`DownloadOptions` 等 option struct。
- 命令构造函数从 `newInstallCmd(handler) (*capp.Cmd, func())` 改为 `newInstallCmd(handler) (*gcli.Command, func())` 或等价封装。
- 保留 resetter 机制，避免多次 `RunWithArgs` 时命令状态泄漏。
- 继续把 CLI 解析和业务执行分开：CLI 只绑定参数和构造 options，业务仍交给 `cliService.handle`。
- 不在迁移过程中重构 app/service/install/client 层。
- 测试先对齐现有行为，再考虑利用 `gcli` 的新能力。

## App 结构设计

当前：

```go
type App struct {
    inner     *capp.App
    resetters []func()
    verbose   *bool
}
```

迁移后建议：

```go
type App struct {
    inner     *gcli.App
    resetters []func()
    verbose   *bool
}
```

保留：

```go
func Main(args []string, stdout, stderr io.Writer) error
func newApp(handler CommandHandler, stdout, stderr io.Writer) *App
func (a *App) RunWithArgs(args []string) error
func (a *App) Verbose() bool
```

`Main` 的 lazy service 初始化保持不变：

```text
Main -> newApp(handler) -> RunWithArgs -> handler -> cliService.handle
```

## 全局选项

现有全局选项：

```text
--verbose, -v
--version, -V
```

迁移要求：

- `--verbose` / `-v` 继续设置 `cliApp.Verbose()`，并影响 `configureVerbose`。
- `gcli` 已默认绑定 `--version` / `-V`，迁移时不再手动注册版本 flag，只需要设置好 app 的版本信息。
- `SetBuildInfo(version, gitHash, buildTime)` 仍负责归一化 build time，并把版本、commit、build date 写入 `gcli` app 可展示的版本信息。
- 无子命令时继续输出 help 到 stdout，不返回错误。
- help 输出格式可以有少量框架差异，但测试不应依赖完整 help 文本；只断言包含核心信息，例如 `Usage` 或 `Commands`。

## 命令映射

| 当前命令 | 别名 | 迁移目标 |
| --- | --- | --- |
| `install` | `i`, `ins` | `gcli.Command{Name:"install", Aliases:[]string{"i","ins"}}` |
| `download` | `dl` | `gcli.Command{Name:"download", Aliases:[]string{"dl"}}` |
| `add` | 无 | `gcli.Command{Name:"add"}` |
| `uninstall` | `remove`, `rm`, `delete` | 保持现有别名 |
| `list` | `ls` | 保持现有别名 |
| `update` | `up` | 保持现有别名 |
| `config` | `cfg` 如已有则保持 | 保持现有行为 |
| `query` | `q` | 保持现有别名 |
| `search` | 无 | 保持现有行为 |

具体别名以当前命令文件为准，迁移时不得无意新增或删除别名。

## 参数绑定策略

`gcli` 支持 `StringOpt`、`BoolOpt`、`IntOpt`、`AddArg`、`MustFromStruct` 等方式。迁移建议优先使用显式绑定，减少 tag 迁移风险：

```go
Config: func(c *gcli.Command) {
    c.StrOpt(&opts.Tag, "tag", "", "", "Release tag")
    c.StrOpt(&opts.System, "system", "", "", "Target system")
    c.AddArg("target", "Installation target(s)", false, true)
}
```

如果实际 `gcli` API 签名和示例不同，以本项目使用的版本文档和编译结果为准。设计目标是显式绑定，不强制具体函数名。

多值参数：

- `install` 的 `target` 继续允许多个参数。
- `update` 的 `target` 继续允许多个参数。
- `search` 的 extra search conditions 继续允许多个参数。
- `query`、`download`、`add`、`uninstall` 的必填/选填参数保持现状。

## Flag 顺序和 trailing flag 校验

现版本基于 gcli v3.8，允许 trailing flags：

```bash
eget install owner/repo --tag v1
```

它与下面写法等价：

```bash
eget install --tag v1 owner/repo
```

实现建议：

- 保留未知/已移除 flag 的预校验。
- 不再因为 flag 位于 positional argument 之后报错。
- `validateNoFlagArgs(args []string) error` 仅作为命令回调里的剩余参数兜底。

## 状态重置

当前命令构造时捕获 `opts` 指针，并通过 resetter 避免多次运行泄漏状态：

```go
return cmd, func() {
    *opts = InstallOptions{ChunkConcurrency: -1, BatchConcurrency: -1}
}
```

迁移后仍保留 resetter。重点命令：

- `install`: `ChunkConcurrency=-1`, `BatchConcurrency=-1`
- `download`: `ChunkConcurrency=-1`
- `add`: `ChunkConcurrency=-1`
- `update`: `ChunkConcurrency=-1`, `BatchConcurrency=-1`
- `search`: `Limit` 默认值、`Extras` 清空
- 其他命令按当前默认值重置

测试必须覆盖：

- 连续两次 `RunWithArgs` 不泄漏 `install --tag`。
- 连续两次 `RunWithArgs` 不泄漏 `update` targets。
- 连续两次 `RunWithArgs` 不泄漏 `search` extras、limit、json。

## 文件调整计划

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

可选新增：

```text
internal/cli/gcli_helpers.go
```

`gcli_helpers.go` 可承载：

- `addStringOpt`
- `addBoolOpt`
- `addIntOpt`
- `argStrings`
- `validateNoTrailingFlagsFromArgs`

只有在重复代码明显时再新增 helper，不为一次性绑定过早抽象。

## 迁移步骤

### 阶段 1：引入 gcli 依赖

- 在 `go.mod` 加入 `github.com/gookit/gcli/v3`。
- 先不删除 `goutil/cflag/capp`，保持可以分命令迁移。
- 跑 `go mod tidy`。

验证：

```bash
go test ./internal/cli
```

### 阶段 2：迁移 App 壳

- `internal/cli/app.go` 从 `capp.App` 改为 `gcli.App`。
- 保留 `Main`、`newApp`、`RunWithArgs`、`Verbose` 对外形态。
- 实现全局 `--verbose`；`--version` / `-V` 使用 `gcli` 内置版本选项。
- 设置 `gcli` app 的版本、commit 和 build date 展示信息。
- 保持无子命令输出 help 且不返回错误。

验证：

```bash
go test ./internal/cli -run TestMain_NoSubcommandReturnsErrorAndHelp
go test ./internal/cli -run TestSetBuildInfoCompactsBuildTime
```

### 阶段 3：逐个迁移现有命令

建议顺序：

1. `list`：参数少，先验证简单命令路径。
2. `query`：有多种 option，验证 string/int/bool。
3. `search`：验证多 positional args 和 state reset。
4. `download`：验证必填 target、alias、download options。
5. `install`：验证多 target、逗号拆分、`--all`、`--batch`。
6. `update`：验证多 target、`--check`、`--all`、`--batch`。
7. `add`：验证 install-like options。
8. `uninstall`：验证单 target。
9. `config`：验证 action/key/value 可选参数。

每迁移一组命令，跑对应测试，避免一次性重写全部 CLI 后难以定位差异。

### 阶段 4：移除 cflag/capp

- 所有命令迁移后删除 `github.com/gookit/goutil/cflag/capp` import。
- 如果 `gookit/goutil` 仍被其他包使用，保留模块依赖。
- 确认 `rg "cflag|capp"` 只剩历史文档或没有结果。

验证：

```bash
rg "cflag|capp" internal cmd
go test ./internal/cli
go test ./...
```

### 阶段 5：文档更新

如果用户文档没有提 CLI 框架，则无需更新 README。需要更新：

```text
docs/superpowers/specs/2026-05-14-sdk-download-design.md
```

确保 SDK 设计文档仍说明：

- SDK 命令依赖 `gcli` 多层级命令能力。
- 不在 `cflag/capp` 下硬编码 `sdk install/list/...`。

## 测试策略

优先保持现有 `internal/cli/app_test.go` 覆盖面。

必须覆盖：

- 无子命令输出 help，不返回错误。
- `gcli` 内置 `--version` / `-V` 输出已设置的版本信息并停止执行。
- `--verbose` / `-v` 设置 verbose。
- 每个命令能 route 到正确 handler name。
- 每个 options struct 字段绑定正确。
- aliases 保持可用。
- trailing flag 可以正常绑定到对应 options。
- `install`、`update` 多 target 和逗号拆分保持不变。
- `install --name` 不能和多个 target 联用的业务校验仍由 `handle` 保持。
- `download --gui` 仍失败，因为 download 没有该 flag。
- 多次 `RunWithArgs` 不泄漏状态。

如果 `gcli` help 文案和 `capp` 不同，测试应放宽：

- 不比较完整 help。
- 只检查 help 非空和包含核心关键词。

## 风险与处理

### Help 文案差异

框架迁移后 help 格式几乎一定变化。处理方式：

- 用户文档只保留命令语义，不依赖具体 help 格式。
- 测试不做完整字符串快照。

### Flag API 差异

`gcli` 的 option API 和 `capp` 不同。处理方式：

- 每个命令用显式绑定，迁移时编译驱动修正。
- 不同时引入 struct tag 绑定和显式绑定，避免行为不透明。

### Trailing flag 行为差异

`gcli v3.8` 支持命令级参数重排。处理方式：

- 允许 trailing flags。
- 保留本项目自己的未知/已移除 flag 校验。

### 多次运行状态泄漏

`gcli.Command` 同样可能持有 option 指针。处理方式：

- 保留 resetter。
- 每次 `RunWithArgs` 前重置 options 和 verbose。

### stdout/stderr 路由差异

当前 app 将 help 写到 stdout，命令输出和错误多写 stderr 或由 service 控制。处理方式：

- `newApp` 明确设置 gcli 的 output/error writer。
- 测试覆盖无子命令、version、help、错误输出。

## 与 SDK 功能的关系

CLI 迁移是 SDK 命令的前置任务，但不是 SDK 功能的一部分。

迁移完成后，SDK 命令可以自然注册为：

```go
var sdkCmd = &gcli.Command{
    Name: "sdk",
    Subs: []*gcli.Command{
        sdkInstallCmd(),
        sdkListCmd(),
        sdkRemoveCmd(),
        sdkIndexCmd(),
    },
}
```

这比在一级 handler 中硬解析 `sdk install` 更清晰，也能让 `sdk index refresh/clear/show` 继续扩展为更深层级命令。

## 成功标准

- `go test ./internal/cli` 通过。
- `go test ./...` 通过。
- `rg "cflag|capp" internal cmd` 没有结果。
- 当前 README 中已有命令示例仍可运行。
- SDK 设计文档的 CLI 前置要求仍成立。
- 未引入任何 SDK 下载实现代码。
