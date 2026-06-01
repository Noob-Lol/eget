# Go 文件整理规划

## 背景

本次扫描目标是找出已经膨胀、职责混杂、后续维护成本较高的 Go 文件，并规划分阶段整理方案。当前只做规划，不直接改业务代码。

## 进度

- [x] 阶段 0：基线保护和规划提交
- [x] 阶段 1：拆分 CLI handler
- [x] 阶段 2：拆分 install runner
- [x] 阶段 3：拆分 client network
- [ ] 阶段 4：拆分 app install/update
- [ ] 阶段 5：拆分 SDK service

## 扫描结论

按行数和函数数量看，最需要关注的不是单纯最大的测试文件，而是同时满足这些条件的文件：

- 生产代码超过 500 行
- 单文件职责超过 2 个
- 测试文件已经跟随生产文件一起膨胀
- 后续功能改动高概率继续落在同一文件

当前热点：

| 文件 | 行数 | 函数数 | 判断 |
| --- | ---: | ---: | --- |
| `internal/cli/handlers.go` | 1335 | 61 | 高优先级，命令处理、输出、doctor、outdated 进度、cache server 混在一起 |
| `internal/install/runner.go` | 1049 | 56 | 高优先级，安装主流程、下载缓存、GUI installer、run-asset、选择逻辑、输出路径混在一起 |
| `internal/client/network.go` | 938 | 63 | 高优先级，HTTP、auth、API cache、ghproxy、range download、notice、cache path 混在一起 |
| `internal/app/install.go` | 811 | 37 | 中高优先级，安装编排、批量调度、配置合并、installed store 记录混在一起 |
| `internal/sdk/service.go` | 770 | 39 | 中优先级，install/list/remove/index/search/fetch/路径解析混在一起 |
| `internal/app/update.go` | 558 | 25 | 中优先级，候选查找、installed option 恢复、批量更新、配置网络选项混在一起 |

测试热点：

| 文件 | 行数 | 判断 |
| --- | ---: | --- |
| `internal/cli/service_test.go` | 2327 | 应随 CLI handler 拆分同步拆成命令级测试文件 |
| `internal/install/runner_test.go` | 1967 | 应随 runner 职责拆分同步拆测试 |
| `internal/cli/app_test.go` | 1490 | CLI parser 测试可按 command group 拆分 |
| `internal/app/install_test.go` | 1419 | 可随 app install 生产拆分同步拆 |

## 整理原则

1. 优先做同 package 文件拆分，不改包名、不改导出 API、不改行为。
2. 每个阶段只处理一个模块，完成后运行定向测试和 `go test ./...`。
3. 每个阶段独立提交，便于 review 和回滚。
4. 先拆生产代码，再拆对应测试；不要跨模块混合改动。
5. 不在整理阶段顺手重构算法或修 bug，除非测试暴露必须修复的问题。
6. 拆分后单个生产文件目标尽量控制在 400 行以内；主入口文件可以保留较短编排函数。

## 阶段 0：基线保护

目标：建立重构前安全基线。

操作：

- 运行 `go test ./...`
- 记录当前最大文件列表，用于整理后对比
- 确认工作区干净后开始阶段性拆分

验收：

- `go test ./...` 通过
- 每个后续阶段开始前无未提交改动

## 阶段 1：拆分 CLI handler

优先级：最高。收益高，风险低，因为可以保持 `package cli` 不变，只移动函数。

现状：

- `internal/cli/handlers.go` 同时包含命令分发、install/download/add、uninstall/list/show、config doctor、update/self-update、sdk、query/search、cache、通用输出工具。
- `internal/cli/service_test.go` 覆盖范围过宽，新增任意 CLI 行为都会继续膨胀。

建议拆分：

- 保留：`internal/cli/handlers.go`
  - 只保留 `handle()` 总分发
  - 可保留极少数通用入口 helper
- 新建：`internal/cli/install_handler.go`
  - `handle install` 分支对应逻辑
  - add/download 的薄处理逻辑如果足够短，可以一起放这里
- 新建：`internal/cli/uninstall_handler.go`
  - `handleUninstall`
- 新建：`internal/cli/list_handler.go`
  - `handleList`
  - `filterNoInstalledListItems`
  - `filterGUIListItems`
  - `listPackageRow`
  - `listPackageVersion`
  - `packageSource`
  - list 时间格式 helper
- 新建：`internal/cli/config_handler.go`
  - `handleConfig`
  - `handleConfigDoctor`
  - doctor path/env/writable helpers
- 新建：`internal/cli/update_handler.go`
  - `handleUpdate`
  - `handleUpdateCheckTargets`
  - self-update source/result 输出 helpers
- 新建：`internal/cli/sdk_handler.go`
  - `handleSDKInstall`
  - `handleSDKList`
  - `handleSDKRemove`
  - `handleSDKPath`
  - `handleSDKSearch`
  - `handleSDKIndex`
  - `handleSDKConfig`
  - SDK index reporter helpers
- 新建：`internal/cli/query_search_handler.go`
  - `handleQuery`
  - `handleSearch`
- 新建：`internal/cli/cache_handler.go`
  - `cleanOptionsFromCLI`
  - `serveOptionsFromCLI`
  - `handleCacheClean`
  - `handleCacheServe`
  - `formatBytes`
  - `stdinIsTerminal`
- 新建：`internal/cli/outdated_progress.go`
  - `outdatedProgressReporter`
  - `apiCacheNoticeCounter`
  - `suppressOutdatedNetworkNotices`
  - proxy/cache summary 输出 helpers
- 新建：`internal/cli/sudo_warning.go`
  - `warnIfSudoUserConfigLooksSkipped`
  - `lookupUserHome`
  - `fileExists`

测试拆分：

- 从 `internal/cli/service_test.go` 拆出：
  - `install_handler_test.go`
  - `list_handler_test.go`
  - `config_handler_test.go`
  - `update_handler_test.go`
  - `sdk_handler_test.go`
  - `query_search_handler_test.go`
  - `cache_handler_test.go`
- 共享 fake 类型可以暂时保留在 `service_test.go` 或新建 `handler_test_helpers_test.go`。

验收命令：

```bash
go test ./internal/cli -count=1
go test ./...
```

提交建议：

```bash
git commit -m "refactor: split cli handlers by command"
```

## 阶段 2：拆分 install runner

优先级：最高。这里是安装主链路，后续变更频率高。

现状：

- `internal/install/runner.go` 同时负责：
  - `Run()` 主流程
  - 下载 body 和缓存读取
  - progress 创建
  - run-asset
  - GUI installer 启动和 materialize
  - 候选 asset / extracted file 选择
  - 输出路径命名规则
  - 平台/架构 token 匹配
- `internal/install/runner_test.go` 已经接近 2000 行，测试主题高度分散。

建议拆分：

- 保留：`internal/install/runner.go`
  - `RunResult`
  - `Runner`
  - `InstallRunner`
  - `NewRunner`
  - `Run`
  - `extractDownloadedBody`
- 新建：`internal/install/runner_download.go`
  - `downloadBody`
  - `downloadBodyResult`
  - `isInvalidCachedDownload`
  - `parseHTTPTime`
  - `fileModTime`
  - `downloadProgress`
  - `NewDownloadProgress`
  - `downloadProgressLayout`
- 新建：`internal/install/runner_run_asset.go`
  - `validateInstallAction`
  - `materializeRunAsset`
  - `runAsset`
- 新建：`internal/install/runner_installer.go`
  - `confirmLaunchInstaller`
  - `defaultConfirmLaunchInstaller`
  - `launchGUIInstaller`
  - `materializeInstallerFile`
  - `installerMaterializePath`
- 新建：`internal/install/runner_select.go`
  - `resolveVersionFallback`
  - `isAssetSelectionMiss`
  - `selectedFileName`
  - `resolveCandidate`
  - `promptReleaseVersion`
  - `candidatePromptTitle`
  - `uniqueCandidateForName`
  - `normalizedAssetNameHint`
  - `assetBaseMatchesName`
  - `resolveExtractedFile`
- 新建：`internal/install/runner_platform.go`
  - `selectionPlatform`
  - `autoSelectExtractedFile`
  - `autoExtractCurrentPlatformExecutables`
  - `isExecutableForGOOS`
  - `archiveNameMatchesPlatform`
  - `platformTokens`
  - `hasAnyToken`
  - `autoSelectOnlyWindowsExecutable`
  - arch/os token vars and helpers
- 新建：`internal/install/runner_output.go`
  - `effectiveOutput`
  - `outputPath`
  - `firstRenameMap`
  - `renamedOutputName`
  - `resolvedOutputName`
  - `applyPreferredName`
  - executable name heuristics
- 新建：`internal/install/runner_extract.go`
  - `shouldApplyDownloadedModTime`
  - `extractAllTo`

测试拆分：

- `runner_download_test.go`
- `runner_run_asset_test.go`
- `runner_installer_test.go`
- `runner_select_test.go`
- `runner_output_test.go`
- `runner_platform_test.go`
- `runner_test.go` 只保留主流程端到端测试

验收命令：

```bash
go test ./internal/install -count=1
go test ./...
```

提交建议：

```bash
git commit -m "refactor: split install runner responsibilities"
```

## 阶段 3：拆分 client network

优先级：高，但风险高于前两阶段。建议在 CLI 和 runner 拆分稳定后再做。

现状：

- `internal/client/network.go` 同时负责：
  - HTTP client 构建
  - token/auth
  - headers
  - ghproxy fallback
  - API cache 读写
  - resumable/range download
  - progress writer
  - proxy/api cache notice
  - cache path 命名
  - provider metadata 判断

建议拆分：

- 保留：`internal/client/network.go`
  - `Options`
  - `DownloadResult`
  - `HTTPGetterFunc`
  - `Get`
  - `GetWithOptions`
  - `NewHTTPGetter`
- 新建：`internal/client/http_client.go`
  - `newHTTPClient`
  - `ProxyFuncFor`
  - `requestWithOptions`
  - `ProbeLastModified`
- 新建：`internal/client/auth.go`
  - `tokenFrom`
  - `ErrNoToken`
  - `getGitHubToken`
  - `setAuthHeader`
  - `setDefaultHeaders`
- 新建：`internal/client/ghproxy.go`
  - `requestAttemptURLs`
  - ghproxy fallback helpers
- 新建：`internal/client/api_cache.go`
  - `resolvedAPICachePath`
  - `loadAPICacheResponse`
  - `storeAPICacheResponse`
  - provider metadata request 判断
- 新建：`internal/client/download_range.go`
  - `Download`
  - `DownloadWithResult`
  - `effectiveChunkCount`
  - `probeRangeSupport`
  - `splitByteRanges`
  - `downloadRangeChunks`
  - `readRangeBody`
  - `intFromContentLength`
- 新建：`internal/client/progress.go`
  - progress writer 相关接口和 helpers
- 新建：`internal/client/notices.go`
  - proxy notice writer
  - api cache notice writer
  - verbose writer
  - reset test helper
- 新建：`internal/client/cache_path.go`
  - `CacheMeta`
  - `CacheFilePath`
  - `CacheFilePathWithMeta`
  - `APICacheFilePath`
  - cache name/version/hash helpers
- 新建：`internal/client/provider.go`
  - `isGitHubAPIRequest`
  - `isGitHubDownloadRequest`
  - `isGitLabAPIRequest`
  - `isGiteaAPIRequest`
  - `isSourceForgeFilesRequest`
  - `isSourceForgeDownloadRequest`

测试拆分：

- `network_test.go` 保留 `GetWithOptions` 端到端行为
- `download_range_test.go`
- `api_cache_test.go`
- `cache_path_test.go`
- `ghproxy_test.go`
- `auth_test.go`

验收命令：

```bash
go test ./internal/client -count=1
go test ./...
```

提交建议：

```bash
git commit -m "refactor: split client network helpers"
```

## 阶段 4：拆分 app install/update

优先级：中高。建议等底层 runner/client 边界清楚后再做。

### `internal/app/install.go`

建议拆分：

- 保留：`install.go`
  - `Service`
  - `InstallTarget`
  - `DownloadTarget`
- 新建：`install_all.go`
  - `InstallAllPackages`
  - `installAllPackagesConcurrent`
  - batch concurrency helpers
- 新建：`install_resolve.go`
  - `resolveInstallRequest`
  - `resolveInstallRequestWithConfig`
  - `resolveInstallOptions`
  - `resolveInstallOptionsWithConfig`
  - config package matching helpers
- 新建：`install_record.go`
  - `installResolvedTarget`
  - installed store entry building
  - tag/release metadata helpers
  - `extractOptionsMap`
- 新建：`install_options.go`
  - `normalizeExtractionOptions`
  - concurrency validation
  - `boolOpt/stringOpt/stringsOpt/intOpt` 如果仍由 app 多处共享，可单独放 `option_helpers.go`

### `internal/app/update.go`

建议拆分：

- 保留：`update.go`
  - `UpdateService`
  - `UpdatePackage`
  - `UpdateAllPackages`
- 新建：`update_target.go`
  - `findUpdateTarget`
  - installed entry 匹配和 enrich
  - installed update target/options restore
- 新建：`update_candidates.go`
  - `ListUpdateCandidates`
  - `ListUpdateCandidatesForTargets`
  - outdated check 调用
- 新建：`update_batch.go`
  - `UpdateCandidates`
  - `updateCandidatesConcurrent`
- 新建：`update_options.go`
  - `applyUpdateCLIOverrides`
  - `applyConfigNetworkOptions`
  - map option decode helpers

验收命令：

```bash
go test ./internal/app -count=1
go test ./...
```

提交建议：

```bash
git commit -m "refactor: split app install and update services"
```

## 阶段 5：拆分 SDK service

优先级：中。SDK 功能边界相对清楚，适合在 app/runner/client 整理之后做。

现状：

- `internal/sdk/service.go` 同时包含 SDK install/remove/path、index refresh/list/search/fetch、config/path 解析。

建议拆分：

- 保留：`service.go`
  - `Service`
  - 公共类型
  - `effectiveClientOptions`
  - `now`
- 新建：`install_service.go`
  - `Install`
  - `InstallMany`
  - install path / root helpers
  - safe path guard
- 新建：`remove_service.go`
  - `Remove`
- 新建：`path_service.go`
  - `Path`
  - `selectInstalledSDKPath`
- 新建：`index_service.go`
  - `RefreshIndex`
  - `RefreshAllIndexes`
  - `ShowIndex`
  - `ListIndexes`
  - `ClearIndex`
  - `ClearAllIndexes`
  - event emit/count helpers
- 新建：`index_fetch.go`
  - `fetchIndex`
  - `fetchIndexBytes`
  - `fetchIndexPage`
  - pagination helpers
- 新建：`search_service.go`
  - `SearchIndex`
  - search filter/sort/limit helpers
- 新建：`config_resolve.go`
  - `resolveConfig`
  - `resolveVersionAndFile`
  - `resolveInstallPath`
  - `sdkRoot`
  - `sdkBasePath`
  - `targetTemplateBasePrefix`
  - `cacheDir`

验收命令：

```bash
go test ./internal/sdk -count=1
go test ./...
```

提交建议：

```bash
git commit -m "refactor: split sdk service responsibilities"
```

## 不建议立即做的事

- 不建议把 `internal/app`、`internal/install`、`internal/client` 直接拆成新 package。当前第一目标是降低单文件复杂度，同 package 拆分更安全。
- 不建议在同一个 PR/提交里同时重命名大量函数。文件拆分后再看是否还需要符号级重命名。
- 不建议先拆测试 helper。测试 helper 应跟生产代码拆分自然浮现后再整理。
- 不建议顺手改 public behavior、输出文本或错误文本；这些都应单独规划。

## 推荐执行顺序

1. 阶段 1：CLI handler
2. 阶段 2：install runner
3. 阶段 3：client network
4. 阶段 4：app install/update
5. 阶段 5：SDK service

这个顺序的理由：

- CLI handler 拆分风险最低，能快速改善当前最膨胀的生产文件之一。
- install runner 是主链路，先拆清楚后，后续 GUI/install/download 改动会更安全。
- client network 风险较高，但职责混杂最明显，需要在前两个阶段稳定后处理。
- app install/update 和 SDK service 属于编排层，适合在底层边界更清楚后整理。

## 每阶段完成标准

每阶段完成后必须满足：

- 生产代码只是移动和小范围整理，没有行为改动。
- 对应 package 定向测试通过。
- `go test ./...` 通过。
- `git diff --stat` 显示改动集中在当前阶段目标文件。
- 单独提交，提交信息使用 `refactor:` 前缀。
