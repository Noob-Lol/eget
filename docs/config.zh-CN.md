# 配置说明

本文档用于说明 `eget` 配置文件。README 中只保留简要介绍；需要完整字段和目录语义时，以本文档为准。

## 配置查找顺序

`eget` 会按以下顺序解析配置文件：

1. `EGET_CONFIG`
2. `{EGET_CONFIG_DIR}/eget.toml`
3. 旧路径 `~/.eget.toml`
4. 当前目录 `eget.toml`
5. XDG / home fallback 路径，例如 `~/.config/eget/eget.toml`

`EGET_CONFIG` 只影响 `eget.toml` 文件位置；`EGET_CONFIG_DIR` 会影响默认配置目录，包括 `.env`、`eget.toml`、`installed.toml` 和 `sdk.installed.json`。

创建默认配置：

```bash
eget config init
```

默认写入：

```text
~/.config/eget/eget.toml
```

`eget` 还会加载同目录下的 dotenv 文件：

```text
~/.config/eget/.env
```

如果设置了 `XDG_CONFIG_HOME`，dotenv 路径也会跟随配置目录：

```text
$XDG_CONFIG_HOME/eget/.env
```

如果设置了 `EGET_CONFIG_DIR`，dotenv 路径为：

```text
{EGET_CONFIG_DIR}/.env
```

`.env` 文件是可选的。它会在 `eget.toml` 之前加载，因此配置文件可以继续通过 gookit/config 的环境变量展开引用敏感信息或内部配置：

```dotenv
GITHUB_TOKEN=...
PROXY_URL=http://127.0.0.1:7890
EGET_SELF_UPDATE_SOURCE=https://example.com/tools/eget/
```

```toml
[global]
github_token = "${GITHUB_TOKEN}"
proxy_url = "${PROXY_URL}"
```

不要把 `.env` 提交到版本库。

## 配置块

支持的配置块：

- `[global]`: 全局默认值、网络和缓存配置。
- `[api_cache]`: provider 元数据 API 响应缓存。
- `[ghproxy]`: GitHub URL 重写代理。
- `["owner/repo"]`: ~旧版直接 package 配置~。
- `[packages.<name>]`: 命名 package 配置。
- `[sdk.<name>]`: SDK 下载和 index 配置。

## Global 配置

示例：

```toml
[global]
target = "~/.local/bin"
gui_target = "~/Applications"
cache_dir = "~/.cache/eget"
proxy_url = ""
user_agent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
system = ""
sys7z_path = ""
chunk_concurrency = 0
batch_concurrency = 0
ignore_update_packages = []
sdk_target = "~/.local/sdks"
sdk_ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }
```

字段说明：

- `target`: CLI 工具默认安装目录。
- `gui_target`: 免安装 GUI 应用默认安装目录。
- `cache_dir`: 缓存根目录。原始下载缓存、API cache、SDK 下载缓存和 SDK index 都基于此目录派生。
- `proxy_url`: HTTP 层代理。GitHub 查询和远程下载都会使用它。
- `user_agent`: HTTP 请求默认 `User-Agent`。为空时使用内置浏览器 UA；配置后覆盖默认值。
- `system`: 默认目标平台，格式为 `GOOS/GOARCH`，例如 `windows/amd64`。
- `sys7z_path`: 可选 7z 可执行文件路径。为空时会从 `PATH` 依次查找 `7z`、`7zz`、`7za`。
- `chunk_concurrency`: 远程下载分块并发数。`0` 表示使用内置默认行为。
- `batch_concurrency`: 批量 package 操作并发数。`0` 表示串行或使用命令自己的默认行为。
- `ignore_update_packages`: 在 `list --outdated`、`update --check`、`update --all` 中跳过的 package 名称。
- `sdk_target`: SDK 安装根目录。SDK 配置里的相对 `target` 会基于该目录解析。
- `sdk_ext_map`: SDK 默认归档扩展名映射，key 使用 Go OS 名称。SDK 级别 `ext_map` 会覆盖它。

目录语义：

- `download` 未指定 `--to` 时默认使用 `cache_dir`。
- `install` 和 `download` 会优先复用 `{cache_dir}/pkg-cache/` 中已有的 package 下载缓存。
- API cache 文件写入 `{cache_dir}/api-cache/`。
- SDK 归档下载缓存写入 `{cache_dir}/sdk-downloads/`。
- SDK index JSON 缓存写入 `{cache_dir}/sdk-index/`。

## API Cache

示例：

```toml
[api_cache]
enable = false
cache_time = 300
```

字段说明：

- `enable`: 是否缓存已知 provider 的元数据响应。
- `cache_time`: 缓存有效期，单位为秒。

> API cache 会缓存 GitHub API、GitLab/Gitea release API、SourceForge files 列表等已知 provider 的 `GET` 响应。缓存文件目录为 `{cache_dir}/api-cache/`。

## GitHub Proxy

示例：

```toml
[ghproxy]
enable = false
host_url = ""
support_api = true
fallbacks = []
```

字段说明：

- `enable`: 是否启用 GitHub URL 重写。
- `host_url`: 主代理地址，例如 `https://ghfast.top/`。
- `support_api`: 启用后也会重写 `api.github.com` 请求。
- `fallbacks`: 主代理失败后按顺序尝试的备用代理地址。

> `proxy_url` 和 `ghproxy` 不是同一种能力。`proxy_url` 是 HTTP 层代理，`ghproxy` 是请求 URL 重写，两者可以同时启用。

## Package 配置

推荐使用 `[packages.<name>]` 管理命名 package。

示例：

```toml
[packages.markview]
repo = "inhere/markview"
target = "~/.local/bin"
tag = "nightly"
asset_filters = ["windows"]
extract_all = true
strip_components = 1
```

常用字段：

- `repo`: package 来源。支持 GitHub 风格 `owner/repo`、直接 URL、SourceForge、已支持的 forge 前缀和 `template:<id>`。
- `target`: 当前 package 的安装目录。
- `system`: 当前 package 的目标平台，格式为 `GOOS/GOARCH`。
- `tag`: 版本 tag 或 release tag 偏好。
- `source_path`: SourceForge files 路径过滤，例如 `stable`。
- `file`: 文件过滤或输出文件名，具体语义取决于命令上下文。
- `asset_filters`: 用于匹配 release asset 的子串列表。
- `download_source`: 下载源码归档，而不是 release asset。
- `extract_all`: 解压选中归档中的全部文件。
- `strip_components`: 解压全部文件时剥离归档内路径前缀层数。
- `is_gui`: 按 GUI package 处理，使用 `gui_target` 相关语义。
- `install_mode`: 可选 GUI 安装模式。配合 `is_gui = true` 设置为 `installer` 时，会强制把普通命名的 `.exe` asset 当作安装器启动；免安装 GUI 应用不要设置该字段。
- `quiet`: 减少当前 package 的输出。
- `upgrade_only`: 仅当 package 已安装时才更新。

也兼容旧版直接配置：

```toml
["inhere/markview"]
tag = "nightly"
```

新配置建议优先使用 `[packages.<name>]`，因为它有明确的本地 package 名称。

### Template Package Source

`repo = "template:<id>"` 用于普通 release provider 以外的独立下载站。它通过配置读取最新版本、渲染下载 URL、可选读取 checksum manifest，然后继续复用普通安装、更新和 installed store 流程。

Claude Code 示例：

```toml
[packages.claude]
repo = "template:claude"
latest_url = "https://downloads.claude.ai/claude-code-releases/latest"
latest_format = "text"
url_template = "https://downloads.claude.ai/claude-code-releases/{version}/{os}-{arch}{libc}/claude{ext}"
os_map = { windows = "win32", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64" }
ext_map = { windows = ".exe", linux = "", darwin = "" }
libc_map = { glibc = "", musl = "-musl" }
checksum_url_template = "https://downloads.claude.ai/claude-code-releases/{version}/manifest.json"
checksum_format = "json"
checksum_json_path = "platforms.{os}-{arch}{libc}.checksum"
install_action = "run-asset"
install_args = ["install", "latest"]
```

YAML latest metadata 示例：

```yaml
version: v1.2.3
released_at: 2026-05-25T10:20:30+08:00
```

```toml
[packages.markview]
repo = "template:markview"
latest_url = "https://example.com/tools/markview/latest.yaml"
latest_format = "yaml"
url_template = "https://example.com/tools/markview/markview-{version}-{os}-{arch}{ext}"
os_map = { windows = "windows", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "amd64", arm64 = "arm64" }
extract_file = "markview"
```

字段说明：

- `latest_url`: 最新版本 metadata 地址。
- `latest_format`: `text`、`json` 或 `yaml`，为空时按 `text` 处理。YAML 会读取 `version` 和可选的 `released_at`。
- `latest_json_path`: `latest_format = "json"` 时用于提取版本的点分路径。
- `version_regex`: 可选正则；有捕获组时使用第一个捕获组，否则使用完整匹配。
- `url_template`: 下载 URL 模板。
- `os_map` / `arch_map` / `ext_map` / `libc_map`: 将当前平台变量映射到下载站命名。
  - template package 中 `{ext}` 默认在 Windows 为 `.exe`，Linux/macOS 为空字符串；只有下载站使用 `.zip`、`.tar.gz` 等其他后缀时才需要设置 `ext_map`。
- `checksum_url_template`: checksum metadata 地址模板。
- `checksum_format`: `text` 或 `json`。
- `checksum_json_path`: `checksum_format = "json"` 时用于提取 checksum 的点分路径，可使用模板变量。
- `checksum_regex`: 可选 checksum 正则提取。
- `install_action = "run-asset"`: 下载和 checksum 校验成功后，执行下载到本地的 asset 本身。
- `install_args`: 传给 `run-asset` 的参数数组。

`url_template`、`checksum_url_template` 和 JSON path 模板支持变量：

- `{name}`: template id。
- `{version}`: latest 或命令行指定的版本。
- `{os}`: 经过 `os_map` 处理后的 OS。
- `{arch}`: 经过 `arch_map` 处理后的 arch。
- `{ext}`: 经过 `ext_map` 处理后的扩展名；未设置 `ext_map` 时 Windows 默认 `.exe`，Linux/macOS 默认空字符串。
- `{libc}`: Linux 下检测到 libc 后经过 `libc_map` 处理的值；非 Linux 或未检测到时为空。

`run-asset` 不是通用 `post_install`。它只执行当前下载并已通过 checksum 校验的 asset，参数必须是数组，不会经过 shell，也不会执行额外脚本。template 的 `latest_url` 和 `checksum_url_template` 是任意站点 metadata，请求会复用 `proxy_url`、`disable_ssl` 等 HTTP 配置，但不会被强制归类为 provider API cache。

## SDK 配置

使用 `[sdk.<name>]` 配置 SDK 归档下载。

也可以通过内置模板快速写入 SDK 配置：

```bash
eget sdk config add --all
eget sdk config add --all --mirror mirror
eget sdk config add jdk --mirror zulu
```

示例：

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

字段说明：

- `aliases`: SDK 别名。例如 `[sdk.go]` 配置 `aliases = ["golang"]` 后，可以使用 `eget sdk install golang@1.22.0`。
- `target`: 安装目录模板。相对路径会基于 `global.sdk_target` 解析。
- `url_template`: 归档下载 URL 模板。
- `index_url`: 远端 HTML 或 JSON index 地址。
- `index_format`: index 格式，通常为 `html` 或 `json`。
- `index_parser`: JSON index 解析器，当前支持 `go-json`、`node-json` 和 `zulu-json`。
- `index_path_prefix`: HTML index 链接路径前缀过滤。
- `filename_pattern`: HTML index 归档文件名模板。
- `strip_components`: 解压时剥离归档内路径前缀层数。
- `os_map`: 将 Go OS 名称映射为 SDK 发布包使用的 OS 名称。
- `arch_map`: 将 Go arch 名称映射为 SDK 发布包使用的 arch 名称。
- `ext_map`: 将 Go OS 名称映射为 SDK 归档扩展名。会覆盖 `global.sdk_ext_map`。

`target`、`url_template`、`filename_pattern` 支持变量：

- `{name}`: SDK 名称。
- `{version}`: 选中的版本。
- `{os}`: 经过 `os_map` 处理后的 OS。
- `{arch}`: 经过 `arch_map` 处理后的 arch。
- `{ext}`: 经过 `ext_map` 处理后的归档扩展名。

HTML index 支持两种常见结构：

- 直接归档文件链接，例如 `go1.22.0.linux-amd64.tar.gz`。
- 版本目录链接，例如 `v20.11.1/`。配置了 `url_template` 后，eget 会从目录名提取版本号，并生成当前平台归档 URL。

SDK 使用细节见 [sdk-usage.md](sdk-usage.md)。

## 安装记录文件

Package 安装记录默认写入：

```text
~/.config/eget/installed.toml
```

SDK 安装记录默认写入：

```text
~/.config/eget/sdk.installed.json
```

设置 `EGET_CONFIG_DIR` 后，这两个记录文件会改为 `{EGET_CONFIG_DIR}/installed.toml` 和 `{EGET_CONFIG_DIR}/sdk.installed.json`。

SDK 安装记录单独存储，是因为 SDK 常见多版本并存，而普通 package 通常是单个当前安装产物。

## 完整示例

更完整的配置示例见 [example.eget.toml](example.eget.toml)，包含 package、Go、Node、Python 和 JDK 试验性 SDK 配置。
