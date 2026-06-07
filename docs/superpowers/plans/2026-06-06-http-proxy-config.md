# HTTP Proxy Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 legacy `global.proxy_url` 升级为独立 `[http_proxy]` 配置块，并让 install/update/download/SDK/GitHub API 请求共享同一套代理解析和 exclude 规则。

**Architecture:** 新增配置模型和 `internal/config` 层 proxy resolver，输出统一的 `ProxyConfig`。`internal/client` 的 proxy func 改为 request-aware，按请求 host 判断 exclude；`internal/app` 和 `internal/cli` 只负责把 resolver 结果透传到 install/sdk/client options。

**Tech Stack:** Go, TOML/gookit config, existing `internal/config`, `internal/client`, `internal/install`, `internal/app`, SDK service, `github.com/gookit/goutil/testutil/assert`.

---

## Design Reference

本计划基于设计文档：

- `docs/superpowers/specs/2026-06-06-http-proxy-config-design.md`

已确认约束：

- 不要求 `eget cfg path` 支持 `http_proxy.url`、`http_proxy.enable`、`http_proxy.exclude`。
- 不支持 `exclude = ["*"]` 或 `NO_PROXY=*` 作为禁用代理语义。
- 保留 legacy `global.proxy_url` fallback。
- 保留 repo/package/CLI 级 `proxy_url` override。

## Target Behavior

新配置：

```toml
[http_proxy]
enable = true
url = "http://127.0.0.1:10801"
exclude = ["mydev.com", "*.corp.local", "10.0.0.0/8"]
```

核心语义：

- `enable = false` 或 `url = ""` 禁用配置代理。
- `enable` 未配置且 `url` 非空时默认启用。
- `[http_proxy]` 存在时优先于 legacy `global.proxy_url`。
- 无 `[http_proxy]` 时继续 fallback `global.proxy_url`。
- `--no-proxy` 禁用配置代理。
- `NO_PROXY=1|true|yes|on` 禁用配置代理。
- `NO_PROXY=mydev.com,*.corp.local` 作为额外 exclude。
- exclude 命中时，不使用代理，也不输出 proxy notice。

## File Structure

### New Files

- `internal/config/proxy.go`
  - Defines `ProxyConfig`, `ProxyResolveOptions`, `ResolveHTTPProxy`.
  - Parses `NO_PROXY` as disable flag or exclude list.
  - Provides `ProxyExcluded(host string, rules []string) bool`.

- `internal/config/proxy_test.go`
  - Unit tests for resolver, NO_PROXY parsing, exclude matching.

### Modified Files

- `internal/config/model.go`
  - Add `HTTPProxySection`.
  - Add `HTTPProxy HTTPProxySection` to `File`.

- `internal/config/gookit.go`
  - Decode/encode `[http_proxy]`.
  - Add `http_proxy` to reserved root keys.
  - Normalize `http_proxy.url`, `http_proxy.enable`, `http_proxy.exclude` in `SetByPath`.

- `internal/config/gookit_test.go`
  - Round-trip tests for `[http_proxy]`.

- `internal/config/loader_dotenv_test.go`
  - Ensure env expansion still works in `http_proxy.url`.

- `internal/config/merge.go`
  - Keep existing repo/package/CLI `proxy_url` merge behavior.
  - Add `ProxyExclude []string` to `Merged` only if implementation uses merge-level propagation.

- `internal/config/merge_test.go`
  - Ensure package/repo/CLI `proxy_url` behavior remains unchanged.

- `internal/client/network.go`
  - Add `ProxyExclude []string` to `client.Options`.

- `internal/client/http_client.go`
  - Change `ProxyFuncFor(proxyURL string)` to `ProxyFuncFor(proxyURL string, exclude []string)`.
  - Use request host to skip proxy when excluded.

- `internal/client/notices.go`
  - Ensure proxy notice prints only when proxy will be used for that request.
  - Optionally change wording from `proxy_url` to `http_proxy`.

- `internal/client/notices_test.go`
  - Update expected wording or add exclude notice tests.

- `internal/client/download_file.go`
  - Pass `ProxyExclude` into proxy notice decision and HTTP client path.

- `internal/client/download_range.go`
  - Same as `download_file.go`.

- `internal/install/options.go`
  - Add `ProxyExclude []string` to `install.Options`.

- `internal/install/network.go`
  - Propagate `ProxyExclude` into `client.Options`.

- `internal/install/runner_network_test.go`
  - Verify proxy URL and exclude propagation.

- `internal/app/install_resolve.go`
  - Resolve global proxy via `config.ResolveHTTPProxy`.
  - Preserve repo/package/CLI `proxy_url` override.
  - Set `install.Options.ProxyURL` and `install.Options.ProxyExclude`.

- `internal/app/update_options.go`
  - Apply same resolver in update network option path.

- `internal/app/sdk.go`
  - Use resolver for SDK client options.

- `internal/app/install_config_test.go`
  - Tests for install path using `[http_proxy]`, legacy fallback, `--no-proxy`, `NO_PROXY`, exclude.

- `internal/app/update_batch_test.go`
  - Tests for update path using `[http_proxy]` and disabling behavior.

- `internal/app/sdk_test.go`
  - Tests for SDK client options using `[http_proxy]` and exclude.

- `internal/cli/options.go`
  - Replace direct `cfg.Global.ProxyURL` read in `applyGlobalNetworkConfig` with resolver.
  - Preserve `s.applyGlobalFlags` behavior for `--no-proxy`.

- `internal/cli/install_handler_test.go`
  - Update global network tests for new config.

- `internal/cli/list_handler_test.go`
  - Update outdated proxy notice tests if wording changes to `http_proxy`.

- `internal/cli/outdated_progress.go`
  - Update notice wording if needed.

- `internal/app/config.go`
  - Config init should include `[http_proxy]` defaults if project config docs expect it.

- `internal/app/config_test.go`
  - Default config tests for `[http_proxy]`.

- `docs/config.md`
  - Document `[http_proxy]` and legacy `global.proxy_url`.

- `docs/config.zh-CN.md`
  - Chinese documentation for `[http_proxy]`.

- `docs/example.eget.toml`
  - Add `[http_proxy]` example.

---

### Task 1: Config Model And TOML Round Trip

**Files:**
- Modify: `internal/config/model.go`
- Modify: `internal/config/gookit.go`
- Modify: `internal/config/gookit_test.go`
- Modify: `internal/config/loader_dotenv_test.go`
- Modify: `internal/app/config.go`
- Modify: `internal/app/config_test.go`

- [x] **Step 1: Run impact analysis**

Run before editing symbols:

```bash
npx gitnexus impact File --repo eget --direction upstream
npx gitnexus impact decodeConfigFile --repo eget --direction upstream
npx gitnexus impact encodeConfigFile --repo eget --direction upstream
npx gitnexus impact SetByPath --repo eget --direction upstream
```

Expected: LOW or known config-surface risk. If HIGH/CRITICAL, report before editing.

- [x] **Step 2: Write failing config round-trip tests**

In `internal/config/gookit_test.go`, add:

```go
func TestHTTPProxySectionRoundTrip(t *testing.T) {
    proxyURL := "http://127.0.0.1:10801"
    enabled := true
    cfg := NewFile()
    cfg.HTTPProxy.URL = &proxyURL
    cfg.HTTPProxy.Enable = &enabled
    cfg.HTTPProxy.Exclude = []string{"mydev.com", "*.corp.local", "10.0.0.0/8"}

    dumped, err := dumpConfigString(cfg)
    assert.NoErr(t, err)
    assert.Contains(t, dumped, "[http_proxy]")
    assert.Contains(t, dumped, `url = "http://127.0.0.1:10801"`)
    assert.Contains(t, dumped, "enable = true")
    assert.Contains(t, dumped, "exclude")

    loaded := newConfigManager()
    assert.NoErr(t, loaded.LoadStrings(configutil.Toml, dumped))
    decoded, err := decodeConfigFile(loaded)
    assert.NoErr(t, err)
    assert.Eq(t, proxyURL, *decoded.HTTPProxy.URL)
    assert.True(t, *decoded.HTTPProxy.Enable)
    assert.Eq(t, []string{"mydev.com", "*.corp.local", "10.0.0.0/8"}, decoded.HTTPProxy.Exclude)
}
```

Adjust helper names if existing tests use a different load helper.

- [x] **Step 3: Write failing SetByPath tests**

In `internal/config/gookit_test.go`, add:

```go
func TestSetByPathHTTPProxyFields(t *testing.T) {
    cfg := NewFile()

    assert.NoErr(t, SetByPath(cfg, "http_proxy.url", "127.0.0.1:10801"))
    assert.Eq(t, "http://127.0.0.1:10801", *cfg.HTTPProxy.URL)

    assert.NoErr(t, SetByPath(cfg, "http_proxy.enable", "false"))
    assert.False(t, *cfg.HTTPProxy.Enable)

    assert.NoErr(t, SetByPath(cfg, "http_proxy.exclude", "mydev.com,*.corp.local"))
    assert.Eq(t, []string{"mydev.com", "*.corp.local"}, cfg.HTTPProxy.Exclude)
}
```

- [x] **Step 4: Write failing dotenv expansion test**

In `internal/config/loader_dotenv_test.go`, extend or add:

```go
func TestLoadHTTPProxyURLEnvExpansion(t *testing.T) {
    t.Setenv("PROXY_URL", "http://127.0.0.1:10801")
    cfg := mustLoadConfigFromString(t, `
[http_proxy]
url = "${PROXY_URL}"
`)
    assert.Eq(t, "http://127.0.0.1:10801", *cfg.HTTPProxy.URL)
}
```

Use the actual helper already present in this test file.

- [x] **Step 5: Verify RED**

Run:

```bash
go test ./internal/config -run "TestHTTPProxy|TestSetByPathHTTPProxy|TestLoadHTTPProxy" -count=1
```

Expected: compile failure because `HTTPProxy` / `HTTPProxySection` does not exist or tests fail because the config block is not decoded.

- [x] **Step 6: Implement config model**

In `internal/config/model.go`, add:

```go
type HTTPProxySection struct {
    Enable  *bool    `toml:"enable" mapstructure:"enable"`
    URL     *string  `toml:"url" mapstructure:"url"`
    Exclude []string `toml:"exclude" mapstructure:"exclude"`
}
```

Add to `File`:

```go
HTTPProxy HTTPProxySection `toml:"http_proxy" mapstructure:"http_proxy"`
```

- [x] **Step 7: Implement decode/encode**

In `internal/config/gookit.go`:

Add decode:

```go
if err := cfg.MapOnExists("http_proxy", &conf.HTTPProxy); err != nil {
    return nil, err
}
```

Add encode map entry:

```go
"http_proxy": httpProxyToMap(file.HTTPProxy),
```

Add helper:

```go
func httpProxyToMap(section HTTPProxySection) map[string]any {
    data := map[string]any{}
    if section.Enable != nil {
        data["enable"] = *section.Enable
    }
    if section.URL != nil {
        data["url"] = *section.URL
    }
    if len(section.Exclude) > 0 {
        data["exclude"] = append([]string(nil), section.Exclude...)
    }
    return data
}
```

Add reserved root key:

```go
case "global", "http_proxy", "api_cache", "ghproxy", "cache_mirror", "packages", "sdk":
```

- [x] **Step 8: Normalize `http_proxy` path values**

Update `normalizePathValue`:

```go
switch pathFieldName(key) {
case "proxy_url", "url":
    if strings.HasPrefix(key, "http_proxy.") && text != "" && !strings.HasPrefix(text, "http") {
        text = "http://" + text
    }
    if pathFieldName(key) == "proxy_url" && text != "" && !strings.HasPrefix(text, "http") {
        text = "http://" + text
    }
    return text, true
case "exclude":
    if strings.HasPrefix(key, "http_proxy.") {
        return splitAndTrim(text), true
    }
```

Keep existing bool normalization for `enable`.

- [x] **Step 9: Update default config init**

In `internal/app/config.go`, update `NewDefaultConfigFile` or equivalent initializer to include an empty `[http_proxy]` only if current config init writes explicit empty sections. If current style avoids empty optional sections, leave it out and document that `[http_proxy]` is user-added.

Update `internal/app/config_test.go` accordingly:

```go
if cfg.HTTPProxy.URL != nil && *cfg.HTTPProxy.URL != "" {
    t.Fatalf("expected default http_proxy.url to be empty, got %#v", cfg.HTTPProxy.URL)
}
```

- [x] **Step 10: Verify GREEN**

Run:

```bash
go test ./internal/config -run "TestHTTPProxy|TestSetByPathHTTPProxy|TestLoadHTTPProxy" -count=1
go test ./internal/app -run TestConfig -count=1
```

Expected: PASS.

- [x] **Step 11: Commit**

```bash
git add internal/config/model.go internal/config/gookit.go internal/config/gookit_test.go internal/config/loader_dotenv_test.go internal/app/config.go internal/app/config_test.go
git commit -m "feat: add http_proxy config model"
```

---

### Task 2: Proxy Resolver And Exclude Matching

**Files:**
- Create: `internal/config/proxy.go`
- Create: `internal/config/proxy_test.go`
- Modify: `internal/util/proxy.go`

- [x] **Step 1: Run impact analysis**

```bash
npx gitnexus impact GlobalProxyDisabled --repo eget --direction upstream
```

Expected: LOW/MEDIUM. If HIGH/CRITICAL, report before editing.

- [x] **Step 2: Write failing resolver tests**

Create `internal/config/proxy_test.go` with tests:

```go
func TestResolveHTTPProxyUsesHTTPProxyURLByDefault(t *testing.T) {
    url := "http://127.0.0.1:10801"
    cfg := NewFile()
    cfg.HTTPProxy.URL = &url

    got := ResolveHTTPProxy(cfg, ProxyResolveOptions{})

    assert.True(t, got.Enabled)
    assert.Eq(t, url, got.URL)
}

func TestResolveHTTPProxyEnableFalseDisables(t *testing.T) {
    url := "http://127.0.0.1:10801"
    enabled := false
    cfg := NewFile()
    cfg.HTTPProxy.URL = &url
    cfg.HTTPProxy.Enable = &enabled

    got := ResolveHTTPProxy(cfg, ProxyResolveOptions{})

    assert.False(t, got.Enabled)
    assert.Eq(t, "", got.URL)
}

func TestResolveHTTPProxyFallsBackToLegacyGlobalProxyURL(t *testing.T) {
    legacy := "http://127.0.0.1:10801"
    cfg := NewFile()
    cfg.Global.ProxyURL = &legacy

    got := ResolveHTTPProxy(cfg, ProxyResolveOptions{})

    assert.True(t, got.Enabled)
    assert.Eq(t, legacy, got.URL)
}

func TestResolveHTTPProxyPrefersNewBlockOverLegacy(t *testing.T) {
    legacy := "http://127.0.0.1:10801"
    current := "http://127.0.0.1:10802"
    cfg := NewFile()
    cfg.Global.ProxyURL = &legacy
    cfg.HTTPProxy.URL = &current

    got := ResolveHTTPProxy(cfg, ProxyResolveOptions{})

    assert.Eq(t, current, got.URL)
}

func TestResolveHTTPProxyNoProxyDisables(t *testing.T) {
    url := "http://127.0.0.1:10801"
    cfg := NewFile()
    cfg.HTTPProxy.URL = &url

    got := ResolveHTTPProxy(cfg, ProxyResolveOptions{NoProxy: true})

    assert.False(t, got.Enabled)
    assert.Eq(t, "", got.URL)
}

func TestResolveHTTPProxyEnvDisableValues(t *testing.T) {
    url := "http://127.0.0.1:10801"
    for _, value := range []string{"1", "true", "yes", "on"} {
        cfg := NewFile()
        cfg.HTTPProxy.URL = &url
        got := ResolveHTTPProxy(cfg, ProxyResolveOptions{EnvNoProxy: value})
        assert.False(t, got.Enabled)
        assert.Eq(t, "", got.URL)
    }
}

func TestResolveHTTPProxyMergesExcludeRules(t *testing.T) {
    url := "http://127.0.0.1:10801"
    cfg := NewFile()
    cfg.HTTPProxy.URL = &url
    cfg.HTTPProxy.Exclude = []string{"mydev.com"}

    got := ResolveHTTPProxy(cfg, ProxyResolveOptions{EnvNoProxy: "*.corp.local,10.0.0.0/8"})

    assert.True(t, got.Enabled)
    assert.Eq(t, []string{"mydev.com", "*.corp.local", "10.0.0.0/8"}, got.Exclude)
}
```

- [x] **Step 3: Write failing exclude tests**

In `internal/config/proxy_test.go`, add:

```go
func TestProxyExcludedMatchesRules(t *testing.T) {
    rules := []string{
        "localhost",
        "127.0.0.1",
        "mydev.com",
        "*.corp.local",
        ".internal.local",
        "10.0.0.0/8",
        "port.example.com:8080",
    }

    cases := []struct {
        host string
        want bool
    }{
        {"localhost", true},
        {"127.0.0.1", true},
        {"mydev.com", true},
        {"api.mydev.com", true},
        {"api.corp.local", true},
        {"corp.local", false},
        {"api.internal.local", true},
        {"10.2.3.4", true},
        {"port.example.com:8080", true},
        {"port.example.com:9090", false},
        {"github.com", false},
    }

    for _, tt := range cases {
        t.Run(tt.host, func(t *testing.T) {
            assert.Eq(t, tt.want, ProxyExcluded(tt.host, rules))
        })
    }
}

func TestProxyExcludedIgnoresStarRule(t *testing.T) {
    assert.False(t, ProxyExcluded("github.com", []string{"*"}))
}
```

- [x] **Step 4: Verify RED**

Run:

```bash
go test ./internal/config -run "TestResolveHTTPProxy|TestProxyExcluded" -count=1
```

Expected: compile failure because resolver functions do not exist.

- [x] **Step 5: Implement resolver types**

Create `internal/config/proxy.go`:

```go
package config

import (
    "net"
    "strconv"
    "strings"
)

type ProxyConfig struct {
    Enabled bool
    URL     string
    Exclude []string
}

type ProxyResolveOptions struct {
    NoProxy    bool
    EnvNoProxy string
    OverrideURL string
    PackageURL string
    RepoURL    string
}
```

- [x] **Step 6: Implement `ResolveHTTPProxy`**

In `internal/config/proxy.go`:

```go
func ResolveHTTPProxy(cfg *File, opts ProxyResolveOptions) ProxyConfig {
    if opts.NoProxy || noProxyEnvDisables(opts.EnvNoProxy) {
        return ProxyConfig{}
    }

    exclude := ParseNoProxyExclude(opts.EnvNoProxy)
    if cfg != nil {
        exclude = append(exclude, cfg.HTTPProxy.Exclude...)
    }

    url := firstNonEmptyProxyURL(opts.OverrideURL, opts.PackageURL, opts.RepoURL)
    if url == "" && cfg != nil {
        if httpProxyConfigured(cfg.HTTPProxy) {
            if cfg.HTTPProxy.Enable != nil && !*cfg.HTTPProxy.Enable {
                return ProxyConfig{Exclude: exclude}
            }
            url = derefString(cfg.HTTPProxy.URL)
        } else {
            url = derefString(cfg.Global.ProxyURL)
        }
    }

    if strings.TrimSpace(url) == "" {
        return ProxyConfig{Exclude: exclude}
    }
    return ProxyConfig{Enabled: true, URL: strings.TrimSpace(url), Exclude: exclude}
}
```

Add small private helpers:

```go
func httpProxyConfigured(section HTTPProxySection) bool {
    return section.Enable != nil || strings.TrimSpace(derefString(section.URL)) != "" || len(section.Exclude) > 0
}

func derefString(value *string) string {
    if value == nil {
        return ""
    }
    return *value
}

func firstNonEmptyProxyURL(values ...string) string {
    for _, value := range values {
        value = strings.TrimSpace(value)
        if value != "" {
            return value
        }
    }
    return ""
}
```

- [x] **Step 7: Implement NO_PROXY parsing**

In `internal/config/proxy.go`:

```go
func noProxyEnvDisables(value string) bool {
    switch strings.ToLower(strings.TrimSpace(value)) {
    case "1", "true", "yes", "on":
        return true
    default:
        return false
    }
}

func ParseNoProxyExclude(value string) []string {
    if noProxyEnvDisables(value) {
        return nil
    }
    parts := strings.Split(value, ",")
    out := make([]string, 0, len(parts))
    for _, part := range parts {
        part = strings.TrimSpace(part)
        if part == "" || part == "*" {
            continue
        }
        out = append(out, part)
    }
    return out
}
```

- [x] **Step 8: Implement exclude matching**

In `internal/config/proxy.go`:

```go
func ProxyExcluded(host string, rules []string) bool {
    hostName, hostPort := splitHostPort(strings.ToLower(strings.TrimSpace(host)))
    if hostName == "" {
        return false
    }
    for _, rule := range rules {
        rule = strings.ToLower(strings.TrimSpace(rule))
        if rule == "" || rule == "*" {
            continue
        }
        ruleHost, rulePort := splitHostPort(rule)
        if rulePort != "" && rulePort != hostPort {
            continue
        }
        if _, ipNet, err := net.ParseCIDR(ruleHost); err == nil {
            if ip := net.ParseIP(hostName); ip != nil && ipNet.Contains(ip) {
                return true
            }
            continue
        }
        if ruleHost == hostName {
            return true
        }
        if strings.HasPrefix(ruleHost, "*.") {
            suffix := strings.TrimPrefix(ruleHost, "*.")
            if strings.HasSuffix(hostName, "."+suffix) {
                return true
            }
            continue
        }
        if strings.HasPrefix(ruleHost, ".") {
            suffix := strings.TrimPrefix(ruleHost, ".")
            if strings.HasSuffix(hostName, "."+suffix) {
                return true
            }
            continue
        }
        if strings.HasSuffix(hostName, "."+ruleHost) {
            return true
        }
    }
    return false
}
```

Add:

```go
func splitHostPort(value string) (string, string) {
    if host, port, err := net.SplitHostPort(value); err == nil {
        return strings.Trim(host, "[]"), port
    }
    if idx := strings.LastIndexByte(value, ':'); idx > 0 && !strings.Contains(value[:idx], ":") {
        if _, err := strconv.Atoi(value[idx+1:]); err == nil {
            return value[:idx], value[idx+1:]
        }
    }
    return strings.Trim(value, "[]"), ""
}
```

- [x] **Step 9: Update `util.GlobalProxyDisabled` compatibility**

Keep the existing function signature for current callers:

```go
func GlobalProxyDisabled(noProxy bool) bool {
    return GlobalProxyDisabledWithEnv(noProxy, os.Getenv("NO_PROXY"))
}
```

Add:

```go
func GlobalProxyDisabledWithEnv(noProxy bool, envNoProxy string) bool {
    if noProxy {
        return true
    }
    switch strings.ToLower(strings.TrimSpace(envNoProxy)) {
    case "1", "true", "yes", "on":
        return true
    default:
        return false
    }
}
```

This changes old behavior intentionally: `NO_PROXY=mydev.com` no longer disables all config proxy; it becomes an exclude rule through `config.ParseNoProxyExclude`.

- [x] **Step 10: Verify GREEN**

Run:

```bash
go test ./internal/config -run "TestResolveHTTPProxy|TestProxyExcluded" -count=1
go test ./internal/util -run TestGlobalProxy -count=1
```

Expected: PASS. If `internal/util` has no tests, `go test ./internal/util` should pass.

- [x] **Step 11: Commit**

```bash
git add internal/config/proxy.go internal/config/proxy_test.go internal/util/proxy.go
git commit -m "feat: resolve http_proxy config"
```

---

### Task 3: Request-Aware Proxy In HTTP Client

**Files:**
- Modify: `internal/client/network.go`
- Modify: `internal/client/http_client.go`
- Modify: `internal/client/notices.go`
- Modify: `internal/client/notices_test.go`
- Modify: `internal/client/download_file.go`
- Modify: `internal/client/download_range.go`
- Modify: `internal/install/options.go`
- Modify: `internal/install/network.go`
- Modify: `internal/install/runner_network_test.go`

- [x] **Step 1: Run impact analysis**

```bash
npx gitnexus impact ProxyFuncFor --repo eget --direction upstream
npx gitnexus impact Options --repo eget --direction upstream
npx gitnexus impact NewHTTPGetter --repo eget --direction upstream
```

If `Options` is ambiguous, use the concrete uid returned for `internal/client/network.go`.

- [x] **Step 2: Write failing client proxy tests**

In `internal/install/runner_network_test.go` or `internal/client/http_client_test.go`, add:

```go
func TestProxyFuncForSkipsExcludedHost(t *testing.T) {
    proxyFunc, err := client.ProxyFuncFor("http://127.0.0.1:7890", []string{"github.com"})
    assert.NoErr(t, err)

    req := httptest.NewRequest("GET", "https://github.com/owner/repo", nil)
    got, err := proxyFunc(req)
    assert.NoErr(t, err)
    assert.Eq(t, (*url.URL)(nil), got)

    req = httptest.NewRequest("GET", "https://api.github.com/repos/owner/repo", nil)
    got, err = proxyFunc(req)
    assert.NoErr(t, err)
    assert.Eq(t, (*url.URL)(nil), got)

    req = httptest.NewRequest("GET", "https://example.com/file.zip", nil)
    got, err = proxyFunc(req)
    assert.NoErr(t, err)
    assert.Eq(t, "http://127.0.0.1:7890", got.String())
}
```

Use package imports already present in the selected test file.

- [x] **Step 3: Write failing proxy notice test**

In `internal/client/notices_test.go`, add a test that calls the notice helper through `GetWithOptions` or a small exported/internal helper and asserts:

```go
assert.NotContains(t, got, "http_proxy for GitHub API request")
```

when URL host is excluded, and:

```go
assert.Contains(t, got, "http_proxy for GitHub API request")
```

when not excluded.

- [x] **Step 4: Verify RED**

Run:

```bash
go test ./internal/client -run "TestProxy|Test.*Notice" -count=1
go test ./internal/install -run TestProxy -count=1
```

Expected: compile failure because `ProxyFuncFor` does not accept exclude, or test failure because proxy is not skipped.

- [x] **Step 5: Extend client options**

In `internal/client/network.go`:

```go
type Options struct {
    ProxyURL     string
    ProxyExclude []string
    // existing fields...
}
```

- [x] **Step 6: Update `ProxyFuncFor` signature**

In `internal/client/http_client.go`:

```go
func newHTTPClient(opts Options) (*http.Client, error) {
    proxyFunc, err := ProxyFuncFor(opts.ProxyURL, opts.ProxyExclude)
    if err != nil {
        return nil, err
    }
    // ...
}

func ProxyFuncFor(proxyURL string, exclude []string) (func(*http.Request) (*url.URL, error), error) {
    if proxyURL == "" {
        proxyFunc := httpproxy.FromEnvironment().ProxyFunc()
        return func(req *http.Request) (*url.URL, error) {
            return proxyFunc(req.URL)
        }, nil
    }
    parsed, err := url.Parse(proxyURL)
    if err != nil {
        return nil, fmt.Errorf("invalid proxy_url %q: %w", proxyURL, err)
    }
    return func(req *http.Request) (*url.URL, error) {
        if config.ProxyExcluded(req.URL.Host, exclude) {
            return nil, nil
        }
        return parsed, nil
    }, nil
}
```

Import `internal/config` in `internal/client/http_client.go`.

- [x] **Step 7: Update all `ProxyFuncFor` callers**

Search:

```bash
rg "ProxyFuncFor" internal
```

Update calls:

```go
client.ProxyFuncFor(opts.ProxyURL, opts.ProxyExclude)
```

or equivalent local package call:

```go
ProxyFuncFor(opts.ProxyURL, opts.ProxyExclude)
```

- [x] **Step 8: Update proxy notice behavior**

In `internal/client/notices.go`, add:

```go
func shouldUseConfiguredProxy(rawURL, proxyURL string, exclude []string) bool {
    if strings.TrimSpace(proxyURL) == "" {
        return false
    }
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return false
    }
    return !config.ProxyExcluded(parsed.Host, exclude)
}
```

Change notice calls in `GetWithOptions`, `DownloadFile`, and range download path from:

```go
printProxyNotice("GitHub API request", opts.ProxyURL)
```

to:

```go
if shouldUseConfiguredProxy(rawURL, opts.ProxyURL, opts.ProxyExclude) {
    printProxyNotice("GitHub API request", opts.ProxyURL)
}
```

If updating wording, change the notice body to:

```go
ccolor.Fprintf(proxyNoticeWriter, " - Using <ylw>http_proxy for %s</>: %s\n", kind, proxyURL)
```

- [x] **Step 9: Extend install options**

In `internal/install/options.go`:

```go
ProxyExclude []string
```

In `internal/install/network.go`, include:

```go
ProxyExclude: append([]string(nil), opts.ProxyExclude...),
```

when building `client.Options`.

- [x] **Step 10: Verify GREEN**

Run:

```bash
go test ./internal/client -run "TestProxy|Test.*Notice" -count=1
go test ./internal/install -run "Test.*Proxy|TestNewHTTPGetter" -count=1
go test ./internal/client ./internal/install
```

Expected: PASS.

- [x] **Step 11: Commit**

```bash
git add internal/client/network.go internal/client/http_client.go internal/client/notices.go internal/client/notices_test.go internal/client/download_file.go internal/client/download_range.go internal/install/options.go internal/install/network.go internal/install/runner_network_test.go
git commit -m "feat: apply proxy excludes to http client"
```

---

### Task 4: Install, Download, Update, And Outdated Integration

**Files:**
- Modify: `internal/app/install_resolve.go`
- Modify: `internal/app/update_options.go`
- Modify: `internal/cli/options.go`
- Modify: `internal/cli/outdated_progress.go`
- Modify: `internal/app/install_config_test.go`
- Modify: `internal/app/update_batch_test.go`
- Modify: `internal/cli/install_handler_test.go`
- Modify: `internal/cli/list_handler_test.go`

- [x] **Step 1: Run impact analysis**

```bash
npx gitnexus impact resolveInstallOptionsWithConfig --repo eget --direction upstream
npx gitnexus impact applyConfigNetworkOptions --repo eget --direction upstream
npx gitnexus impact applyGlobalNetworkConfig --repo eget --direction upstream
```

Expected: LOW/MEDIUM. If HIGH/CRITICAL, report before editing.

- [x] **Step 2: Write failing install config tests**

In `internal/app/install_config_test.go`, add or update tests:

```go
func TestInstallTargetUsesHTTPProxyConfig(t *testing.T) {
    t.Setenv("NO_PROXY", "")
    runner := &fakeRunner{}
    svc := Service{
        Runner: runner,
        LoadConfig: func() (*cfgpkg.File, error) {
            cfg := cfgpkg.NewFile()
            proxyURL := "http://127.0.0.1:10801"
            cfg.HTTPProxy.URL = &proxyURL
            cfg.HTTPProxy.Exclude = []string{"mydev.com"}
            cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
            return cfg, nil
        },
    }

    _, err := svc.InstallTarget("fzf", install.Options{})
    assert.NoErr(t, err)
    assert.Eq(t, "http://127.0.0.1:10801", runner.opts.ProxyURL)
    assert.Eq(t, []string{"mydev.com"}, runner.opts.ProxyExclude)
}
```

Add tests for:

- `enable=false` disables.
- legacy `global.proxy_url` fallback still works.
- `[http_proxy]` wins over legacy.
- `NO_PROXY=mydev.com` becomes `ProxyExclude`.
- `NO_PROXY=1` disables.

- [x] **Step 3: Write failing update tests**

In `internal/app/update_batch_test.go` or `internal/app/update_package_test.go`, add:

```go
func TestUpdateAllPackagesUsesHTTPProxyConfig(t *testing.T) {
    t.Setenv("NO_PROXY", "")
    installer := &fakeUpdateInstaller{}
    svc := UpdateService{
        Installer: installer,
        LoadConfig: func() (*cfgpkg.File, error) {
            cfg := cfgpkg.NewFile()
            proxyURL := "http://127.0.0.1:10801"
            cfg.HTTPProxy.URL = &proxyURL
            cfg.HTTPProxy.Exclude = []string{"mydev.com"}
            cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
            return cfg, nil
        },
        LoadInstalled: func() (*storepkg.Config, error) {
            return &storepkg.Config{Installed: map[string]storepkg.Entry{
                "BurntSushi/ripgrep": {Repo: "BurntSushi/ripgrep", Tag: "v13.0.0"},
            }}, nil
        },
        LatestInfo: func(app.LatestCheckTarget) (app.LatestInfo, error) {
            return app.LatestInfo{Tag: "v14.0.0"}, nil
        },
    }

    _, err := svc.UpdateAllPackages(install.Options{})
    assert.NoErr(t, err)
    assert.Eq(t, "http://127.0.0.1:10801", installer.options[0].ProxyURL)
    assert.Eq(t, []string{"mydev.com"}, installer.options[0].ProxyExclude)
}
```

Adjust fake names to existing test helpers.

- [x] **Step 4: Write failing CLI global network tests**

In `internal/cli/install_handler_test.go`, update `TestApplyGlobalNetworkConfig...` or add:

```go
func TestApplyGlobalNetworkConfigUsesHTTPProxy(t *testing.T) {
    t.Setenv("NO_PROXY", "")
    proxyURL := "http://127.0.0.1:10801"
    cfg := cfgpkg.NewFile()
    cfg.HTTPProxy.URL = &proxyURL
    cfg.HTTPProxy.Exclude = []string{"mydev.com"}

    opts := install.Options{}
    applyGlobalNetworkConfig(&opts, cfg)

    assert.Eq(t, proxyURL, opts.ProxyURL)
    assert.Eq(t, []string{"mydev.com"}, opts.ProxyExclude)
}
```

- [x] **Step 5: Verify RED**

Run:

```bash
go test ./internal/app -run "TestInstallTarget.*Proxy|TestUpdate.*Proxy|TestUpdateAllPackages.*Proxy" -count=1
go test ./internal/cli -run "TestApplyGlobalNetworkConfig.*Proxy|TestHandleListOutdated" -count=1
```

Expected: tests fail because app/cli still use `cfg.Global.ProxyURL`.

- [x] **Step 6: Implement install resolver integration**

In `internal/app/install_resolve.go`, replace direct global proxy mutation:

```go
global := cfg.Global
if util.GlobalProxyDisabled(cli.NoProxy) {
    global.ProxyURL = nil
}
```

with resolver-based logic:

```go
repoSection := cfg.Repos[repoKey]
proxy := cfgpkg.ResolveHTTPProxy(cfg, cfgpkg.ProxyResolveOptions{
    NoProxy:    cli.NoProxy,
    EnvNoProxy: os.Getenv("NO_PROXY"),
    OverrideURL: cli.ProxyURL,
    PackageURL: util.DerefString(pkg.ProxyURL),
    RepoURL:    util.DerefString(repoSection.ProxyURL),
})

global := cfg.Global
global.ProxyURL = nil
if proxy.Enabled {
    global.ProxyURL = &proxy.URL
}
```

Use `repoSection` when calling `MergeInstallOptions`.

Set returned install options:

```go
ProxyURL:     merged.ProxyURL,
ProxyExclude: append([]string(nil), proxy.Exclude...),
```

If `merged.ProxyURL` can override `proxy.URL`, ensure `ProxyExclude` still comes from resolver. If cleaner, skip `global.ProxyURL` mutation and set `ProxyURL: proxy.URL` directly after merge.

- [x] **Step 7: Implement update network integration**

In `internal/app/update_options.go`, update `applyConfigNetworkOptions`:

```go
proxy := cfgpkg.ResolveHTTPProxy(cfg, cfgpkg.ProxyResolveOptions{
    NoProxy: opts.NoProxy,
    EnvNoProxy: os.Getenv("NO_PROXY"),
    OverrideURL: opts.ProxyURL,
})
opts.ProxyURL = proxy.URL
opts.ProxyExclude = append([]string(nil), proxy.Exclude...)
```

Keep cache/api/ghproxy logic unchanged.

- [x] **Step 8: Implement CLI global network integration**

In `internal/cli/options.go`, update `applyGlobalNetworkConfig`:

```go
proxy := cfgpkg.ResolveHTTPProxy(cfg, cfgpkg.ProxyResolveOptions{
    NoProxy: opts.NoProxy,
    EnvNoProxy: os.Getenv("NO_PROXY"),
    OverrideURL: opts.ProxyURL,
})
opts.ProxyURL = proxy.URL
opts.ProxyExclude = append([]string(nil), proxy.Exclude...)
```

Remove direct `cfg.Global.ProxyURL` read.

- [x] **Step 9: Update outdated progress notice wording**

If Task 3 changed notice wording to `http_proxy`, update `internal/cli/outdated_progress.go`:

```go
ccolor.Fprintf(s.stderrWriter(), " - Using <ylw>http_proxy for GitHub API request</>: %s\n", s.proxyURL)
```

Update `internal/cli/list_handler_test.go` expected string from the old `proxy_url` notice wording to `http_proxy for GitHub API request`.

- [x] **Step 10: Verify GREEN**

Run:

```bash
go test ./internal/app -run "TestInstallTarget.*Proxy|TestUpdate.*Proxy|TestUpdateAllPackages.*Proxy" -count=1
go test ./internal/cli -run "TestApplyGlobalNetworkConfig.*Proxy|TestHandleListOutdated" -count=1
go test ./internal/app ./internal/cli
```

Expected: PASS.

- [x] **Step 11: Commit**

```bash
git add internal/app/install_resolve.go internal/app/update_options.go internal/cli/options.go internal/cli/outdated_progress.go internal/app/install_config_test.go internal/app/update_batch_test.go internal/cli/install_handler_test.go internal/cli/list_handler_test.go
git commit -m "feat: use http_proxy for package network options"
```

---

### Task 5: SDK Integration And Documentation

**Files:**
- Modify: `internal/app/sdk.go`
- Modify: `internal/app/sdk_test.go`
- Modify: `docs/config.md`
- Modify: `docs/config.zh-CN.md`
- Modify: `docs/example.eget.toml`
- Modify: `docs/superpowers/specs/2026-06-06-http-proxy-config-design.md` if implementation details differ from design

- [x] **Step 1: Run impact analysis**

```bash
npx gitnexus impact NewDefaultSDKService --repo eget --direction upstream
npx gitnexus impact sdkClientOptionsFromConfig --repo eget --direction upstream
```

Expected: LOW/MEDIUM. If HIGH/CRITICAL, report before editing.

- [x] **Step 2: Write failing SDK tests**

In `internal/app/sdk_test.go`, add:

```go
func TestNewDefaultSDKServiceUsesHTTPProxyConfig(t *testing.T) {
    t.Setenv("NO_PROXY", "")
    cfg := cfgpkg.NewFile()
    proxyURL := "http://127.0.0.1:10801"
    cfg.HTTPProxy.URL = &proxyURL
    cfg.HTTPProxy.Exclude = []string{"mydev.com"}

    service, err := NewDefaultSDKService(cfg)
    assert.NoErr(t, err)
    assert.Eq(t, proxyURL, service.ClientOpts.ProxyURL)
    assert.Eq(t, []string{"mydev.com"}, service.ClientOpts.ProxyExclude)
}

func TestNewDefaultSDKServiceHTTPProxyEnableFalseDisables(t *testing.T) {
    t.Setenv("NO_PROXY", "")
    cfg := cfgpkg.NewFile()
    proxyURL := "http://127.0.0.1:10801"
    enabled := false
    cfg.HTTPProxy.URL = &proxyURL
    cfg.HTTPProxy.Enable = &enabled

    service, err := NewDefaultSDKService(cfg)
    assert.NoErr(t, err)
    assert.Eq(t, "", service.ClientOpts.ProxyURL)
}

func TestNewDefaultSDKServiceHTTPProxyPrefersNewBlockOverLegacy(t *testing.T) {
    t.Setenv("NO_PROXY", "")
    cfg := cfgpkg.NewFile()
    legacyURL := "http://127.0.0.1:10801"
    currentURL := "http://127.0.0.1:10802"
    cfg.Global.ProxyURL = &legacyURL
    cfg.HTTPProxy.URL = &currentURL

    service, err := NewDefaultSDKService(cfg)
    assert.NoErr(t, err)
    assert.Eq(t, currentURL, service.ClientOpts.ProxyURL)
}
```

- [x] **Step 3: Verify RED**

Run:

```bash
go test ./internal/app -run "TestNewDefaultSDKService.*HTTPProxy" -count=1
```

Expected: compile or test failure because `sdkClientOptionsFromConfig` does not use `http_proxy`.

- [x] **Step 4: Implement SDK resolver integration**

In `internal/app/sdk.go`, update `sdkClientOptionsFromConfig`:

```go
proxy := cfgpkg.ResolveHTTPProxy(cfg, cfgpkg.ProxyResolveOptions{
    NoProxy: noProxy,
    EnvNoProxy: os.Getenv("NO_PROXY"),
})
opts.ProxyURL = proxy.URL
opts.ProxyExclude = append([]string(nil), proxy.Exclude...)
```

Remove direct `cfg.Global.ProxyURL` read.

- [x] **Step 5: Verify GREEN**

Run:

```bash
go test ./internal/app -run "TestNewDefaultSDKService.*HTTPProxy|TestNewDefaultSDKService.*Proxy" -count=1
go test ./internal/sdk -count=1
```

Expected: PASS.

- [x] **Step 6: Update config docs**

In `docs/config.md`, update proxy sections:

Add:

```markdown
### HTTP Proxy

Use `[http_proxy]` for global HTTP-layer proxy settings:

```toml
[http_proxy]
enable = true
url = "http://127.0.0.1:10801"
exclude = ["mydev.com", "*.corp.local", "10.0.0.0/8"]
```

`enable = false` or `url = ""` disables this config proxy. `exclude` matches request hosts and skips the configured proxy for those hosts. `global.proxy_url` is still read as a legacy fallback when `[http_proxy]` is not configured.
```

Mirror the same content in `docs/config.zh-CN.md`.

- [x] **Step 7: Update example config**

In `docs/example.eget.toml`, replace or supplement:

```toml
[global]
proxy_url = ""
```

with:

```toml
[http_proxy]
enable = false
url = ""
exclude = []
```

Keep a comment noting `global.proxy_url` legacy fallback only if the example already documents compatibility comments.

- [x] **Step 8: Verify docs references**

Run:

```bash
rg "global.proxy_url|proxy_url|http_proxy|NO_PROXY" docs/config.md docs/config.zh-CN.md docs/example.eget.toml
```

Expected: docs mention `[http_proxy]` as preferred and `global.proxy_url` as legacy fallback.

- [x] **Step 9: Full verification**

Run:

```bash
go test ./...
git diff --check
npx gitnexus detect-changes --repo eget
```

Expected:

- `go test ./...`: PASS.
- `git diff --check`: no output.
- GitNexus affected scope matches config/client/app/sdk proxy paths. If HIGH/CRITICAL, inspect and summarize before committing.

- [x] **Step 10: Commit**

```bash
git add internal/app/sdk.go internal/app/sdk_test.go docs/config.md docs/config.zh-CN.md docs/example.eget.toml docs/superpowers/specs/2026-06-06-http-proxy-config-design.md
git commit -m "feat: use http_proxy for sdk network options"
```

---

### Task 6: Final Audit And Compatibility Sweep

**Files:**
- Modify only files needed by failures discovered in this task.

- [x] **Step 1: Search for stale direct global proxy usage**

Run:

```bash
rg "GlobalProxyDisabled|Global\\.ProxyURL|proxy_url for|ProxyURL" internal docs -n
```

Expected:

- Direct `cfg.Global.ProxyURL` reads remain only in config model/docs/legacy fallback code/tests.
- Runtime network paths use `ResolveHTTPProxy` or receive already resolved `ProxyURL`/`ProxyExclude`.
- Notice wording is consistent.

- [x] **Step 2: Verify command-level no-proxy behavior**

Run targeted tests:

```bash
go test ./internal/cli -run "NoProxy|Proxy" -count=1
go test ./internal/app -run "NoProxy|Proxy" -count=1
```

Expected: PASS.

- [x] **Step 3: Verify full suite**

Run:

```bash
go test ./...
git diff --check
npx gitnexus detect-changes --repo eget
```

Expected: PASS / no diff check output. Record GitNexus risk in final response or commit notes.

- [x] **Step 4: Final commit if needed**

If Task 6 required fixes:

```bash
git add <changed-files>
git commit -m "test: cover http_proxy compatibility paths"
```

If no files changed, do not create an empty commit.

## Execution Notes

- Use TDD for each task: add failing tests, verify failure, implement minimal code, verify pass.
- Run `go test ./...` before claiming the implementation complete because this affects the CLI and network main paths.
- Run `npx gitnexus detect-changes --repo eget` before every commit, per project rules.
- Do not remove legacy `global.proxy_url` support in this work.
- Do not add `cfg path` support for `http_proxy.*`.
- Do not implement `*` as proxy disable syntax.
- Keep refactors local to proxy/config/client integration. Do not split unrelated large files during this work.

## Completion Criteria

- `[http_proxy]` loads, saves, and can be set by path.
- `global.proxy_url` still works when `[http_proxy]` is absent.
- `[http_proxy]` wins over `global.proxy_url` when present.
- `enable=false`, `url=""`, `--no-proxy`, `NO_PROXY=1|true|yes|on` disable config proxy.
- `NO_PROXY` host lists and `[http_proxy].exclude` skip proxy by request host.
- Install/download/update/outdated/SDK network paths receive `ProxyURL` and `ProxyExclude`.
- Proxy notice only appears when the request actually uses configured proxy.
- `go test ./...` passes.
