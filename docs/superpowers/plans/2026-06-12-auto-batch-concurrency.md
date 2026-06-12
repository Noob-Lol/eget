# Auto Batch Concurrency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `batch_concurrency = 0` automatically use bounded package-level concurrency for outdated checks and batch install/update flows.

**Architecture:** Keep the existing scheduler shape and change only the shared effective batch calculation. `0` remains the auto sentinel, `1` remains explicit serial, and values greater than `1` remain explicit worker counts capped by package count and the existing max validation.

**Tech Stack:** Go, existing `internal/app` batch schedulers, existing `github.com/gookit/goutil/testutil/assert` test style.

---

### Task 1: Add Failing Tests For Auto Batch

**Files:**
- Modify: `internal/app/list_outdated_test.go`
- Modify: `internal/app/install_all_test.go`
- Modify: `internal/app/update_batch_test.go`

- [x] **Step 1: Add a failing outdated-check test**

Add a test proving `global.batch_concurrency = 0` runs multiple latest checks concurrently while preserving result order.

- [x] **Step 2: Add a failing install-all test**

Add a test proving `InstallAllPackages` with `BatchConcurrency: 0` automatically runs multiple package installs concurrently.

- [x] **Step 3: Add a failing update-all test**

Add a test proving `UpdateAllPackages` with `BatchConcurrency: 0` automatically runs multiple package updates concurrently.

- [x] **Step 4: Run focused tests and confirm RED**

Run:

```bash
go test ./internal/app -run 'AutoBatch|BatchConcurrency' -v
```

Expected: new auto tests fail because current `effectiveBatchConcurrency(0, total)` returns `1`.

### Task 2: Implement Auto Batch Concurrency

**Files:**
- Modify: `internal/app/install_options.go`

- [x] **Step 1: Add the auto batch default**

Introduce a small default worker count:

```go
const defaultAutoBatchConcurrency = 6
```

- [x] **Step 2: Update effective batch calculation**

Change `effectiveBatchConcurrency` so:

```text
total <= 1 -> 1
value == 1 -> 1
value <= 0 -> min(total, defaultAutoBatchConcurrency)
value > total -> total
otherwise -> value
```

- [x] **Step 3: Run focused tests and confirm GREEN**

Run:

```bash
go test ./internal/app -run 'AutoBatch|BatchConcurrency|InstallAllPackages|UpdateAllPackages|ListOutdatedPackages' -v
```

Expected: PASS.

### Task 3: Update User-Facing Documentation

**Files:**
- Modify: `docs/config.md`
- Modify: `docs/config.zh-CN.md`
- Modify: `docs/architecture.md`
- Modify: `docs/superpowers/specs/2026-05-09-concurrent-downloads-design.md`
- Modify: `docs/superpowers/plans/2026-05-10-concurrent-downloads.md`

- [x] **Step 1: Update config docs**

Document that `batch_concurrency = 0` now auto-selects up to 6 package workers.

- [x] **Step 2: Update architecture/design notes**

Replace stale “auto equals serial” language with the new bounded auto behavior.

### Task 4: Final Verification

**Files:**
- Modify: `docs/superpowers/plans/2026-06-12-auto-batch-concurrency.md`

- [x] **Step 1: Run full tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [x] **Step 2: Run GitNexus change detection**

Run:

```bash
npx gitnexus detect-changes --repo eget
```

Expected: changed symbols are limited to auto batch concurrency tests/docs and `effectiveBatchConcurrency`.

- [x] **Step 3: Mark this plan complete**

Update every checkbox in this file to checked after verification succeeds.
