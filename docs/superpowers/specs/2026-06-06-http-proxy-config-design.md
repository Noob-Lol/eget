# HTTP Proxy 配置设计

## 背景

当前 `eget` 使用 `global.proxy_url` 表达全局 HTTP 层代理。这个字段已经被安装、更新、下载、SDK 下载、GitHub API 查询等路径复用，但它只能表达一个代理地址，缺少显式启用状态和按目标排除规则。

现有行为里还有两个控制入口：

- app 级 `--no-proxy`：手动禁用 `global.proxy_url`。
- 环境变量 `NO_PROXY`：控制禁止 `global.proxy_url` 生效。

随着 SDK、cache mirror、URL template、SourceForge、forge 等网络路径增加，继续把代理策略散落在 `global.proxy_url`、`NO_PROXY`、各入口 options 和 client proxy func 中，会让行为越来越难判断。需要把“配置代理”提升为一等配置块，并把代理解析收敛到统一的策略层。

## 目标

- 新增独立配置块 `[http_proxy]`。
- 支持全局启用开关、代理地址和排除规则。
- 保留 legacy `global.proxy_url` 读取兼容。
- 保留 repo/package/CLI 级 `proxy_url` override 能力。
- 统一 `--no-proxy`、`NO_PROXY`、`http_proxy.exclude` 的行为。
- 让 GitHub API、远程下载、SDK index/download/install/update/list outdated 等网络路径使用同一套代理解析结果。
- 代理排除规则按请求 host 生效，而不是只能粗暴禁用整个代理。

## 非目标

- 不要求 `eget cfg path` 支持 `http_proxy.url`、`http_proxy.enable`、`http_proxy.exclude`。
- 不支持 `exclude = ["*"]` 作为禁用代理的语义。禁用代理已经由 `enable = false`、`url = ""`、`--no-proxy` 表达。
- 不废弃 repo/package/CLI 级 `proxy_url` override。
- 不立即删除 legacy `global.proxy_url`。
- 不为每次读取 legacy `global.proxy_url` 输出 deprecated warning。
- 不改变 `ghproxy` 的语义。`ghproxy` 仍然是 GitHub URL rewrite，`http_proxy` 是 HTTP 层代理，两者可同时启用。
- 不要求 `sdk search` 使用代理。`sdk search` 只读本地 index cache，不涉及网络。

## 配置格式

推荐新配置：

```toml
[http_proxy]
enable = true
url = "http://127.0.0.1:10801"
exclude = [
  "localhost",
  "127.0.0.1",
  "mydev.com",
  "*.corp.local",
  "10.0.0.0/8",
]
```

字段含义：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `enable` | bool，可省略 | 是否启用配置代理 |
| `url` | string | 代理地址 |
| `exclude` | string array | 不走该代理的 host/domain/ip/cidr 规则 |

`enable` 使用可省略语义：

- `enable = true`：启用配置代理，前提是 `url` 非空。
- `enable = false`：禁用配置代理，即使 `url` 非空也不使用。
- `enable` 未配置：如果 `url` 非空，则默认启用。
- `url = ""`：不启用配置代理。

## 兼容策略

旧配置继续可读：

```toml
[global]
proxy_url = "http://127.0.0.1:10801"
```

兼容规则：

1. 如果存在新配置 `[http_proxy]`，优先使用新配置。
2. 如果没有配置 `[http_proxy]`，但 `global.proxy_url` 有值，则 fallback 到旧配置。
3. `global.proxy_url` 作为 deprecated legacy 配置保留读取。
4. 暂时不输出 deprecated warning，避免每次命令都打扰用户。
5. 后续如需迁移，可以单独设计迁移命令或迁移提示。

判断“是否存在 `[http_proxy]`”时，建议以该块内任一字段非零为准：

- `http_proxy.url` 非空。
- `http_proxy.enable` 非 nil。
- `http_proxy.exclude` 非空。

## 优先级

统一代理策略优先级：

```text
--no-proxy
> NO_PROXY 全局禁用形式
> [http_proxy].enable = false
> 目标 host 命中 NO_PROXY / [http_proxy].exclude
> CLI/repo/package proxy_url override
> [http_proxy].url
> legacy [global].proxy_url
```

说明：

- `--no-proxy` 是最高优先级，一旦设置，禁用 eget 配置代理。
- `NO_PROXY=1`、`NO_PROXY=true`、`NO_PROXY=yes`、`NO_PROXY=on` 禁用整个 eget 配置代理。
- `NO_PROXY=mydev.com,*.corp.local,10.0.0.0/8` 作为额外 exclude 规则。
- `[http_proxy].enable = false` 是配置级禁用。
- exclude 命中后，不使用配置代理。
- repo/package/CLI 级 `proxy_url` override 仍可覆盖全局代理地址。
- exclude 对最终代理地址生效，不区分代理地址来自全局、repo、package 还是 CLI。

不支持 `NO_PROXY=*` 作为全局禁用。需要全局禁用时使用：

```bash
eget --no-proxy install fzf
```

或配置：

```toml
[http_proxy]
enable = false
```

或：

```toml
[http_proxy]
url = ""
```

## Repo/Package Override

继续保留当前已有能力：

```toml
[repos."owner/repo"]
proxy_url = "http://127.0.0.1:10802"

[packages.foo]
proxy_url = "http://127.0.0.1:10803"
```

代理地址选择顺序：

```text
CLI proxy_url override
> package.proxy_url
> repo.proxy_url
> http_proxy.url
> legacy global.proxy_url
```

exclude 规则仍应用在最终代理上。例如 package 指定了 `proxy_url`，但目标请求 host 命中 `http_proxy.exclude` 或 `NO_PROXY`，该请求仍不走代理。

## Exclude 匹配规则

`exclude` 只匹配请求 URL 的 host，不匹配 path/query。

支持规则：

```toml
exclude = [
  "localhost",
  "127.0.0.1",
  "mydev.com",
  "*.mydev.com",
  ".corp.local",
  "10.0.0.0/8",
  "mydev.com:8080",
]
```

语义：

| 规则 | 匹配 |
| --- | --- |
| `localhost` | 精确匹配 `localhost` |
| `127.0.0.1` | 精确匹配该 IP |
| `mydev.com` | 匹配 `mydev.com` 和子域名，例如 `api.mydev.com` |
| `*.mydev.com` | 匹配子域名，例如 `api.mydev.com`，不匹配裸域 `mydev.com` |
| `.corp.local` | 等价于 `*.corp.local` |
| `10.0.0.0/8` | CIDR 匹配 IP |
| `mydev.com:8080` | host + port 精确匹配 |

匹配细节：

- host 统一转 lowercase。
- 如果请求 host 带端口，先拆分 host 和 port。
- exclude 规则带端口时，host 和 port 都必须匹配。
- exclude 规则不带端口时，忽略请求端口。
- IPv6 后续可支持，实现时应使用 `net.SplitHostPort` 避免错误拆分。
- 不支持 `*` 作为禁用代理语义。

## 配置模型

新增配置结构：

```go
type HTTPProxySection struct {
    Enable  *bool    `toml:"enable" mapstructure:"enable"`
    URL     *string  `toml:"url" mapstructure:"url"`
    Exclude []string `toml:"exclude" mapstructure:"exclude"`
}
```

`config.File` 增加：

```go
type File struct {
    Global      Section            `toml:"global" mapstructure:"global"`
    HTTPProxy   HTTPProxySection   `toml:"http_proxy" mapstructure:"http_proxy"`
    ApiCache    APICacheSection    `toml:"api_cache" mapstructure:"api_cache"`
    Ghproxy     GhproxySection     `toml:"ghproxy" mapstructure:"ghproxy"`
    CacheMirror CacheMirrorSection `toml:"cache_mirror" mapstructure:"cache_mirror"`
    Repos       map[string]Section
    Packages    map[string]Section `toml:"packages" mapstructure:"packages"`
    SDK         map[string]SDKSection `toml:"sdk" mapstructure:"sdk"`
}
```

运行时建议新增解析结果：

```go
type ProxyConfig struct {
    Enabled bool
    URL     string
    Exclude []string
}
```

以及 resolver：

```go
type ProxyResolveOptions struct {
    NoProxy    bool
    EnvNoProxy string
    OverrideURL string
    PackageURL string
    RepoURL    string
}

func ResolveHTTPProxy(cfg *config.File, opts ProxyResolveOptions) ProxyConfig
```

resolver 负责：

- 处理 `--no-proxy`。
- 处理 `NO_PROXY`。
- 处理 `[http_proxy].enable/url/exclude`。
- fallback legacy `global.proxy_url`。
- 合并 env exclude 和 config exclude。
- 得出最终 `ProxyConfig`。

## Client 层设计

当前很多网络路径只传：

```go
ProxyURL string
```

这不足以支持 exclude，因为 exclude 需要知道每次请求的目标 host。

建议扩展 client options：

```go
type Options struct {
    ProxyURL     string
    ProxyExclude []string
    // existing fields...
}
```

将 proxy func 改为 request-aware：

```go
func ProxyFuncFor(proxyURL string, exclude []string) (func(*http.Request) (*url.URL, error), error)
```

行为：

```go
if proxyURL == "" {
    return nil, nil
}

parsedProxy, err := url.Parse(proxyURL)
if err != nil {
    return nil, fmt.Errorf("invalid proxy_url %q: %w", proxyURL, err)
}

return func(req *http.Request) (*url.URL, error) {
    if ProxyExcluded(req.URL.Host, exclude) {
        return nil, nil
    }
    return parsedProxy, nil
}, nil
```

这样 GitHub API、下载请求、SDK index、SDK download 都可以共享同一套代理判断。

## NO_PROXY 解析

建议把当前 `util.GlobalProxyDisabled(noProxy bool)` 升级为两层能力：

```go
func GlobalProxyDisabled(noProxy bool, envNoProxy string) bool
func ParseNoProxyExclude(envNoProxy string) []string
```

全局禁用值：

```text
1
true
yes
on
```

`NO_PROXY=mydev.com,*.corp.local,10.0.0.0/8` 解析为 exclude 列表。

`NO_PROXY=*` 不作为全局禁用。建议实现时将 `*` 作为无效 exclude 规则忽略，避免破坏用户已有 shell 环境。

## 配置合并

当前 `config.Merge` 会合并：

```go
global.ProxyURL
repo.ProxyURL
pkg.ProxyURL
cli.ProxyURL
```

设计调整：

- `Merged.ProxyURL` 可以暂时保留，降低改动面。
- 新增 `Merged.ProxyExclude []string`，或在 `install.Options` / `client.Options` 中新增 `ProxyExclude`。
- 全局默认代理 URL 来源从 `global.ProxyURL` 改为 `ResolveHTTPProxy` 的结果。
- package/repo/CLI 的 `proxy_url` override 继续走 `Merged.ProxyURL`。
- exclude 统一应用到最终 request。

伪逻辑：

```go
proxy := ResolveHTTPProxy(cfg, ProxyResolveOptions{
    NoProxy: cli.NoProxy,
    EnvNoProxy: os.Getenv("NO_PROXY"),
    OverrideURL: cli.ProxyURL,
    PackageURL: pkg.ProxyURL,
    RepoURL: repo.ProxyURL,
})

opts.ProxyURL = proxy.URL
opts.ProxyExclude = proxy.Exclude
```

`NoProxy` 不应被滥用成“最终没有 proxy”。建议语义保持清楚：

- `NoProxy`：用户显式要求禁用配置代理。
- `ProxyURL == ""`：最终没有可用代理。
- `ProxyExclude`：按目标 host 排除。

## SDK 接入

`app.NewDefaultSDKService(cfg, noProxy)` 应从新 resolver 获取代理配置：

```go
proxy := ResolveHTTPProxy(cfg, ProxyResolveOptions{
    NoProxy: noProxy,
    EnvNoProxy: os.Getenv("NO_PROXY"),
})

opts.ProxyURL = proxy.URL
opts.ProxyExclude = proxy.Exclude
```

需要使用代理配置的 SDK 路径：

- `sdk index refresh`
- `sdk download`
- `sdk install`

不需要代理配置的 SDK 路径：

- `sdk search`：只读本地 index cache。
- `sdk list`：只读本地 installed store。
- `sdk path`：只读本地 installed store。
- `sdk remove`：只操作本地安装目录和 installed store。

## Install/Update/Outdated 接入

需要统一这些路径：

- `eget install`
- `eget download`
- `eget update`
- `eget list --outdated`
- GitHub API latest release 请求
- GitHub release asset 下载
- SourceForge / forge / URL template 下载
- checksum 下载

相关现有位置包括：

- `internal/app/install_resolve.go`
- `internal/app/update_options.go`
- `internal/app/sdk.go`
- `internal/cli/options.go`
- `internal/client/http_client.go`
- `internal/client/download_file.go`
- `internal/client/download_range.go`
- `internal/install/network.go`

设计目标是这些入口最终都只消费：

```go
ProxyURL
ProxyExclude
```

而不是各自判断 `global.proxy_url`。

## 输出文案

当前文案：

```text
 - Using http_proxy for GitHub API request: http://127.0.0.1:10801
 - Using http_proxy for download request: http://127.0.0.1:10801
```

建议改为：

```text
 - Using http_proxy for GitHub API request: http://127.0.0.1:10801
 - Using http_proxy for download request: http://127.0.0.1:10801
```

不区分代理地址来自全局、repo、package 还是 CLI，先统一叫 `http_proxy`，降低输出复杂度。

proxy notice 应只在实际使用代理时输出。若请求 host 命中 exclude，不应输出 proxy notice。

## 错误处理

代理 URL 非法时，可以先沿用当前错误风格：

```text
invalid proxy_url "xxx": ...
```

后续如果需要更精确来源，可以引入来源信息：

```go
type ProxySource string

const (
    ProxySourceHTTPProxy    ProxySource = "http_proxy.url"
    ProxySourceGlobalLegacy ProxySource = "global.proxy_url"
    ProxySourcePackage      ProxySource = "package.proxy_url"
    ProxySourceRepo         ProxySource = "repo.proxy_url"
    ProxySourceCLI          ProxySource = "--proxy-url"
)
```

但这不是首期必要范围。

## 测试计划

配置加载：

- `[http_proxy] url + enable=true` 生效。
- `[http_proxy] url + enable` 未配置时，url 非空默认生效。
- `[http_proxy] url + enable=false` 不生效。
- `[http_proxy] url=""` 不生效。
- 无 `[http_proxy]` 时 fallback `global.proxy_url`。
- 同时存在 `[http_proxy].url` 和 `global.proxy_url` 时，优先新配置。
- `[http_proxy].exclude` 能正确加载。

代理解析：

- `--no-proxy` 禁用新旧配置。
- `NO_PROXY=1` 禁用新旧配置。
- `NO_PROXY=true` 禁用新旧配置。
- `NO_PROXY=mydev.com` 作为 exclude。
- `NO_PROXY=mydev.com,*.corp.local` 与 `[http_proxy].exclude` 合并。
- `NO_PROXY=*` 不作为全局禁用。

exclude 匹配：

- exact host：`localhost`。
- IP：`127.0.0.1`。
- domain：`mydev.com` 匹配 `mydev.com` 和 `api.mydev.com`。
- wildcard：`*.mydev.com` 匹配 `api.mydev.com`，不匹配 `mydev.com`。
- dot-prefix：`.corp.local` 匹配 `api.corp.local`。
- CIDR：`10.0.0.0/8` 匹配 `10.2.3.4`。
- host:port：`mydev.com:8080` 只匹配该端口。
- unmatched host 继续走代理。

client 行为：

- GitHub API 请求命中 proxy 时使用 proxy。
- download 请求命中 proxy 时使用 proxy。
- 命中 exclude 时不使用 proxy。
- proxy notice 只在实际使用代理时输出。
- exclude 后不输出 proxy notice。

业务入口：

- `eget install` 使用新 `[http_proxy]`。
- `eget download` 使用新 `[http_proxy]`。
- `eget update` 使用新 `[http_proxy]`。
- `eget list --outdated` 使用新 `[http_proxy]`。
- `eget sdk install/download/index refresh` 使用新 `[http_proxy]`。
- `eget sdk search` 不受影响。

兼容：

- 旧配置 `global.proxy_url` 的现有测试保留。
- package/repo `proxy_url` override 保留。
- CLI `--no-proxy` 原有行为保留。

## 推荐实施阶段

建议拆成 4 个 commit，降低风险：

### 阶段 1：配置模型

提交建议：

```text
feat: add http_proxy config model
```

范围：

- 新增 `HTTPProxySection`。
- 配置加载/保存支持。
- 保留 legacy `global.proxy_url`。
- 更新配置文档和示例。

### 阶段 2：代理解析器

提交建议：

```text
feat: resolve http proxy config
```

范围：

- 新增 proxy resolver。
- 支持 enable/url/exclude/NO_PROXY/legacy fallback。
- 补 resolver 单测。

### 阶段 3：HTTP client exclude

提交建议：

```text
feat: apply proxy excludes to http client
```

范围：

- 扩展 client/install/sdk options。
- `ProxyFuncFor` 支持 request host exclude。
- proxy notice 改为只在实际使用代理时输出。
- 补 client 层测试。

### 阶段 4：业务入口接入

提交建议：

```text
feat: use http_proxy across install and sdk
```

范围：

- install/update/outdated/sdk 接入 resolver。
- 保留 package/repo/CLI `proxy_url` override。
- 更新旧测试和新增集成路径测试。
- 可选调整 proxy notice 文案为 `http_proxy`。

## 推荐方案

采用“新 `[http_proxy]` 作为全局代理配置块，保留 package/repo/CLI `proxy_url` override，保留 legacy `global.proxy_url` fallback”的方案。

这个方案的好处：

- 配置结构更清楚。
- 不破坏已有用户配置。
- `exclude` 能按请求 host 生效。
- `NO_PROXY`、`--no-proxy`、SDK、install/update/download 能统一到一套逻辑。
- 后续如果要支持更多代理类型或更复杂策略，有清晰扩展点。
