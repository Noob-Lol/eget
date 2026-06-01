# Go 公共工具与子包整理计划

## 背景

上一轮已经完成生产文件和大测试文件拆分，当前主要问题从“单文件膨胀”转为两个方向：

- `internal/cli` 和 `internal/install` 根目录文件数量偏多，部分职责可以下沉到更明确的子包。
- 多个包里存在小型公共 helper 重复实现，例如 `cloneStringMap`、`firstNonEmptyString`、`expandPathOrRaw`、`fileExists`。

本计划基于当前 GitNexus 索引和本地扫描结果制定。当前 GitNexus CLI 可用，索引状态为 up-to-date；本会话没有 GitNexus MCP resources 暴露，因此实施时优先使用：

```bash
gitnexus status
gitnexus cypher --repo eget "<read-only cypher>"
gitnexus detect-changes --repo eget
```

## 进度

- [ ] 阶段 0：计划提交
- [ ] 阶段 1：抽取低风险公共工具到 `internal/util`
- [ ] 阶段 2：抽取 CLI 渲染逻辑到 `internal/cli/render`
- [ ] 阶段 3：抽取 CLI prompt 逻辑到 `internal/cli/prompts`
- [ ] 阶段 4：抽取 install detector 逻辑到 `internal/install/detect`
- [ ] 阶段 5：评估并拆分 install archive 子包

## 总体原则

1. 每阶段独立提交，提交信息使用 `refactor:` 前缀。
2. 每阶段只处理一个边界，不混合顺手改行为。
3. 优先移动完整函数和类型；只有跨 package 必须访问时才导出。
4. 不为减少文件数量牺牲 API 边界。根目录文件多但协作紧密的 runner/service 暂不强拆。
5. 每阶段至少运行定向测试和 `go test ./...`。
6. 涉及 `internal/cli` 或 `internal/install` 的跨包改动后，运行 GitNexus 变更影响检查：

```bash
gitnexus detect-changes --repo eget
```

如果该命令超时或不可用，使用 `git diff --stat` 和对应 package 测试作为兜底。

## 阶段 0：计划提交

目标：保存当前整理计划，后续实施时用 checkbox 跟踪。

文件：

- 新增：`docs/plans/go-subpackage-utility-plan.md`

验收：

```bash
git status --short
```

提交：

```bash
git add docs/plans/go-subpackage-utility-plan.md
git commit -m "docs: plan go subpackage utility split"
```

## 阶段 1：抽取低风险公共工具到 `internal/util`

目标：先处理跨包重复 helper，减少后续子包拆分时的重复导出和临时桥接。

### 现状

GitNexus / 本地扫描确认重复点：

| helper | 当前文件 | 建议 |
| --- | --- | --- |
| `cloneStringMap` | `internal/app/install_options.go`、`internal/config/gookit.go`、`internal/install/service.go`、`internal/sdk/config.go` | 抽为 `util.CloneStringMap` |
| `firstNonEmptyString` | `internal/app/config.go`、`internal/cli/config_handler.go` | 抽为 `util.FirstNonEmptyString` |
| `expandPathOrRaw` | `internal/app/config.go`、`internal/cli/config_handler.go` | 抽为 `util.ExpandPathOrRaw` |
| `fileExists` / `dirExists` | `internal/config/paths.go`、`internal/installed/store.go`、`internal/cli/sudo_warning.go`、`internal/cli/config_handler.go` | 抽为 `util.FileExists` / `util.DirExists`，按调用点谨慎替换 |

不在本阶段处理：

- `formatBytes`：`internal/app/cache/ui.go` 和 `internal/cli/cache_handler.go` 输出格式不同，属于用户可见行为，单独评估。
- `install/network.go` 包装 `client` 的函数：这是兼容/测试 hook facade，不属于重复 helper。
- 测试 helper：例如 `writeTestFile`、`writeCLIFile`，暂不抽到公共 testutil。

### 计划修改

文件：

- 修改：`internal/util/helpers.go`
- 修改：`internal/app/install_options.go`
- 修改：`internal/app/config.go`
- 修改：`internal/config/gookit.go`
- 修改：`internal/install/service.go`
- 修改：`internal/sdk/config.go`
- 视调用点修改：`internal/config/paths.go`
- 视调用点修改：`internal/installed/store.go`
- 视调用点修改：`internal/cli/sudo_warning.go`
- 视调用点修改：`internal/cli/config_handler.go`

新增 `internal/util/helpers.go` 函数：

```go
func CloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}

func FirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func ExpandPathOrRaw(path string) string {
	expanded, err := Expand(path)
	if err != nil {
		return path
	}
	return expanded
}

func FileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && !info.IsDir()
}

func DirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && info.IsDir()
}
```

注意：

- `CloneStringMap(nil)` 返回 `nil`。
- `config/gookit.go` 当前只在 `len(map) > 0` 后调用 `cloneStringMap`，改成 `util.CloneStringMap` 不应影响 dump 输出。
- `FileExists` 使用 `filepath.Clean`，替换前确认原函数对空 path 的语义一致。

验收：

```bash
go test ./internal/app ./internal/cli ./internal/config ./internal/install ./internal/sdk ./internal/installed -count=1
go test ./...
gitnexus detect-changes --repo eget
```

提交：

```bash
git add internal/util internal/app internal/cli internal/config internal/install internal/sdk internal/installed
git commit -m "refactor: share common utility helpers"
```

## 阶段 2：抽取 CLI 渲染逻辑到 `internal/cli/render`

目标：减少 `internal/cli` 根目录最大生产文件 `render.go` 的职责重量，让 CLI handler 只负责调度，渲染 DTO 和输出逻辑进入子包。

### 现状

`internal/cli/render.go` 当前约 500 行，包含：

- display DTO：release/list/show/query/search/sdk display structs
- 时间格式化：`compactTime`、`compactTimeOmit`
- 表格文本处理：`truncateTableText`
- JSON 输出：`printJSON`
- query/search 输出：`printQueryResult`、`printSearchResult`
- SDK index 输出：`printSDKIndexSummary`、`sdkIndexSummary`、platform/version rows

主要调用点：

- `internal/cli/list_handler.go`
- `internal/cli/query_search_handler.go`
- `internal/cli/sdk_handler.go`
- `internal/cli/update_handler.go`
- `internal/cli/app.go` 中 `compactTimeLayout` 用于 build time 规范化

### 子包边界

新增子包：

```text
internal/cli/render
```

候选文件：

- 新增：`internal/cli/render/time.go`
- 新增：`internal/cli/render/query.go`
- 新增：`internal/cli/render/search.go`
- 新增：`internal/cli/render/list.go`
- 新增：`internal/cli/render/sdk.go`
- 新增：`internal/cli/render/json.go`
- 新增测试：按现有测试迁移到 `internal/cli/render/*_test.go`，或保留在 `internal/cli` 通过导出 API 测试。

建议导出 API：

```go
const CompactTimeLayout = "2006-01-02T15:04:05"

func CompactTime(value time.Time) string
func CompactTimeOmit(value time.Time) string
func TruncateTableText(value string, max int) string
func PrintJSON(value any) error
func PrintQueryResult(result app.QueryResult)
func QueryResultJSON(result app.QueryResult) (string, error)
func PrintSearchResult(result app.SearchResult)
func SDKEntriesToDisplay(entries []sdk.InstalledEntry) []SDKInstalledEntryDisplay
func SDKCachedIndexesToDisplay(infos []sdk.CachedIndexInfo) []SDKCachedIndexDisplay
func PrintSDKIndexSummary(index sdk.Index)
```

display DTO 是否导出按调用需要决定：

- 如果只在 render 包内部使用，保持小写。
- 如果 `sdk_handler.go` 需要构造或断言 display 数据，导出对应 DTO。

### 调用点调整

示例：

```go
// before
return printJSON(sdkEntriesToDisplay(entries))

// after
return render.PrintJSON(render.SDKEntriesToDisplay(entries))
```

```go
// before
printQueryResult(result)

// after
render.PrintQueryResult(result)
```

```go
// before
rows = append(rows, []any{entry.Name, entry.Version, entry.Path, compactTime(entry.InstalledAt)})

// after
rows = append(rows, []any{entry.Name, entry.Version, entry.Path, render.CompactTime(entry.InstalledAt)})
```

注意：

- 避免 `internal/cli/render` import `internal/cli`，否则会形成循环。
- `render` 可以 import `internal/app` 和 `internal/sdk`。
- `internal/cli/app.go` 只需要 `render.CompactTimeLayout`，不要为 build info 保留旧常量。

验收：

```bash
go test ./internal/cli -count=1
go test ./...
gitnexus detect-changes --repo eget
```

提交：

```bash
git add internal/cli
git commit -m "refactor: move cli rendering to subpackage"
```

## 阶段 3：抽取 CLI prompt 逻辑到 `internal/cli/prompts`

目标：把交互式选择和确认逻辑从 `internal/cli` 根目录下沉，减少 CLI 根目录工具函数数量。

### 现状

候选文件：

- `internal/cli/prompts.go`
- `internal/cli/prompt_test.go`

主要调用点：

- `internal/cli/wiring.go`：`runner.Prompt = promptSelect`
- `internal/cli/config_handler.go`：`promptConfirmOverwrite`
- `internal/cli/uninstall_handler.go`：确认删除逻辑如果仍使用 prompt helper

### 子包边界

新增子包：

```text
internal/cli/prompts
```

建议 API：

```go
func SelectIndex(choices []string) (int, error)
func Select(title, filterPrompt string, choices []string) (int, error)
func ConfirmOverwrite(path string) (bool, error)
func ConfirmRemove(target string) (bool, error)
```

内部测试可保留未导出的 `runSelectIndex`：

```go
func runSelectIndex(in io.Reader, out io.Writer, be backend.Backend, title, filterPrompt string, choices []string) (int, error)
```

### 调用点调整

```go
runner.Prompt = prompts.Select
```

```go
confirmed, err := prompts.ConfirmOverwrite(info.Path)
```

注意：

- `prompts` 不应 import `internal/cli`。
- prompt 函数当前直接使用 `os.Stderr` / stdin 行为，移动时保持行为不变。

验收：

```bash
go test ./internal/cli -count=1
go test ./...
gitnexus detect-changes --repo eget
```

提交：

```bash
git add internal/cli
git commit -m "refactor: move cli prompts to subpackage"
```

## 阶段 4：抽取 install detector 逻辑到 `internal/install/detect`

目标：把 release asset detector 逻辑从 `internal/install` 根目录中抽出，降低 install 根目录文件数量，并为后续 archive 子包拆分建立更清晰边界。

### 现状

候选文件：

- `internal/install/detectors.go`
- `internal/install/detectors_test.go`
- `internal/install/defaults_detector_test.go`
- `internal/install/service_detector_test.go` 可按需要保留在 install 包测试 service 行为

相关入口：

- `internal/install/service.go` 中 `Detector` interface
- `internal/install/default_service.go` 中 detector factory wiring
- `internal/install/service.go` 中 `SelectDetector`

### 子包边界

新增子包：

```text
internal/install/detect
```

建议 API：

```go
type Detector interface {
	Detect(assets []string) (string, []string, error)
}

func NewAllDetector() Detector
func NewAssetDetector(asset string, anti bool, re *regexp.Regexp) Detector
func NewChain(detectors []Detector, system Detector) Detector
func NewSystemDetector(goos, goarch string) (Detector, error)
func NewSystemDetectorWithLibc(goos, goarch, libc string) (Detector, error)
func CompileAssetRegex(expr string) (*regexp.Regexp, error)
func SelectableReleaseAssets(assets []string) []string
```

注意：

- 当前 `compileAssetRegex`、`selectableReleaseAssets` 等是小写；跨包后如果 service 或测试直接依赖，需要导出。
- `install.Service` 可以继续保留自己的 `type Detector = detect.Detector` 或直接引用 `detect.Detector`。
- 目标是让 `default_service.go` wiring 变成：

```go
AllDetectorFactory: detect.NewAllDetector,
SystemDetectorFactory: detect.NewSystemDetector,
AssetDetectorFactory: detect.NewAssetDetector,
DetectorChainFactory: detect.NewChain,
```

### 风险点

- `service_detector_test.go` 当前可能依赖未导出 detector 类型或 helper。优先改成测试 `Service.SelectDetector` 的行为，不测试具体实现类型。
- 如果 export API 变多，先只导出 service/default wiring 真正需要的函数，不为了测试导出内部细节。

验收：

```bash
go test ./internal/install -count=1
go test ./...
gitnexus detect-changes --repo eget
```

提交：

```bash
git add internal/install
git commit -m "refactor: move install detectors to subpackage"
```

## 阶段 5：评估并拆分 install archive 子包

目标：评估 `internal/install/archive*`、`chooser.go`、`system7z.go` 是否能形成稳定 archive 子包；如果边界清楚，再拆。

### 现状

候选文件：

- `internal/install/archive.go`
- `internal/install/archive_formats.go`
- `internal/install/archive_paths.go`
- `internal/install/file_modes.go`
- `internal/install/chooser.go`
- `internal/install/system7z.go`
- 对应测试：`defaults_archive_test.go`、`chooser_test.go`、`system7z_test.go`

主要牵连类型：

- `Chooser`
- `Extractor`
- `ExtractedFile`
- `ArchiveExtractOptions`
- `File`
- `FileType`
- `Archive`
- `ArchiveFn`
- `DecompFn`

### 建议边界

优先目标子包：

```text
internal/install/archive
```

理想 API：

```go
type FileType byte
type File struct { ... }
type Archive interface { ... }
type Chooser interface { ... }
type Extractor interface { ... }
type ExtractedFile struct { ... }
type ExtractOptions struct { StripComponents int }

func NewFileChooser(expr string) (Chooser, error)
func NewBinaryChooser(tool string) *BinaryChooser
func NewArchiveExtractor(file Chooser, ar ArchiveFn, decompress DecompFn) *ArchiveExtractor
func NewTarArchive(data []byte, decompress DecompFn) (Archive, error)
func NewZipArchive(data []byte, decompress DecompFn) (Archive, error)
func NewSevenZipArchive(data []byte, decompress DecompFn) (Archive, error)
func NewSystem7zExtractor(filename, tool string, chooser Chooser, exe string) *System7zExtractor
```

### 推荐实施方式

这个阶段不要直接一次性搬完。先做可行性小步：

1. 用 GitNexus 查询 archive 相关类型调用方：

```bash
gitnexus cypher --repo eget "MATCH (caller)-[:CodeRelation {type:'CALLS'}]->(callee) WHERE callee.name IN ['NewArchiveExtractor','NewFileChooser','NewBinaryChooser','NewSystem7zExtractor'] RETURN callee.name, caller.name, caller.filePath LIMIT 100"
```

2. 如果调用方主要集中在 `internal/install` 内部，则可以抽。
3. 如果调用方广泛跨 `app` / `sdk` / `client`，先停止，改为只整理 install 根目录内部文件命名，不做子包。

### 风险点

- `Extractor` / `ExtractedFile` 当前被 runner 大量使用。把它们移出 install 包会导致 `RunResult`、runner 测试、service factory 都变更，影响面较大。
- `system7z.go` 有测试 hook `runSystem7zCommand`，移动到子包后测试需要在同 package 或提供 test-only hook。
- archive 安全路径函数多数未导出，测试如果移到外部 package 会需要额外导出。建议 archive 子包测试使用 `package archive`，保留内部可见性。

### 验收

```bash
go test ./internal/install -count=1
go test ./...
gitnexus detect-changes --repo eget
```

提交：

```bash
git add internal/install
git commit -m "refactor: move install archive helpers to subpackage"
```

如果评估发现阶段 5 需要导出大量内部类型，允许只完成评估并在计划中记录：

```markdown
- [x] 阶段 5：评估 install archive 子包，暂缓实施
```

并提交：

```bash
git add docs/plans/go-subpackage-utility-plan.md
git commit -m "docs: record install archive split assessment"
```

## 不建议本轮处理

- 不建议抽 `internal/cli/commands`：`*_cmd.go` 依赖 `CommandHandler` 和大量 option 类型，容易引入循环依赖。
- 不建议抽 `internal/cli/handlers`：handler 是 `cliService` 方法，移动会迫使导出 service 字段或写大量接口。
- 不建议抽 `internal/install/runner`：runner 和 `Service`、`Options`、`RunResult`、安装主链路 helper 绑定紧，跨包后导出面会明显变大。
- 不建议抽 `formatBytes`：两个实现输出格式不同，用户可见，需单独决策。

## 推荐执行顺序

1. 阶段 1：公共 util
2. 阶段 2：CLI render
3. 阶段 3：CLI prompts
4. 阶段 4：install detect
5. 阶段 5：install archive 可行性评估和可选实施

这个顺序的理由：

- 公共 util 风险最低，且能减少后续迁移时的重复桥接。
- CLI render 和 prompts 不依赖 `cliService`，不容易形成 import cycle。
- install detect 边界比 archive 清楚，适合先做。
- archive 牵涉 extractor、chooser、system7z、runner，必须最后处理。
