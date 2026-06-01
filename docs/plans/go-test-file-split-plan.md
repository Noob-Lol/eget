# Go 测试文件整理规划

## 背景

上一轮已经把主要膨胀的生产 Go 文件拆分到更清晰的同 package 文件中。本轮继续整理测试文件，目标是降低单个测试文件的阅读和维护成本，不改变测试语义、不改生产行为。

## 进度

- [x] 阶段 0：基线扫描和计划提交
- [x] 阶段 1：拆分 CLI parser 测试
- [x] 阶段 2：拆分 app service 测试
- [x] 阶段 3：拆分 install package 测试
- [x] 阶段 4：拆分 config/source 等剩余热点测试

## 当前热点

| 文件 | 行数 | 判断 |
| --- | ---: | --- |
| `internal/cli/app_test.go` | 1490 | CLI parser 和 command route 测试混在一起，适合按 command group 拆分 |
| `internal/app/install_test.go` | 1419 | install/download/add/install-all/resolve/options 测试混在一起 |
| `internal/install/service_test.go` | 872 | finder/detector/verifier/extractor 测试混在一起 |
| `internal/install/runner_download_test.go` | 692 | 下载缓存、range、resume 行为测试较集中但仍偏大 |
| `internal/app/update_test.go` | 639 | update package/all/candidates 混在一起 |
| `internal/install/defaults_test.go` | 604 | 默认选项和环境探测测试偏大 |
| `internal/app/list_test.go` | 584 | list/outdated/check progress 混在一起 |
| `internal/config/loader_test.go` | 562 | loader 场景较多，可按配置来源拆分 |

## 拆分原则

1. 只做测试文件同 package 拆分，不改测试断言和生产逻辑。
2. 共享 fake/helper 可以保留在原文件，或移动到 `*_test_helpers_test.go`。
3. 每个阶段完成后运行定向测试和 `go test ./...`。
4. 每个阶段独立提交，提交信息使用 `test:` 或 `refactor:` 前缀。
5. 不为拆分而重写测试结构；优先移动完整顶层声明块。

## 阶段 1：拆分 CLI parser 测试

目标文件：`internal/cli/app_test.go`

建议拆分：

- `app_test.go`：保留 build info、no subcommand、全局状态隔离等基础测试
- `app_install_test.go`：install/download/add flag binding
- `app_config_test.go`：config command routing
- `app_sdk_test.go`：sdk command routing
- `app_list_show_test.go`：list/show/info command routing
- `app_query_search_test.go`：query/search routing
- `app_update_uninstall_test.go`：update/uninstall routing
- `app_cache_test.go`：cache clean/serve routing

验收：

```bash
go test ./internal/cli -count=1
go test ./...
```

## 阶段 2：拆分 app service 测试

目标文件：

- `internal/app/install_test.go`
- `internal/app/update_test.go`
- `internal/app/list_test.go`

建议拆分：

- `install_record_test.go`
- `download_target_test.go`
- `install_add_test.go`
- `install_all_test.go`
- `install_options_test.go`
- `update_package_test.go`
- `update_candidates_test.go`
- `update_batch_test.go`
- `list_outdated_test.go`

验收：

```bash
go test ./internal/app -count=1
go test ./...
```

## 阶段 3：拆分 install package 测试

目标文件：

- `internal/install/service_test.go`
- `internal/install/runner_download_test.go`
- `internal/install/defaults_test.go`

建议拆分：

- `service_finder_test.go`
- `service_detector_test.go`
- `service_verifier_test.go`
- `service_extractor_test.go`
- `runner_download_cache_test.go`
- `runner_download_range_test.go`
- `defaults_platform_test.go`
- `defaults_options_test.go`

验收：

```bash
go test ./internal/install -count=1
go test ./...
```

## 阶段 4：拆分剩余热点测试

目标文件按实际扫描结果处理：

- `internal/config/loader_test.go`
- `internal/config/gookit_test.go`
- `internal/config/merge_test.go`
- `internal/source/sourceforge/finder_test.go`
- 其他仍超过 500 行的测试文件

验收：

```bash
go test ./...
```
