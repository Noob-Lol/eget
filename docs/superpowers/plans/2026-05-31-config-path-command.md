# Config Path Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `eget config path` / `eget cfg path` to print important local config paths, with optional existence checks.

**Architecture:** Extend the existing config command tree and reuse the existing `ConfigService` as the path resolver. CLI parsing stays in `internal/cli/config_cmd.go`, path target mapping lives in `internal/app/config.go`, and output rendering stays in `internal/cli/handlers.go`.

**Tech Stack:** Go, gookit/gcli, existing `internal/config`, installed store, SDK store, and `github.com/gookit/goutil/testutil/assert`.

---

### Task 1: App Path Resolver

**Files:**
- Modify: `internal/app/config.go`
- Test: `internal/app/config_test.go`

- [x] **Step 1: Add failing tests**

Add tests for default `config_file`, configured `cache_dir`, `sdk_dir`, `bin_dir`, and store file targets.

- [x] **Step 2: Verify RED**

Run: `go test ./internal/app -run TestConfigPathInfo -count=1`

Expected: compile or test failure because `ConfigPathInfo` does not exist.

- [x] **Step 3: Implement resolver**

Add `ConfigPathInfo(target string) (ConfigPathResult, error)` on `ConfigService`, with default target `config_file` and supported targets:
`config_dir`, `config_file`, `env_file`, `bin_dir`, `cache_dir`, `sdk_dir`, `pkg_store_file`, `sdk_store_file`.

- [x] **Step 4: Verify GREEN**

Run: `go test ./internal/app -run TestConfigPathInfo -count=1`

Expected: PASS.

### Task 2: CLI Parsing

**Files:**
- Modify: `internal/cli/config_cmd.go`
- Test: `internal/cli/app_test.go`

- [x] **Step 1: Add failing tests**

Add parser tests for `cfg path`, `cfg path --check cache_dir`, and invalid extra args.

- [x] **Step 2: Verify RED**

Run: `go test ./internal/cli -run TestMain_Config -count=1`

Expected: failure because the `path` subcommand is not registered.

- [x] **Step 3: Implement command binding**

Extend `ConfigOptions` with `Check bool` and `Target string`; add `newConfigPathCmd`.

- [x] **Step 4: Verify GREEN**

Run: `go test ./internal/cli -run TestMain_Config -count=1`

Expected: PASS.

### Task 3: Handler Output And Docs

**Files:**
- Modify: `internal/cli/handlers.go`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Test: `internal/cli/service_test.go`

- [x] **Step 1: Add failing handler tests**

Add tests for plain path output and `path, exists: bool` output.

- [x] **Step 2: Verify RED**

Run: `go test ./internal/cli -run TestHandleConfigPath -count=1`

Expected: failure because `handleConfig` does not handle `path`.

- [x] **Step 3: Implement handler and docs**

Call `ConfigService.ConfigPathInfo`, print just the path normally, and print `<path>, exists: <bool>` when `--check` is set. Document the new command in README files.

- [x] **Step 4: Verify full suite**

Run: `go test ./...`

Expected: PASS.
