# SDK Path 命令实现计划

> **给 agentic workers：** 必须按任务逐步执行；推荐使用 `superpowers:subagent-driven-development`，也可以使用 `superpowers:executing-plans`。每个步骤用 checkbox（`- [ ]`）跟踪进度。

**目标：** 新增 `eget sdk path <target>`，用于输出 SDK 配置基础目录或已安装版本目录。

**架构：** 路径选择逻辑放在 `internal/sdk.Service`，CLI 只负责解析参数和打印结果。`sdk path go` / `sdk path java` 解析 SDK 配置并输出基础目录；`sdk path go:1.20`、`sdk path go@1.21.5`、`sdk path java:17` 从 `sdk.installed.json` 读取已安装记录并返回匹配版本路径。

**技术栈：** Go、gookit/gcli、现有 `internal/sdk` target 解析、SDK 配置解析、SDK installed store，以及 `github.com/gookit/goutil/testutil/assert`。

---

## 语义定义

`eget sdk path <target>` 复用当前 `sdk.ParseTarget` 支持的 SDK target 语法：

- `go` / `java`：不带版本。输出对应 SDK 的配置基础目录，不查已安装最高版本。
- `go:1.20` / `java:17`：版本前缀。输出已安装版本中匹配该前缀的最高版本路径。
- `go@1.21.5`：精确版本。输出该已安装版本路径。

基础目录解析规则：

- 复用现有 SDK 配置 alias 语义。例如 `[sdk.jdk] aliases = ["java"]` 时，`java` 解析到 `jdk`。
- 通过 `Service.resolveConfig` 获取规范 SDK 名、`global.sdk_target`、`TargetTemplate` 和平台映射。
- 从 SDK root 和 target template 推导基础目录：
  - `gosdk/go{version}` -> `<sdk_root>/gosdk`
  - `jdk/openjdk-{version}` -> `<sdk_root>/jdk`
  - `jdk/zulu-{version}` -> `<sdk_root>/jdk`
  - `{version}` 或 `go{version}` -> `<sdk_root>`
  - 绝对模板 `D:/tools/jdk/openjdk-{version}` -> `D:/tools/jdk`
- 基础目录不做存在性检查；该命令报告的是配置语义路径。

已安装版本路径解析规则：

- 从 `Service.Store` 读取 SDK installed store。
- 使用配置解析出的规范 SDK 名查找记录，所以 `java:17` 查询的是 `jdk` 安装记录。
- 精确版本：匹配 `entry.Version == target.Version`。
- 前缀版本：匹配稳定版本，且 `entry.Version` 以 `target.Version + "."` 开头，然后用现有 `compareVersion` 选择最高版本。
- 找不到匹配项时返回明确错误，例如 `sdk go version 1.20 is not installed`。

输出规则：

- 只输出路径和换行。
- 首版不加表格、标签、JSON、`--check` 或存在性检查。

## 文件分工

- `internal/sdk/service.go`：新增 SDK path API 和选择逻辑。
- `internal/sdk/service_test.go`：新增 SDK path resolver 单元测试。
- `internal/cli/sdk_cmd.go`：新增 `sdk path` 子命令和 option。
- `internal/cli/app.go`：把 `sdk path` 加入手写 flag 白名单。
- `internal/cli/app_test.go`：新增 CLI 解析测试。
- `internal/cli/service.go`：扩展 `sdkCLIService` 接口。
- `internal/cli/handlers.go`：路由 `sdk.path` 并打印路径。
- `internal/cli/service_test.go`：新增 handler 输出测试。
- `README.md`：补充 `sdk path` 文档。
- `README.zh-CN.md`：补充 `sdk path` 中文文档。

---

### 任务 1：SDK Service 路径解析

**文件：**
- 修改：`internal/sdk/service.go`
- 测试：`internal/sdk/service_test.go`

- [x] **步骤 1：新增基础路径失败测试**

在 `internal/sdk/service_test.go` 增加：

```go
func TestServicePathReturnsConfiguredSDKBasePath(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	cfg.SDK["jdk"] = cfgpkg.SDKSection{
		Aliases: []string{"java"},
		Target:  stringPtr("jdk/openjdk-{version}"),
		ExtMap:  map[string]string{"linux": "tar.gz"},
	}
	svc := Service{Config: cfg, Store: Store{Path: filepath.Join(root, "sdk.installed.json")}, GOOS: "linux", GOARCH: "amd64"}

	goEntry, err := svc.Path("go")
	assert.NoErr(t, err)
	assert.Eq(t, filepath.Join(root, "sdks", "gosdk"), filepath.Clean(goEntry.Path))
	assert.Eq(t, "go", goEntry.Name)
	assert.Eq(t, "", goEntry.Version)

	javaEntry, err := svc.Path("java")
	assert.NoErr(t, err)
	assert.Eq(t, filepath.Join(root, "sdks", "jdk"), filepath.Clean(javaEntry.Path))
	assert.Eq(t, "jdk", javaEntry.Name)
	assert.Eq(t, "", javaEntry.Version)
}
```

- [x] **步骤 2：新增已安装版本路径失败测试**

在 `internal/sdk/service_test.go` 增加：

```go
func TestServicePathReturnsInstalledVersionPath(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	store := Store{Path: filepath.Join(root, "sdk.installed.json")}
	assert.NoErr(t, store.Record(InstalledEntry{Name: "go", Version: "1.20.3", Path: filepath.Join(root, "sdks", "gosdk", "go1.20.3")}))
	assert.NoErr(t, store.Record(InstalledEntry{Name: "go", Version: "1.20.12", Path: filepath.Join(root, "sdks", "gosdk", "go1.20.12")}))
	assert.NoErr(t, store.Record(InstalledEntry{Name: "go", Version: "1.21.5", Path: filepath.Join(root, "sdks", "gosdk", "go1.21.5")}))
	svc := Service{Config: cfg, Store: store, GOOS: "linux", GOARCH: "amd64"}

	prefix, err := svc.Path("go:1.20")
	assert.NoErr(t, err)
	assert.Eq(t, "1.20.12", prefix.Version)
	assert.Eq(t, filepath.Join(root, "sdks", "gosdk", "go1.20.12"), filepath.Clean(prefix.Path))

	exact, err := svc.Path("go@1.21.5")
	assert.NoErr(t, err)
	assert.Eq(t, "1.21.5", exact.Version)
	assert.Eq(t, filepath.Join(root, "sdks", "gosdk", "go1.21.5"), filepath.Clean(exact.Path))
}

func TestServicePathUsesSDKAliasForInstalledVersion(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	cfg.SDK["jdk"] = cfgpkg.SDKSection{
		Aliases: []string{"java"},
		Target:  stringPtr("jdk/zulu-{version}"),
		ExtMap:  map[string]string{"linux": "tar.gz"},
	}
	store := Store{Path: filepath.Join(root, "sdk.installed.json")}
	assert.NoErr(t, store.Record(InstalledEntry{Name: "jdk", Version: "17.0.9", Path: filepath.Join(root, "sdks", "jdk", "zulu-17.0.9")}))
	assert.NoErr(t, store.Record(InstalledEntry{Name: "jdk", Version: "17.0.11", Path: filepath.Join(root, "sdks", "jdk", "zulu-17.0.11")}))
	svc := Service{Config: cfg, Store: store, GOOS: "linux", GOARCH: "amd64"}

	got, err := svc.Path("java:17")
	assert.NoErr(t, err)
	assert.Eq(t, "jdk", got.Name)
	assert.Eq(t, "17.0.11", got.Version)
	assert.Eq(t, filepath.Join(root, "sdks", "jdk", "zulu-17.0.11"), filepath.Clean(got.Path))
}

func TestServicePathReturnsErrorForMissingInstalledVersion(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	svc := Service{Config: cfg, Store: Store{Path: filepath.Join(root, "sdk.installed.json")}, GOOS: "linux", GOARCH: "amd64"}

	_, err := svc.Path("go:1.20")
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "not installed")
}
```

- [x] **步骤 3：运行测试确认 RED**

执行：

```bash
go test ./internal/sdk -run TestServicePath -count=1
```

预期：编译失败，因为 `Service.Path` 还未定义。

- [x] **步骤 4：实现 SDK path API**

在 `internal/sdk/service.go` 增加 `Service.Path` 和 helper。实现要点：

```go
func (s Service) Path(rawTarget string) (InstalledEntry, error) {
	target, err := ParseTarget(rawTarget)
	if err != nil {
		return InstalledEntry{}, err
	}
	cfg, err := s.resolveConfig(target.Name)
	if err != nil {
		return InstalledEntry{}, err
	}
	if target.Kind == VersionLatest {
		path, err := s.sdkBasePath(cfg)
		if err != nil {
			return InstalledEntry{}, err
		}
		return InstalledEntry{Name: cfg.Name, Path: path}, nil
	}
	entries, err := s.Store.List(cfg.Name)
	if err != nil {
		return InstalledEntry{}, err
	}
	entry, ok := selectInstalledSDKPath(target, cfg.Name, entries)
	if !ok {
		return InstalledEntry{}, fmt.Errorf("sdk %s version %s is not installed", cfg.Name, target.Version)
	}
	return entry, nil
}
```

选择已安装版本的 helper：

```go
func selectInstalledSDKPath(target Target, name string, entries []InstalledEntry) (InstalledEntry, bool) {
	candidates := make([]InstalledEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name != name {
			continue
		}
		switch target.Kind {
		case VersionExact:
			if entry.Version == target.Version {
				return entry, true
			}
		case VersionPrefix:
			if isStableVersion(entry.Version) && strings.HasPrefix(entry.Version, target.Version+".") {
				candidates = append(candidates, entry)
			}
		}
	}
	if len(candidates) == 0 {
		return InstalledEntry{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareVersion(candidates[i].Version, candidates[j].Version) > 0
	})
	return candidates[0], true
}
```

基础路径 helper：

```go
func (s Service) sdkBasePath(cfg Config) (string, error) {
	prefix := targetTemplateBasePrefix(cfg.TargetTemplate)
	if prefix == "" {
		return s.sdkRoot(cfg), nil
	}
	if expanded, err := util.Expand(prefix); err == nil && expanded != "" {
		prefix = expanded
	}
	if filepath.IsAbs(prefix) {
		return filepath.Clean(prefix), nil
	}
	return filepath.Join(s.sdkRoot(cfg), filepath.FromSlash(prefix)), nil
}
```

`targetTemplateBasePrefix` 只需要针对 `{version}` 前的静态路径做最小处理：取 `{version}` 前缀，去掉末尾路径分隔符，再返回其目录；如果模板本身就是 `{version}` 或 `go{version}`，返回空字符串，交给 `sdkRoot`。

- [x] **步骤 5：运行测试确认 GREEN**

执行：

```bash
go test ./internal/sdk -run TestServicePath -count=1
```

预期：通过。

- [x] **步骤 6：提交任务 1**

执行：

```bash
git add internal/sdk/service.go internal/sdk/service_test.go docs/superpowers/plans/2026-05-31-sdk-path-command.md
git commit -m "feat(sdk): resolve sdk path targets"
```

---

### 任务 2：CLI 解析 `sdk path`

**文件：**
- 修改：`internal/cli/sdk_cmd.go`
- 修改：`internal/cli/app.go`
- 测试：`internal/cli/app_test.go`

- [x] **步骤 1：新增 CLI 解析失败测试**

在 `internal/cli/app_test.go` 增加：

```go
func TestMain_SDKPathRoutesAndBindsTarget(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"sdk", "path", "java:17"})
	assert.NoErr(t, err)
	assert.Eq(t, 1, len(calls))
	assert.Eq(t, "sdk.path", calls[0].name)
	opts, ok := calls[0].options.(*SDKPathOptions)
	assert.True(t, ok)
	assert.Eq(t, "java:17", opts.Target)
}

func TestMain_SDKPathRejectsMissingTarget(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(func(string, any) error {
		t.Fatal("handler should not run")
		return nil
	}, &stdout, &stderr).RunWithArgs([]string{"sdk", "path"})
	assert.Err(t, err)
}
```

- [x] **步骤 2：运行测试确认 RED**

执行：

```bash
go test ./internal/cli -run TestMain_SDKPath -count=1
```

预期：编译失败，因为 `SDKPathOptions` 未定义，或命令未注册导致路由失败。

- [x] **步骤 3：新增 option 和子命令**

在 `internal/cli/sdk_cmd.go` 增加：

```go
type SDKPathOptions struct {
	Target string
}
```

在 `newSDKCmd` 中：

- 创建 `pathOpts := &SDKPathOptions{}`。
- 将 `newSDKPathCmd(pathOpts, handler)` 加入 `cmd.Subs`。
- 在 reset 函数中重置 `*pathOpts = SDKPathOptions{}`。

新增命令函数：

```go
func newSDKPathCmd(opts *SDKPathOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("path", "Print SDK path")
	cmd.Config = func(c *gcli.Command) {
		c.AddArg("target", "SDK target, for example go, go:1.20, or java:17", true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Target = c.Arg("target").String()
		if err := validateNoFlagArgs(append([]string{opts.Target}, args...)); err != nil {
			return err
		}
		if len(args) > 0 {
			return fmt.Errorf("too many arguments: %v", args)
		}
		snapshot := *opts
		return handler("sdk.path", &snapshot)
	}
	return cmd
}
```

在 `internal/cli/app.go` 的 `commandFlagSpecs["sdk"].subs` 中添加：

```go
"path": {},
```

- [x] **步骤 4：运行测试确认 GREEN**

执行：

```bash
go test ./internal/cli -run TestMain_SDKPath -count=1
```

预期：通过。

- [ ] **步骤 5：提交任务 2**

执行：

```bash
git add internal/cli/sdk_cmd.go internal/cli/app.go internal/cli/app_test.go docs/superpowers/plans/2026-05-31-sdk-path-command.md
git commit -m "feat(cli): parse sdk path command"
```

---

### 任务 3：CLI handler 和文档

**文件：**
- 修改：`internal/cli/service.go`
- 修改：`internal/cli/handlers.go`
- 修改：`internal/cli/service_test.go`
- 修改：`README.md`
- 修改：`README.zh-CN.md`

- [ ] **步骤 1：新增 handler 失败测试**

在 `internal/cli/service_test.go` 的 `fakeSDKService` 增加字段：

```go
pathTarget string
pathEntry  sdk.InstalledEntry
```

增加方法：

```go
func (f *fakeSDKService) Path(target string) (sdk.InstalledEntry, error) {
	f.pathTarget = target
	return f.pathEntry, f.err
}
```

增加测试：

```go
func TestHandleSDKPathPrintsPath(t *testing.T) {
	fake := &fakeSDKService{
		pathEntry: sdk.InstalledEntry{Name: "jdk", Version: "17.0.11", Path: "D:/tools/jdk/zulu-17.0.11"},
	}
	svc := &cliService{sdkService: fake}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("sdk.path", &SDKPathOptions{Target: "java:17"})
	assert.NoErr(t, err)
	assert.Eq(t, "java:17", fake.pathTarget)
	assert.Eq(t, "D:/tools/jdk/zulu-17.0.11\n", out.String())
}
```

- [ ] **步骤 2：运行测试确认 RED**

执行：

```bash
go test ./internal/cli -run TestHandleSDKPath -count=1
```

预期：编译失败，因为 `sdkCLIService` 缺少 `Path`，或 handler 尚未路由 `sdk.path`。

- [ ] **步骤 3：实现 handler**

在 `internal/cli/service.go` 扩展 `sdkCLIService`：

```go
Path(string) (sdk.InstalledEntry, error)
```

在 `internal/cli/handlers.go` 的 `handle` 中增加：

```go
case "sdk.path":
	opts := options.(*SDKPathOptions)
	return s.handleSDKPath(opts)
```

新增：

```go
func (s *cliService) handleSDKPath(opts *SDKPathOptions) error {
	if opts == nil || opts.Target == "" {
		return fmt.Errorf("sdk path target is required")
	}
	entry, err := s.sdkService.Path(opts.Target)
	if err != nil {
		return err
	}
	ccolor.Println(entry.Path)
	return nil
}
```

- [ ] **步骤 4：更新 README**

在 `README.md` 的 SDK 示例中增加：

```bash
eget sdk path go
eget sdk path go:1.20
eget sdk path java:17
```

在 `sdk` 命令说明中增加：

```markdown
- `sdk path <target>` prints a configured SDK base path for unversioned targets such as `go` or `java`, and prints an installed version path for versioned targets such as `go:1.20`, `go@1.21.5`, or `java:17`.
```

在 `README.zh-CN.md` 增加中文说明：

```markdown
- `sdk path <target>` 输出 SDK 路径；`go`、`java` 等不带版本目标输出配置的 SDK 基础目录，`go:1.20`、`go@1.21.5`、`java:17` 等带版本目标输出已安装版本目录。
```

- [ ] **步骤 5：运行目标测试**

执行：

```bash
go test ./internal/cli -run TestHandleSDKPath -count=1
```

预期：通过。

- [ ] **步骤 6：提交任务 3**

执行：

```bash
git add internal/cli/service.go internal/cli/handlers.go internal/cli/service_test.go README.md README.zh-CN.md docs/superpowers/plans/2026-05-31-sdk-path-command.md
git commit -m "feat(sdk): print sdk paths"
```

---

### 任务 4：最终验证

**文件：**
- 正常情况下不需要改源码；如果验证发现问题，再按问题所在文件做最小修复。

- [ ] **步骤 1：运行聚焦测试**

执行：

```bash
go test ./internal/sdk ./internal/cli
```

预期：通过。

- [ ] **步骤 2：运行全量测试**

执行：

```bash
go test ./...
```

预期：通过。

- [ ] **步骤 3：运行手动 CLI 检查**

执行：

```bash
go run ./cmd/eget sdk path go
go run ./cmd/eget sdk path java
```

预期：

- 如果本机配置了 SDK，命令只打印路径。
- 如果本机没有配置对应 SDK，返回清晰配置错误。
- 命令不能 panic，不能输出表格或额外标签。

- [ ] **步骤 4：提交计划 checkbox 更新**

如果最终只剩计划 checkbox 状态变化，执行：

```bash
git add docs/superpowers/plans/2026-05-31-sdk-path-command.md
git commit -m "docs: complete sdk path plan checklist"
```

如果没有剩余变更，则不需要提交。

## 自查

- 覆盖范围：计划覆盖不带版本的 SDK 基础路径、前缀版本已安装路径、精确版本已安装路径、`java` alias、CLI 解析、handler 输出、文档和最终验证。
- 占位检查：没有未完成占位内容；每个任务都有明确文件、测试代码、命令和预期结果。
- 类型一致性：`SDKPathOptions`、`Service.Path`、`InstalledEntry`、`sdkCLIService.Path`、`sdk.path` 路由在各任务中保持一致。
