# SDK Path Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `eget sdk path <target>` to print either a configured SDK base directory or an installed SDK version directory.

**Architecture:** Keep SDK path selection in `internal/sdk.Service` so CLI output remains thin. `sdk path go` / `sdk path java` resolve the configured SDK section and print the base directory derived from `global.sdk_target` plus the static prefix of that SDK's target template. Versioned targets such as `go:1.20` and `go@1.21.5` read `sdk.installed.json` and return the installed path for the highest matching prefix version or exact version.

**Tech Stack:** Go, gookit/gcli, existing `internal/sdk` target parsing/config resolution/store, and `github.com/gookit/goutil/testutil/assert`.

---

## Semantics

`eget sdk path <target>` accepts the same SDK target grammar already parsed by `sdk.ParseTarget`:

- `go` / `java`: no version. Print the configured SDK base path, not an installed version path.
- `go:1.20` / `java:17`: prefix version. Print the installed path for the highest installed stable version matching the prefix.
- `go@1.21.5`: exact version. Print the installed path for that installed version.

Base path resolution:

- Resolve aliases through existing SDK config semantics. For example, `java` resolves to configured `jdk` when `[sdk.jdk] aliases = ["java"]`.
- Resolve the SDK config using `Service.resolveConfig`, preserving platform maps and `global.sdk_target`.
- Compute the base directory from the SDK root and target template:
  - `gosdk/go{version}` -> `<sdk_root>/gosdk`
  - `jdk/openjdk-{version}` -> `<sdk_root>/jdk`
  - `jdk/zulu-{version}` -> `<sdk_root>/jdk`
  - `{version}` or `go{version}` -> `<sdk_root>`
  - absolute target templates such as `D:/tools/jdk/openjdk-{version}` -> `D:/tools/jdk`
- Do not check whether the base directory exists; this command reports configured paths.

Installed version path resolution:

- Load installed SDK entries from `Service.Store`.
- Use the canonical SDK name returned by config resolution, so `java:17` searches installed `jdk` entries.
- For exact targets, match `entry.Version == target.Version`.
- For prefix targets, match stable installed versions whose `entry.Version` has prefix `target.Version + "."`, then select the highest version using existing `compareVersion`.
- Return a clear error if no installed entry matches.

Output:

- Print only the path followed by a newline.
- No table, labels, JSON, or existence check in this first version.

## File Map

- `internal/sdk/service.go`: add service-level SDK path API and helper selection logic.
- `internal/sdk/service_test.go`: add SDK path resolver tests.
- `internal/cli/sdk_cmd.go`: add `path` subcommand and options.
- `internal/cli/app.go`: add `sdk path` to manual flag spec map.
- `internal/cli/app_test.go`: add CLI parser tests.
- `internal/cli/service.go`: extend `sdkCLIService` with the path API.
- `internal/cli/handlers.go`: route `sdk.path` to SDK service and print path.
- `internal/cli/service_test.go`: add handler output tests.
- `README.md`: document `sdk path`.
- `README.zh-CN.md`: document `sdk path`.

---

### Task 1: SDK Service Path Resolver

**Files:**
- Modify: `internal/sdk/service.go`
- Test: `internal/sdk/service_test.go`

- [ ] **Step 1: Write failing tests for base path targets**

Add tests to `internal/sdk/service_test.go`:

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

- [ ] **Step 2: Write failing tests for installed version targets**

Add tests to `internal/sdk/service_test.go`:

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

- [ ] **Step 3: Run tests to verify RED**

Run:

```bash
go test ./internal/sdk -run TestServicePath -count=1
```

Expected: build failure because `Service.Path` is undefined.

- [ ] **Step 4: Implement service path API**

Add this API to `internal/sdk/service.go`:

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

func targetTemplateBasePrefix(template string) string {
	before, _, _ := strings.Cut(template, "{version}")
	before = strings.TrimRight(before, `/\`)
	if before == "" {
		return ""
	}
	if strings.ContainsAny(filepath.Base(filepath.FromSlash(before)), "{}") {
		return filepath.Dir(filepath.FromSlash(before))
	}
	return filepath.Dir(filepath.FromSlash(before))
}
```

Adjust helper implementation if tests reveal Windows path edge cases, but keep behavior scoped to the semantics above.

- [ ] **Step 5: Run tests to verify GREEN**

Run:

```bash
go test ./internal/sdk -run TestServicePath -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 1**

Run:

```bash
git add internal/sdk/service.go internal/sdk/service_test.go docs/superpowers/plans/2026-05-31-sdk-path-command.md
git commit -m "feat(sdk): resolve sdk path targets"
```

---

### Task 2: CLI Parser For `sdk path`

**Files:**
- Modify: `internal/cli/sdk_cmd.go`
- Modify: `internal/cli/app.go`
- Test: `internal/cli/app_test.go`

- [ ] **Step 1: Write failing parser tests**

Add tests to `internal/cli/app_test.go`:

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

- [ ] **Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/cli -run TestMain_SDKPath -count=1
```

Expected: compile failure because `SDKPathOptions` is undefined or route failure because `sdk path` is not registered.

- [ ] **Step 3: Add CLI option and subcommand**

In `internal/cli/sdk_cmd.go`, add:

```go
type SDKPathOptions struct {
	Target string
}
```

In `newSDKCmd`, create `pathOpts := &SDKPathOptions{}`, include `newSDKPathCmd(pathOpts, handler)` in `cmd.Subs`, and reset it.

Add:

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

In `internal/cli/app.go`, add `path: {}` under the `sdk` subcommand flag spec.

- [ ] **Step 4: Run tests to verify GREEN**

Run:

```bash
go test ./internal/cli -run TestMain_SDKPath -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 2**

Run:

```bash
git add internal/cli/sdk_cmd.go internal/cli/app.go internal/cli/app_test.go docs/superpowers/plans/2026-05-31-sdk-path-command.md
git commit -m "feat(cli): parse sdk path command"
```

---

### Task 3: CLI Handler And Documentation

**Files:**
- Modify: `internal/cli/service.go`
- Modify: `internal/cli/handlers.go`
- Modify: `internal/cli/service_test.go`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

- [ ] **Step 1: Write failing handler test**

Extend `fakeSDKService` in `internal/cli/service_test.go` with fields:

```go
pathTarget string
pathEntry  sdk.InstalledEntry
```

Add method:

```go
func (f *fakeSDKService) Path(target string) (sdk.InstalledEntry, error) {
	f.pathTarget = target
	return f.pathEntry, f.err
}
```

Add test:

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

- [ ] **Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/cli -run TestHandleSDKPath -count=1
```

Expected: compile failure because `sdkCLIService` lacks `Path` or handler does not route `sdk.path`.

- [ ] **Step 3: Implement handler**

In `internal/cli/service.go`, extend `sdkCLIService`:

```go
Path(string) (sdk.InstalledEntry, error)
```

In `internal/cli/handlers.go`, add a case in `handle`:

```go
case "sdk.path":
	opts := options.(*SDKPathOptions)
	return s.handleSDKPath(opts)
```

Add:

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

- [ ] **Step 4: Update README files**

In `README.md`, add SDK examples:

```bash
eget sdk path go
eget sdk path go:1.20
eget sdk path java:17
```

In the `sdk` command section, add:

```markdown
- `sdk path <target>` prints a configured SDK base path for unversioned targets such as `go` or `java`, and prints an installed version path for versioned targets such as `go:1.20`, `go@1.21.5`, or `java:17`.
```

In `README.zh-CN.md`, add the equivalent Chinese text:

```markdown
- `sdk path <target>` 输出 SDK 路径；`go`、`java` 等不带版本目标输出配置的 SDK 基础目录，`go:1.20`、`go@1.21.5`、`java:17` 等带版本目标输出已安装版本目录。
```

- [ ] **Step 5: Run target tests**

Run:

```bash
go test ./internal/cli -run TestHandleSDKPath -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 3**

Run:

```bash
git add internal/cli/service.go internal/cli/handlers.go internal/cli/service_test.go README.md README.zh-CN.md docs/superpowers/plans/2026-05-31-sdk-path-command.md
git commit -m "feat(sdk): print sdk paths"
```

---

### Task 4: Final Verification

**Files:**
- No source changes expected unless verification finds a bug.

- [ ] **Step 1: Run focused tests**

Run:

```bash
go test ./internal/sdk ./internal/cli
```

Expected: PASS.

- [ ] **Step 2: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run manual CLI checks**

Run:

```bash
go run ./cmd/eget sdk path go
go run ./cmd/eget sdk path java
```

Expected: both commands print one path or return a clear config error if SDK config is not present in the local test environment. The command must not panic and must not print tables or labels.

- [ ] **Step 4: Commit plan checkbox updates if needed**

If Task 4 changed only the plan checkbox state, run:

```bash
git add docs/superpowers/plans/2026-05-31-sdk-path-command.md
git commit -m "docs: complete sdk path plan checklist"
```

If no source or plan changes remain, no commit is needed.

## Self-Review

- Spec coverage: The plan covers unversioned SDK base paths, prefix installed version paths, exact installed version paths, Java alias handling, CLI parsing, handler output, docs, and full verification.
- Placeholder scan: No placeholder steps remain; each task has concrete files, test code, commands, and expected outcomes.
- Type consistency: `SDKPathOptions`, `Service.Path`, `InstalledEntry`, `sdkCLIService.Path`, and handler route `sdk.path` are used consistently across tasks.
