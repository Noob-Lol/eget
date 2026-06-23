# Windows Asset Auto Select Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use test-driven-development for this small behavior change. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Windows 平台 release asset 存在 `gnu` 和 `msvc` 两个同平台变体时，自动优先选择 `msvc`，避免不必要的交互选择。

**Architecture:** 保持现有安装主链路不变，只在 `InstallRunner.resolveCandidate()` 进入 prompt 前增加一次平台变体自动选择。平台判断复用 `selectionPlatform()` 和已有平台 token 工具，不能确定唯一候选时继续走原来的 prompt。

**Tech Stack:** Go、现有 `internal/install` 测试套件、`github.com/gookit/goutil/x/assert`。

---

### Task 1: Windows Asset Variant Auto Select

**Files:**
- Modify: `internal/install/runner_select_test.go`
- Modify: `internal/install/runner_select.go`
- Modify: `internal/install/runner_platform.go`

- [x] **Step 1: Write failing test**

在 `internal/install/runner_select_test.go` 添加用例，验证 `windows/amd64` 下 `gnu` + `msvc` 不触发 prompt，并选择 `msvc`。

- [x] **Step 2: Verify test fails**

Run: `go test ./internal/install -run TestResolveCandidateAutoSelectsWindowsMSVCVariant -count=1`

Expected: FAIL，当前实现会调用 prompt。

- [x] **Step 3: Implement minimal auto-select**

在 `resolveCandidate()` 进入历史选择和 prompt 前调用资产候选自动选择。仅当 Windows 候选中唯一匹配 `msvc` 优先级时返回，否则不改变行为。

- [x] **Step 4: Verify install package tests**

Run: `go test ./internal/install -count=1`

Expected: PASS。

- [x] **Step 5: Verify full test suite**

Run: `go test ./...`

Expected: PASS。
