# Uninstall Purge Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use test-driven-development for this behavior change. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 支持 `eget rm --purge NAME` 在卸载 package 后同时删除 `[packages.NAME]` 配置。

**Architecture:** 保留现有 `Uninstall(target)` 默认行为，新增带 options 的卸载路径供 CLI `--purge` 调用。配置清理由 app 层完成，优先按 package name 删除，repo 目标仅在唯一匹配配置时删除，避免误删。

**Tech Stack:** Go、`github.com/gookit/gcli/v3`、现有 `internal/app` 和 `internal/cli` 测试套件、`github.com/gookit/goutil/x/assert`。

---

### Task 1: Add Purge Option

**Files:**
- Modify: `internal/app/uninstall.go`
- Modify: `internal/app/uninstall_test.go`
- Modify: `internal/cli/uninstall_cmd.go`
- Modify: `internal/cli/uninstall_handler.go`
- Modify: `internal/cli/uninstall_handler_test.go`
- Modify: `internal/cli/app_test.go`
- Modify: `internal/cli/wiring.go`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

- [x] **Step 1: Write failing app test**

验证 `UninstallWithOptions(..., Purge:true)` 会删除 package 配置并保存。

- [x] **Step 2: Write failing CLI tests**

验证 `rm --purge NAME` 能解析 `Purge:true`，handler 会把 purge 传给 app service。

- [x] **Step 3: Implement app purge**

新增 app 层 options、配置保存依赖和删除逻辑，默认 `Uninstall()` 行为不变。

- [x] **Step 4: Implement CLI flag**

新增 `--purge` flag 并在 handler 调用 app 层 options。

- [x] **Step 5: Update docs**

更新 README 中 uninstall 说明。

- [x] **Step 6: Verify**

运行局部测试、`go test ./... -count=1`、`git diff --check` 和 GitNexus detect-changes。
