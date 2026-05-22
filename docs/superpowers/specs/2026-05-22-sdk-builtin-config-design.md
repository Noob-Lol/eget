# 内置 SDK 配置模板设计

## 背景

当前 `eget sdk` 依赖用户手写 `[sdk.<name>]` 配置。Go、Node、JDK 这类常用 SDK 的配置字段较多，包含 `target`、`index_url`、`url_template`、`filename_pattern`、平台映射、解压参数等，从 0 开始配置成本偏高，也容易写错文件名模式或镜像 URL。

本设计新增内置 SDK 配置模板，并通过 `eget sdk config add` 快速写入用户的 `eget.toml`。默认写入官方源配置；用户显式加 `--mirror` 时写入内置镜像源配置。

## 目标

- 内置 `go`、`node`、`jdk` 三个 SDK 配置模板。
- 用户可以通过命令把模板写入当前 eget 配置文件。
- 默认使用官方 index/download 地址。
- 通过 `--mirror` 使用内置镜像地址。
- 支持单个添加、别名添加、批量添加和强制覆盖。
- 不自动刷新 index，不自动安装 SDK。

## 非目标

- 不实现可扩展的第三方模板仓库。
- 不实现交互式选择镜像。
- 不实现 `sdk config list/show/edit`。
- 不让 `eget config init` 默认写入 SDK 配置。
- 不在第一版支持多个命名镜像，例如 `--mirror aliyun`、`--mirror huaweicloud`。

## 命令设计

新增命令树：

```bash
eget sdk config add <name>
eget sdk config add --all
```

选项：

```bash
--all, -a      添加全部内置 SDK 配置
--force, -f    已存在时覆盖
--mirror, -m   使用内置镜像源配置；默认使用官方源配置
```

示例：

```bash
eget sdk config add go
eget sdk config add node
eget sdk config add jdk
eget sdk config add java
eget sdk config add --all
eget sdk config add jdk --mirror
eget sdk config add --all --mirror
eget sdk config add jdk --force
```

## SDK 名称和别名

内置规范名：

| 规范名 | 别名 |
| --- | --- |
| `go` | `golang` |
| `node` | `nodejs` |
| `jdk` | `java` |

用户传入别名时，写入配置仍使用规范名。例如：

```bash
eget sdk config add java
```

写入：

```toml
[sdk.jdk]
aliases = ["java"]
```

## 默认源和镜像源策略

默认不加 `--mirror` 时写入官方源配置：

- Go: `https://go.dev/dl/`
- Node: `https://nodejs.org/dist/`
- JDK: `https://jdk.java.net/archive/`

加 `--mirror` 时写入内置镜像源配置：

- Go: `https://mirrors.aliyun.com/golang/`
- Node: `https://mirrors.aliyun.com/nodejs-release/`
- JDK: `https://mirrors.huaweicloud.com/openjdk/`

JDK 官方源说明：

- 官方 JDK 配置优先使用 `https://jdk.java.net/archive/`，因为页面包含可直接下载的 OpenJDK 归档链接。
- 官方 JDK 配置不依赖 `url_template`，而是依赖 index 中解析到的完整下载 URL。
- 因此安装官方 JDK 前需要先构建 index：

```bash
eget sdk index build jdk
eget sdk install jdk@21.0.2
```

镜像 JDK 配置继续使用华为云 OpenJDK 镜像，可通过 `url_template` 根据版本直接渲染下载 URL。

## 内置模板数据

新增强类型模板模块：

```text
internal/sdk/builtin_config.go
```

建议类型：

```go
type BuiltinConfigSource string

const (
    BuiltinConfigOfficial BuiltinConfigSource = "official"
    BuiltinConfigMirror   BuiltinConfigSource = "mirror"
)

type BuiltinConfig struct {
    Name    string
    Aliases []string
    Source  BuiltinConfigSource
    Section cfgpkg.SDKSection
}
```

建议 API：

```go
func BuiltinConfigs(source BuiltinConfigSource) []BuiltinConfig
func FindBuiltinConfig(name string, source BuiltinConfigSource) (BuiltinConfig, bool)
func BuiltinConfigNames() []string
```

模板使用 `cfgpkg.SDKSection`，不从 `docs/example.eget.toml` 动态解析。原因：

- 文档示例包含注释和说明，不适合作为运行时数据源。
- 运行时模板需要强类型字段，便于测试和避免字段漂移。
- 文档可以从模板行为反向更新，但不应该驱动运行时逻辑。

## 官方源模板内容

Go 官方模板：

```toml
[sdk.go]
aliases = ["golang"]
target = "gosdk/go{version}"
url_template = "https://go.dev/dl/go{version}.{os}-{arch}.{ext}"
index_url = "https://go.dev/dl/"
index_format = "html"
filename_pattern = "go{version}.{os}-{arch}.{ext}"
strip_components = 1
ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }
```

Node 官方模板：

```toml
[sdk.node]
aliases = ["nodejs"]
target = "nodejs/node{version}"
url_template = "https://nodejs.org/dist/v{version}/node-v{version}-{os}-{arch}.{ext}"
index_url = "https://nodejs.org/dist/"
index_format = "html"
filename_pattern = "node-v{version}-{os}-{arch}.{ext}"
strip_components = 1
os_map = { windows = "win", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64", 386 = "x86" }
ext_map = { windows = "zip", linux = "tar.xz", darwin = "tar.gz" }
```

JDK 官方模板：

```toml
[sdk.jdk]
aliases = ["java"]
target = "jdk/openjdk-{version}"
index_url = "https://jdk.java.net/archive/"
index_format = "html"
filename_pattern = "openjdk-{version}_{os}-{arch}_bin.{ext}"
strip_components = 1
arch_map = { amd64 = "x64", arm64 = "aarch64" }
os_map = { darwin = "macos" }
ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }
```

## 镜像源模板内容

Go 镜像模板：

```toml
[sdk.go]
aliases = ["golang"]
target = "gosdk/go{version}"
url_template = "https://mirrors.aliyun.com/golang/go{version}.{os}-{arch}.{ext}"
index_url = "https://mirrors.aliyun.com/golang/"
index_format = "html"
filename_pattern = "go{version}.{os}-{arch}.{ext}"
strip_components = 1
ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }
```

Node 镜像模板：

```toml
[sdk.node]
aliases = ["nodejs"]
target = "nodejs/node{version}"
url_template = "https://mirrors.aliyun.com/nodejs-release/v{version}/node-v{version}-{os}-{arch}.{ext}"
index_url = "https://mirrors.aliyun.com/nodejs-release/"
index_format = "html"
index_path_prefix = "/nodejs-release/"
filename_pattern = "node-v{version}-{os}-{arch}.{ext}"
strip_components = 1
os_map = { windows = "win", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64", 386 = "x86" }
ext_map = { windows = "zip", linux = "tar.xz", darwin = "tar.gz" }
```

JDK 镜像模板：

```toml
[sdk.jdk]
aliases = ["java"]
target = "jdk/openjdk-{version}"
index_url = "https://mirrors.huaweicloud.com/openjdk/"
index_format = "html"
url_template = "https://mirrors.huaweicloud.com/openjdk/{version}/openjdk-{version}_{os}-{arch}_bin.{ext}"
filename_pattern = "openjdk-{version}_{os}-{arch}_bin.{ext}"
strip_components = 1
arch_map = { amd64 = "x64", arm64 = "aarch64" }
os_map = { darwin = "macos" }
ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }
```

## 应用层设计

在 `app.ConfigService` 增加 SDK 配置写入能力，因为该服务已经负责配置文件读写。

建议类型：

```go
type SDKConfigAddOptions struct {
    Name   string
    All    bool
    Force  bool
    Mirror bool
}

type SDKConfigAddResult struct {
    Items []SDKConfigAddItem
}

type SDKConfigAddItem struct {
    Name   string
    Action string
    Reason string
}
```

`Action` 值：

- `added`: 新增。
- `updated`: 使用 `--force` 覆盖。
- `skipped`: `--all` 时发现已存在并跳过。

建议方法：

```go
func (s ConfigService) AddSDKConfig(opts SDKConfigAddOptions) (SDKConfigAddResult, error)
```

行为规则：

- `Name` 和 `All` 必须二选一。
- `Mirror=false` 使用官方模板。
- `Mirror=true` 使用镜像模板。
- `cfg.SDK == nil` 时初始化 map。
- 单个添加时，`Name` 先通过 `FindBuiltinConfig` 解析别名到规范名。
- 不存在时写入 `cfg.SDK[name] = builtin.Section`。
- 已存在且 `Force=false`：
  - 单个添加返回错误。
  - `--all` 记为 `skipped`，不返回错误。
- 已存在且 `Force=true`：覆盖为当前选择源的模板。
- 只有出现 `added` 或 `updated` 时才保存配置文件。

## CLI 设计

在 `internal/cli/sdk_cmd.go` 增加：

```text
sdk
  config
    add
```

建议 options：

```go
type SDKConfigOptions struct {
    Action string
    Name   string
    All    bool
    Force  bool
    Mirror bool
}
```

handler key：

```text
sdk.config.add
```

CLI 校验：

- `eget sdk config add`：错误，要求 `<name>` 或 `--all`。
- `eget sdk config add jdk --all`：错误，要求 `<name>` 和 `--all` 二选一。
- `eget sdk config add ruby`：错误，提示可用内置 SDK。

输出：

```text
✓ Added SDK config: jdk
✓ Updated SDK config: jdk
- Skipped SDK config: jdk already exists
```

如果使用 `--mirror`，输出可以追加 source：

```text
✓ Added SDK config: jdk (mirror)
```

## 与现有命令的关系

`eget sdk config add` 只写配置。

构建 index 继续使用：

```bash
eget sdk index build jdk
```

安装 SDK 继续使用：

```bash
eget sdk install jdk@21.0.2
```

完整首次使用流程：

```bash
eget config init
eget sdk config add --all --mirror
eget sdk index build jdk
eget sdk install jdk@21.0.2
```

## 测试计划

模板层：

- `FindBuiltinConfig("go", official)` 返回官方 Go 模板。
- `FindBuiltinConfig("golang", official)` 返回规范名 `go`。
- `FindBuiltinConfig("java", mirror)` 返回规范名 `jdk`。
- `FindBuiltinConfig("ruby", official)` 返回 false。
- 官方 Go 模板使用 `https://go.dev/dl/`。
- 镜像 Go 模板使用 `https://mirrors.aliyun.com/golang/`。
- 官方 JDK 模板没有 `url_template`，并使用 `https://jdk.java.net/archive/`。
- 镜像 JDK 模板包含华为云 `url_template`。

应用层：

- 空配置添加 `jdk`，写入 `[sdk.jdk]` 官方模板。
- 空配置添加 `jdk --mirror`，写入华为云模板。
- 通过别名 `java` 添加，实际写入 `[sdk.jdk]`。
- 已存在且不带 `--force`，单个添加返回错误。
- 已存在且带 `--force`，覆盖为当前 source 的模板。
- `--all` 添加全部缺失模板。
- `--all` 遇到已存在模板时跳过并继续。
- `Name` 和 `All` 同时设置返回互斥错误。

CLI 层：

- `eget sdk config add jdk` 分发 `sdk.config.add`，`Name=jdk`。
- `eget sdk config add --all --mirror --force` 分发对应 flags。
- `eget sdk config add jdk --all` 返回互斥错误。
- help 中包含 `sdk config add --all --mirror` 示例。

集成验证：

```bash
go test ./...
```

可选手工验证：

```bash
eget sdk config add jdk --mirror
eget config get sdk.jdk.index_url
eget sdk index build jdk
```

## 风险和约束

- 官方 JDK index 使用 `jdk.java.net/archive`，适合稳定归档版本；`@latest` 语义取决于该页面可解析到的最高稳定版本。
- 如果未来要支持多个镜像名，需要把 `Mirror bool` 演进为 `Source string` 或 `MirrorName string`，但第一版不引入这个复杂度。
- 模板字段需要和 `docs/example.eget.toml` 保持人工同步；实现时应通过测试锁定关键 URL 和 filename pattern。

## 成功标准

- 用户可以不用手写 TOML，通过 `eget sdk config add jdk --mirror` 写入可用 JDK 镜像配置。
- 默认不加 `--mirror` 时写入官方源配置。
- `--all` 能一次写入 `go`、`node`、`jdk`。
- 已有配置不会被意外覆盖。
- `go test ./...` 通过。
