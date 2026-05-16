# Config Subcommands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `eget config <action>` 从 action 位置参数改为真正的 `gcli` 子命令，同时保持现有业务层 `ConfigOptions` 和 `handleConfig` 不变。

**Architecture:** `config` 顶层命令只负责承载子命令和展示帮助，不再直接绑定 `action/key/value` 位置参数。`init/list/get/set` 子命令在 CLI 层填充现有 `ConfigOptions`，继续调用 `handler("config", &snapshot)`，避免改动 `internal/cli/handlers.go` 和 app/config 业务服务。`RunWithArgs` 的 `validateKnownFlags` 需要理解 `config` 子命令参数边界，确保 `config get global.target`、`config set key value` 不被预校验误判。

**Tech Stack:** Go 1.25、`github.com/gookit/gcli/v3`、现有 `internal/cli` 测试套件、`github.com/gookit/goutil/testutil/assert`。

---

## Scope

本计划只迁移 `config` 命令的 CLI 解析结构，不新增配置功能，不修改配置文件格式，不修改 SDK 设计，不实现 SDK 命令。

保持兼容的命令：

```text
eget config init
eget config list
eget config ls
eget config get global.target
eget config set global.target ~/.local/bin
eget cfg list
```

新的子命令行为：

```text
eget config
```

应展示 `config` 命令帮助，不调用 `CommandHandler`。这符合多层级命令模型，也能作为后续 `sdk install/list/remove/index` 的实现参考。

## File Structure

需要修改：

```text
internal/cli/config_cmd.go
internal/cli/app.go
internal/cli/app_test.go
```

不修改：

```text
internal/cli/handlers.go
internal/app/*
internal/config/*
```

理由：

- `config_cmd.go` 负责 CLI 命令结构，应在这里注册 `config` 子命令。
- `app.go` 目前有 `validateKnownFlags` 预校验，必须让它支持 `config` 子命令参数。
- `app_test.go` 已有 config 路由测试，新增和调整测试放在同一文件，保持 CLI 行为集中验证。
- `handlers.go` 的 `handleConfig(*ConfigOptions)` 已能处理 `init/list/ls/get/set`，本次只改变 options 的来源。

## Expected Command Mapping

| CLI 输入 | Handler name | ConfigOptions |
| --- | --- | --- |
| `config init` | `config` | `Action: "init"` |
| `config list` | `config` | `Action: "list"` |
| `config ls` | `config` | `Action: "list"` |
| `config get global.target` | `config` | `Action: "get", Key: "global.target"` |
| `config set global.target ~/.local/bin` | `config` | `Action: "set", Key: "global.target", Value: "~/.local/bin"` |
| `cfg list` | `config` | `Action: "list"` |

## Task 1: Add Failing Tests For Config Subcommands

**Files:**

- Modify: `internal/cli/app_test.go`

- [ ] **Step 1: Replace the old config action test with subcommand route tests**

In `internal/cli/app_test.go`, replace `TestMain_ConfigActionRoutesToConfigCommand` with:

```go
func TestMain_ConfigSubcommandsRouteToConfigCommand(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      ConfigOptions
		wantCalls int
	}{
		{
			name:      "init",
			args:      []string{"config", "init"},
			want:      ConfigOptions{Action: "init"},
			wantCalls: 1,
		},
		{
			name:      "list",
			args:      []string{"config", "list"},
			want:      ConfigOptions{Action: "list"},
			wantCalls: 1,
		},
		{
			name:      "list alias",
			args:      []string{"config", "ls"},
			want:      ConfigOptions{Action: "list"},
			wantCalls: 1,
		},
		{
			name:      "get",
			args:      []string{"config", "get", "global.target"},
			want:      ConfigOptions{Action: "get", Key: "global.target"},
			wantCalls: 1,
		},
		{
			name:      "set",
			args:      []string{"config", "set", "global.target", "~/.local/bin"},
			want:      ConfigOptions{Action: "set", Key: "global.target", Value: "~/.local/bin"},
			wantCalls: 1,
		},
		{
			name:      "top alias",
			args:      []string{"cfg", "list"},
			want:      ConfigOptions{Action: "list"},
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			assert.NoErr(t, err)
			assert.Eq(t, tt.wantCalls, len(calls))
			assert.Eq(t, "config", calls[0].name)

			opts, ok := calls[0].options.(*ConfigOptions)
			assert.True(t, ok)
			assert.Eq(t, tt.want.Action, opts.Action)
			assert.Eq(t, tt.want.Key, opts.Key)
			assert.Eq(t, tt.want.Value, opts.Value)
		})
	}
}
```

- [ ] **Step 2: Add a help behavior test for bare config**

In `internal/cli/app_test.go`, add this test near the config tests:

```go
func TestMain_ConfigWithoutSubcommandShowsHelp(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"config"})
	assert.NoErr(t, err)
	assert.Eq(t, 0, len(calls))

	help := stdout.String()
	if !strings.Contains(help, "Available Commands") && !strings.Contains(help, "Usage") {
		t.Fatalf("expected config help output, got %q", help)
	}
	if !strings.Contains(help, "init") || !strings.Contains(help, "get") || !strings.Contains(help, "set") {
		t.Fatalf("expected config subcommands in help output, got %q", help)
	}
}
```

- [ ] **Step 3: Run config tests and verify RED**

Run:

```bash
go test ./internal/cli -run "Config"
```

Expected:

```text
FAIL
```

Expected failure reason:

```text
config ls does not map to Action "list"
```

or:

```text
config without subcommand still calls handler
```

If the test fails because of a typo, missing import, or compile error unrelated to the intended behavior, fix the test and rerun until it fails for the expected behavior gap.

## Task 2: Implement Config Subcommands

**Files:**

- Modify: `internal/cli/config_cmd.go`

- [ ] **Step 1: Replace action argument binding with subcommand registration**

Rewrite `newConfigCmd` in `internal/cli/config_cmd.go` to this structure:

```go
func newConfigCmd(handler CommandHandler) (*gcli.Command, func()) {
	opts := &ConfigOptions{}
	cmd := gcli.NewCommand("config", "Manage configuration")
	cmd.Aliases = []string{"cfg"}
	cmd.Help = `<info>Config actions</>:
  init                Initialize the config file with default values
  list | ls           Print current config values and file status
  get KEY             Print one config value
  set KEY VALUE       Update one config value

<info>Examples</>:
  eget config init
  eget config list
  eget config get global.target
  eget config set global.target ~/.local/bin`

	cmd.Subs = []*gcli.Command{
		newConfigActionCmd("init", nil, opts, handler),
		newConfigActionCmd("list", []string{"ls"}, opts, handler),
		newConfigGetCmd(opts, handler),
		newConfigSetCmd(opts, handler),
	}
	return cmd, func() {
		*opts = ConfigOptions{}
	}
}
```

- [ ] **Step 2: Add a shared helper for no-argument config actions**

Add this helper below `newConfigCmd` in `internal/cli/config_cmd.go`:

```go
func newConfigActionCmd(action string, aliases []string, opts *ConfigOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand(action, "Run config "+action)
	cmd.Aliases = aliases
	cmd.Func = func(_ *gcli.Command, args []string) error {
		if err := validateNoFlagArgs(args); err != nil {
			return err
		}
		opts.Action = action
		snapshot := *opts
		return handler("config", &snapshot)
	}
	return cmd
}
```

Note: for `list`, this helper sets `Action: "list"` even when invoked as `config ls`, because `ls` is an alias of the `list` subcommand.

- [ ] **Step 3: Add the get subcommand helper**

Add this helper below `newConfigActionCmd`:

```go
func newConfigGetCmd(opts *ConfigOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("get", "Print one config value")
	cmd.Config = func(c *gcli.Command) {
		c.AddArg("key", "Config key", true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Action = "get"
		opts.Key = c.Arg("key").String()
		if err := validateNoFlagArgs(append([]string{opts.Key}, args...)); err != nil {
			return err
		}
		snapshot := *opts
		return handler("config", &snapshot)
	}
	return cmd
}
```

- [ ] **Step 4: Add the set subcommand helper**

Add this helper below `newConfigGetCmd`:

```go
func newConfigSetCmd(opts *ConfigOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("set", "Update one config value")
	cmd.Config = func(c *gcli.Command) {
		c.AddArg("key", "Config key", true)
		c.AddArg("value", "Config value", true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Action = "set"
		opts.Key = c.Arg("key").String()
		opts.Value = c.Arg("value").String()
		if err := validateNoFlagArgs(append([]string{opts.Key, opts.Value}, args...)); err != nil {
			return err
		}
		snapshot := *opts
		return handler("config", &snapshot)
	}
	return cmd
}
```

- [ ] **Step 5: Run config tests and verify GREEN for subcommands**

Run:

```bash
go test ./internal/cli -run "Config"
```

Expected:

```text
ok   github.com/inherelab/eget/internal/cli
```

If `TestMain_ConfigWithoutSubcommandShowsHelp` fails because `gcli` writes command help to stderr instead of stdout, update the test to use `stdout.String() + stderr.String()` and keep the assertion that the handler is not called.

## Task 3: Update Flag Prevalidation For Config Subcommands

**Files:**

- Modify: `internal/cli/app.go`
- Test: `internal/cli/app_test.go`

- [ ] **Step 1: Add a failing test for unknown config subcommand flags**

Add this test near other config tests in `internal/cli/app_test.go`:

```go
func TestMain_ConfigSubcommandRejectsUnknownFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(func(string, any) error { return nil }, &stdout, &stderr).
		RunWithArgs([]string{"config", "get", "--bad", "global.target"})
	if err == nil {
		t.Fatal("expected config get --bad to be rejected")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Fatalf("expected error to mention bad flag, got %v", err)
	}
}
```

- [ ] **Step 2: Run the new test and verify RED if current prevalidation misses it**

Run:

```bash
go test ./internal/cli -run "ConfigSubcommandRejectsUnknownFlag"
```

Expected:

```text
FAIL
```

Expected failure reason:

```text
err == nil
```

If it already fails with an error mentioning `bad`, keep the test and continue to Step 3; the production change may only need cleanup of config flag specs.

- [ ] **Step 3: Add config subcommand flag specs**

In `internal/cli/app.go`, replace the current empty config entry:

```go
"config":    {},
```

with:

```go
"config": {
	subs: map[string]flagSpec{
		"init": {},
		"list": {},
		"ls":   {},
		"get":  {},
		"set":  {},
	},
},
```

This requires extending `flagSpec` from:

```go
type flagSpec struct {
	bools  map[string]bool
	values map[string]bool
}
```

to:

```go
type flagSpec struct {
	bools  map[string]bool
	values map[string]bool
	subs   map[string]flagSpec
}
```

- [ ] **Step 4: Teach validateKnownFlags to descend into subcommands**

In `internal/cli/app.go`, update `validateKnownFlags` so after it resolves the top-level command spec, it consumes one subcommand token when the current spec has `subs`.

Use this exact shape:

```go
func validateKnownFlags(args []string) error {
	cmdName, start := findCommandArg(args)
	if cmdName == "" {
		return nil
	}
	spec, ok := commandFlagSpecs[cmdName]
	if !ok {
		return nil
	}
	if len(spec.subs) > 0 && start < len(args) {
		subName := args[start]
		if !strings.HasPrefix(subName, "-") {
			if subSpec, ok := spec.subs[subName]; ok {
				spec = subSpec
				start++
			}
		}
	}
	for i := start; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return nil
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return nil
		}
		name := strings.TrimLeft(arg, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		if spec.bools[name] {
			continue
		}
		if spec.values[name] {
			if !strings.Contains(arg, "=") {
				i++
			}
			continue
		}
		return fmt.Errorf("option provided but not defined: %s", arg)
	}
	return nil
}
```

This keeps the existing trailing-flag rule: once a non-flag positional argument appears after the command/subcommand, prevalidation stops and command-specific `validateNoFlagArgs` catches trailing flags.

- [ ] **Step 5: Run config prevalidation tests and verify GREEN**

Run:

```bash
go test ./internal/cli -run "Config"
```

Expected:

```text
ok   github.com/inherelab/eget/internal/cli
```

## Task 4: Verify Full CLI Behavior And Commit

**Files:**

- Modify: `internal/cli/config_cmd.go`
- Modify: `internal/cli/app.go`
- Modify: `internal/cli/app_test.go`

- [ ] **Step 1: Run focused CLI tests**

Run:

```bash
go test ./internal/cli
```

Expected:

```text
ok   github.com/inherelab/eget/internal/cli
```

- [ ] **Step 2: Run full test suite**

Run:

```bash
go test ./...
```

Expected:

```text
所有 package 通过
```

- [ ] **Step 3: Run manual config parse checks**

Run:

```bash
go run ./cmd/eget config --help
go run ./cmd/eget config list
go run ./cmd/eget cfg list
go run ./cmd/eget config get global.target
```

Expected:

```text
config --help 显示 init/list/get/set 子命令
config list 和 cfg list 能进入业务层
config get global.target 能进入业务层
```

If a command fails because local config does not exist or a config key is missing, that is acceptable only if the failure happens after CLI parsing and the error does not mention command/flag parsing.

- [ ] **Step 4: Commit implementation**

Run:

```bash
git add internal/cli/config_cmd.go internal/cli/app.go internal/cli/app_test.go
git commit -m "refactor(cli): make config actions subcommands"
```

Expected:

```text
提交只包含 config 子命令迁移和相关测试
```

## Task 5: Update This Plan After Implementation

**Files:**

- Modify: `docs/superpowers/plans/2026-05-16-config-subcommands.md`

- [ ] **Step 1: Mark completed checkboxes**

After Task 1 through Task 4 pass, update every completed checkbox in this plan from:

```markdown
- [ ] **Step ...**
```

to:

```markdown
- [x] **Step ...**
```

- [ ] **Step 2: Commit plan progress**

Run:

```bash
git add docs/superpowers/plans/2026-05-16-config-subcommands.md
git commit -m "docs(cli): complete config subcommand plan"
```

Expected:

```text
计划文件记录最终执行状态
```

## Final Success Criteria

- `config` 命令不再通过 `action` 位置参数实现。
- `config` 注册 `init/list/get/set` 真实子命令。
- `config ls` 是 `config list` 的子命令别名。
- `cfg list` 仍可用。
- `ConfigOptions` 和 `handleConfig` 不需要改动。
- `eget config` 展示帮助，不调用 handler。
- `go test ./internal/cli` 通过。
- `go test ./...` 通过。
- 本次不引入 SDK 命令或 SDK 下载代码。

## Self-Review Notes

- Spec coverage: 当前用户目标是“尝试将 config 命令的 action 改为子命令实现”，计划覆盖测试、实现、预校验、手动验证和提交。
- Placeholder scan: 本计划没有 `TBD`、`TODO`、未展开的“类似上面”等占位步骤。
- Type consistency: 所有代码片段继续使用现有 `ConfigOptions`、`CommandHandler`、`newApp`、`validateKnownFlags`、`validateNoFlagArgs`、`commandCall` 和 `assert`。
