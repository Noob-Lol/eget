# SDK 下载安装设计

## 目标

为 `eget` 增加 SDK 多版本下载安装能力，优先覆盖 Go、Node 这类官方或镜像站提供可解压二进制 SDK 包的运行时。

首版目标：

- 支持 `eget sdk install <target>` 安装指定 SDK 版本。
- 支持 `eget sdk list` 查看已安装 SDK 版本。
- 支持 `eget sdk remove <target>` 删除已安装 SDK 版本。
- 支持 `eget sdk index` 管理解析后的 SDK 索引缓存 JSON。
- 支持通过配置定义 SDK 下载 URL 模板、版本索引 URL、OS/Arch/扩展名映射和安装目录模板。
- 支持同一个 SDK 多版本共存。
- 复用现有下载能力：代理、缓存、HTTP Range 分片并发、SSL 开关、进度条。
- 支持 SDK 下载断点续传，避免大文件中断后重新下载。
- 复用现有归档处理能力：zip、tar.gz、tar.xz、tar.bz2、7z 等。

## 非目标

首版不实现：

- 自动修改 `PATH`、shell profile、系统环境变量。
- SDK shim 管理，例如 `go` 自动指向当前版本。
- `sdk use` 或 `current` 版本切换。
- `sdk env`、`sdk script`、shell hook、目录自动切换、`.xenv.toml` 写入。
- Python 全平台二进制 SDK 安装承诺。Python 官方分发模型不统一，首版只保留配置扩展能力。
- 对每个 SDK 内部包管理器做额外初始化，例如 npm global prefix、Go env、pip 配置。
- 复杂 semver 约束表达式，例如 `^1.21`、`>=20 <21`。

## 与 kite xenv 的边界

项目外已有独立的 `kite xenv` CLI 负责本机 SDK 环境管理。`xenv` 的定位类似 `mise`、`vfox`、`asdf`，已经覆盖：

- SDK 激活和取消激活：`kite xenv use` / `kite xenv unuse`。
- 全局、当前 shell 会话、项目目录 `.xenv.toml` 三种作用域。
- 环境变量和 `PATH` 管理。
- shell hook 和目录级自动加载。
- 本地 SDK 索引：`kite xenv tools index`。

因此 `eget sdk` 的边界应明确为 SDK acquisition layer：

```text
eget sdk: download -> resume/cache -> extract -> record installed SDKs
kite xenv: index local SDKs -> activate -> manage PATH/env/project state
```

`eget` 不直接实现 `use`、`env`、`script`、shell hook、shim、项目自动切换，也不直接写入 `~/.xenv/tools.local.json` 或 `.xenv.toml`。这些状态文件由 `xenv` 自己维护，避免两个工具同时成为环境状态的权威来源。

推荐集成方式是目录约定和显式索引：

```bash
eget sdk install go@1.22.0
kite xenv tools index
xenv use go:1.22
```

`eget sdk.installed.json` 只记录 `eget` 安装过的 SDK，服务于 `eget sdk list/remove`、缓存复用和审计；`xenv` 的 `tools.local.json` 是环境激活权威来源。两者允许部分重复，但职责不同。

后续可以考虑在 `eget` 中提供可选提示或文档说明，提示用户安装后运行 `kite xenv tools index`。首版不自动调用 `kite`，避免让 `eget` 对外部 CLI 产生运行时依赖。

## 命令设计

### CLI 前置要求

当前 `eget` CLI 基于 `github.com/gookit/goutil/cflag/capp` 构建，适合当前较简单的一级命令模型，但不适合 `sdk install/list/remove/index` 这种多层级命令。SDK 命令实现前应先把 CLI 迁移到 `gookit/gcli`。

迁移原则：

- 先完成现有命令在 `gookit/gcli` 下的等价迁移，再新增 `sdk` 命令。
- 不在现有 `cflag/capp` handler 中通过手写解析 `sdk install`、`sdk list`、`sdk index` 等字符串来模拟多层级命令。
- `sdk` 应作为真正的父命令，`install`、`list`、`remove`、`index` 作为子命令注册。
- CLI 迁移本身应作为独立任务，不和 SDK 下载实现混在同一个实现步骤中。

新增顶层命令：

```bash
eget sdk install <target...>
eget sdk list
eget sdk remove <target>
eget sdk index <action> [sdk]
```

### install 目标格式

`eget sdk install <target...>` 的每个目标只支持 `name[@version]` 或 `name[:version]` 形式，不支持 `go 1.21.1` 这类空格分隔版本。这样可以为后续一次安装多个 SDK 预留参数形态，并与 `kite xenv` 常见的 `go:1.22` 版本写法保持兼容。

```text
go@1.21.1
go:1.21.1
go
go@latest
go:latest
go@1.21
go:1.21
node@20.11.1
node:20.11.1
```

解析规则：

| 输入 | 解析结果 |
| --- | --- |
| `go@1.21.1` | sdk=`go`, version=`1.21.1` |
| `go:1.21.1` | sdk=`go`, version=`1.21.1` |
| `go` | sdk=`go`, version=`latest` |
| `go@latest` | sdk=`go`, version=`latest` |
| `go:latest` | sdk=`go`, version=`latest` |
| `go@1.21` | sdk=`go`, version prefix=`1.21` |
| `go:1.21` | sdk=`go`, version prefix=`1.21` |
| `node@20.11.1` | sdk=`node`, version=`20.11.1` |
| `node:20.11.1` | sdk=`node`, version=`20.11.1` |

CLI 层需要允许 `install` 接收一个或多个目标参数：

```bash
eget sdk install go
eget sdk install go@1.21.1
eget sdk install go:1.21.1
eget sdk install go@1.21.1 node:20.11.1
```

非法格式：

- `go@`
- `go:`
- `@1.21.1`
- `:1.21.1`
- `go 1.21.1`
- `go@1.21.1 1.22.0`

### 版本解析语义

版本输入分为三类：

| 类型 | 示例 | 语义 |
| --- | --- | --- |
| latest | `go` / `go@latest` / `go:latest` | 从 SDK 索引里选择最新稳定版本 |
| exact | `go@1.21.1` / `go:1.21.1` | 安装精确版本 |
| prefix | `go@1.21` / `go:1.21` | 从 SDK 索引里选择匹配前缀的最新 patch，例如 `1.21.13` |

版本规范化：

- 用户输入 `1.21.1`，内部版本字段保存为 `1.21.1`。
- URL 模板里的 `{version}` 默认使用不带 SDK 前缀的版本号。
- 如 Go URL 需要 `go1.21.1`，模板应写 `go{version}`。
- 如 Node URL 需要 `v20.11.1`，模板应写 `v{version}`。

这能避免用户输入和模板都带前缀时生成 `gogo1.21.1` 或 `vv20.11.1`。

### list

基础输出：

```bash
eget sdk list
```

显示字段：

```text
Name  Version  Path                    Installed At
go    1.21.1   ~/sdks/gosdk/go1.21.1   2026-05-14 10:11:12
node  20.11.1  ~/sdks/nodejs/node20.11.1 2026-05-14 10:15:12
```

`eget sdk list` 不显示 `Current` 字段。当前激活版本属于 `kite xenv` 的职责，用户可通过 `kite xenv list activity` 或 `kite xenv tools list` 查看。

可选参数建议：

```bash
eget sdk list go
eget sdk list --json
```

### remove

```bash
eget sdk remove go@1.21.1
```

删除行为：

- 从安装记录中查找 sdk/version。
- 删除记录里的 `path` 目录。
- 删除 `sdk.installed.json` 中对应条目。
- 如果目录不存在但记录存在，仍删除记录，并输出 warning。
- 如果记录不存在，返回错误。

安全约束：

- 只允许删除记录中 `path` 指向的目录。
- 删除前必须确认目标路径在 `global.sdk_target` 或绝对配置 `target` 目录之下。
- Windows 下使用 `Remove-Item -LiteralPath` 对应的 Go `os.RemoveAll` 前也要完成路径归一化校验。

### index

`index` 用于管理解析后的索引缓存 JSON。建议支持：

```bash
eget sdk index list
eget sdk index show go
eget sdk index refresh go
eget sdk index refresh --all
eget sdk index clear go
eget sdk index clear --all
```

语义：

- `list`：列出已有索引缓存，显示 sdk、版本数量、来源 URL、更新时间。
- `show <sdk>`：输出规范化后的索引 JSON。
- `refresh <sdk>`：重新请求 `index_url`，解析并写入缓存。
- `refresh --all`：刷新所有配置了 `index_url` 的 SDK。
- `clear <sdk>`：删除指定 SDK 索引缓存。
- `clear --all`：删除全部 SDK 索引缓存。

安装时索引使用策略：

- exact 版本如果 `url_template` 足够生成 URL，可以不强制读取索引。
- latest / prefix 必须读取索引。
- 如果 `index_url` 存在，安装时优先使用索引解析出的下载 URL。
- 如果索引缓存不存在或过期，自动刷新。
- 如果刷新失败但缓存存在，允许使用旧缓存，并输出 warning。
- 如果刷新失败且缓存不存在，返回错误。

## 配置设计

### 全局配置

`global.sdk_download_ext` 改名为 `global.sdk_ext_map`。

```toml
[global]
sdk_target = "~/sdks"
sdk_ext_map = {
  windows = "zip",
  linux = "tar.gz",
  darwin = "tar.gz"
}
```

字段语义：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `sdk_target` | string | SDK 默认安装根目录 |
| `sdk_ext_map` | map[string]string | 按 OS 映射默认下载扩展名 |

### SDK 配置

SDK 级别 `download_ext` 改名为 `ext_map`。

```toml
[sdk.go]
aliases = ["golang"]
target = "gosdk/go{version}"
url_template = "https://mirrors.aliyun.com/golang/go{version}.{os}-{arch}.{ext}"
index_url = "https://mirrors.aliyun.com/golang/"
index_format = "html"
strip_components = 1
ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }

[sdk.node]
aliases = ["nodejs"]
target = "nodejs/node{version}"
url_template = "https://cdn.npmmirror.com/binaries/node/v{version}/node-v{version}-{os}-{arch}.{ext}"
index_url = "https://registry.npmmirror.com/binary.html"
index_format = "html"
index_path_prefix = "/binaries/node/"
strip_components = 1
os_map = { windows = "win", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64", 386 = "x86" }
ext_map = { windows = "zip", linux = "tar.xz", darwin = "tar.gz" }
```

字段语义：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `aliases` | []string | 否 | SDK 别名，例如 `golang`、`nodejs` |
| `target` | string | 是 | 安装目录模板。相对路径基于 `global.sdk_target` |
| `url_template` | string | 条件必填 | 下载 URL 模板 |
| `index_url` | string | 条件必填 | 版本索引页面或接口 |
| `index_format` | string | 否 | `auto` / `json` / `html`，默认 `auto` |
| `index_path_prefix` | string | 否 | HTML 页面中过滤链接路径的前缀 |
| `strip_components` | int | 否 | 解压时剥离归档路径前 N 段，默认 0 |
| `os_map` | map[string]string | 否 | `runtime.GOOS` 到下载命名的映射 |
| `arch_map` | map[string]string | 否 | `runtime.GOARCH` 到下载命名的映射 |
| `ext_map` | map[string]string | 否 | SDK 级扩展名映射，覆盖 `global.sdk_ext_map` |

模板变量：

| 变量 | 示例 |
| --- | --- |
| `{name}` | `go` |
| `{version}` | `1.21.1` |
| `{os}` | `windows` / `linux` / `win` |
| `{arch}` | `amd64` / `x64` |
| `{ext}` | `zip` / `tar.gz` / `tar.xz` |

模板变量解析顺序：

1. `version` 来自用户输入或索引解析后的规范版本。
2. `os` 先取 `sdk.os_map[runtime.GOOS]`，不存在时取 `runtime.GOOS`。
3. `arch` 先取 `sdk.arch_map[runtime.GOARCH]`，不存在时取 `runtime.GOARCH`。
4. `ext` 先取 `sdk.ext_map[effectiveOS]`，不存在时取 `global.sdk_ext_map[runtime.GOOS]`，再不存在时报错。

注意：`ext_map` 的 key 建议使用 Go 原始 OS，例如 `windows`、`linux`、`darwin`，不要使用映射后的 `win`。这样配置和当前平台保持一致。

## 索引解析设计

### 规范化索引 JSON

无论远端是 HTML 还是 JSON，都解析成统一缓存结构：

```json
{
  "schema": 1,
  "sdk": "go",
  "source_url": "https://mirrors.aliyun.com/golang/",
  "fetched_at": "2026-05-14T10:11:12+08:00",
  "items": [
    {
      "version": "1.21.1",
      "stable": true,
      "files": [
        {
          "os": "windows",
          "arch": "amd64",
          "ext": "zip",
          "url": "https://mirrors.aliyun.com/golang/go1.21.1.windows-amd64.zip",
          "filename": "go1.21.1.windows-amd64.zip"
        }
      ]
    }
  ]
}
```

### HTML 解析

用户明确要求 HTML 必须支持，因为当前计划中的两个 mirror 都是 HTML 页面：

- Go mirror: `https://mirrors.aliyun.com/golang/`
- Node mirror index: `https://registry.npmmirror.com/binary.html`

HTML 解析建议：

- 使用标准库 `golang.org/x/net/html`，不要用正则直接解析完整 HTML。
- 提取所有 `<a href="...">`。
- 将相对链接解析为绝对 URL。
- 使用 `index_path_prefix` 过滤无关链接。
- 对每个链接的文件名应用 SDK 级文件名解析规则。

Go 文件名解析规则：

```text
go{version}.{os}-{arch}.{ext}
go1.21.1.windows-amd64.zip
go1.21.1.linux-amd64.tar.gz
```

Node 文件名解析规则：

```text
node-v{version}-{os}-{arch}.{ext}
node-v20.11.1-win-x64.zip
node-v20.11.1-linux-x64.tar.xz
```

为避免每个 SDK 都写死规则，建议配置增加可选字段：

```toml
filename_pattern = "go{version}.{os}-{arch}.{ext}"
```

首版内置支持 `go` 和 `node` 两种 pattern。是否开放通用 pattern 编译见“不明确项”。

### JSON 解析

JSON 首版支持两种：

- Go 官方 `https://go.dev/dl/?mode=json`
- Node 官方 `https://nodejs.org/dist/index.json`

JSON parser 可以内置 provider：

```toml
index_parser = "go-json"
index_parser = "node-json"
```

如果 `index_parser` 为空：

- `index_format = html` 时走 HTML 链接解析。
- `index_format = json` 时如果结构不是内置支持格式，返回清晰错误。

## 安装记录设计

建议使用新的 `sdk.installed.json`，比复用现有 `installed.toml` 更合适。

原因：

- SDK 是 `name + version` 多版本模型；现有 installed store 是 package/repo 单版本模型。
- SDK 记录天然需要按 name 聚合多个版本。
- JSON 更适合保存解析后的结构化数组，也方便 `sdk list --json` 复用。
- 避免污染现有 `list/update/uninstall` 的 package 语义。

默认路径：

```text
~/.config/eget/sdk.installed.json
```

如果设置了 `XDG_CONFIG_HOME`：

```text
$XDG_CONFIG_HOME/eget/sdk.installed.json
```

建议记录结构：

```json
{
  "schema": 1,
  "installed": {
    "go": {
      "versions": {
        "1.21.1": {
          "name": "go",
          "version": "1.21.1",
          "path": "/home/me/sdks/gosdk/go1.21.1",
          "url": "https://mirrors.aliyun.com/golang/go1.21.1.linux-amd64.tar.gz",
          "filename": "go1.21.1.linux-amd64.tar.gz",
          "os": "linux",
          "arch": "amd64",
          "ext": "tar.gz",
          "installed_at": "2026-05-14T10:11:12+08:00",
          "strip_components": 1
        }
      }
    }
  }
}
```

不记录 `current` 字段。SDK 当前激活状态属于 `kite xenv`，`eget` 的 installed store 只记录下载和安装事实。

## 索引缓存文件设计

索引缓存与安装记录分开。

默认目录：

```text
~/.cache/eget/sdk-index/
```

文件：

```text
~/.cache/eget/sdk-index/go.json
~/.cache/eget/sdk-index/node.json
```

如果用户配置了 `global.cache_dir`，则使用：

```text
{cache_dir}/sdk-index/go.json
{cache_dir}/sdk-index/node.json
```

缓存过期策略：

- 复用 `api_cache.cache_time` 作为索引缓存 TTL。
- 如果 `api_cache.enable = false`，仍可以写入 SDK 索引缓存，但安装时默认每次刷新。这里需要确认。
- 建议新增 `sdk_index_cache_time` 会让配置变复杂，首版不加。

## 安装流程

安装主流程：

```text
parse target
-> resolve sdk config by name or alias
-> resolve version
   -> exact: direct or index lookup
   -> latest/prefix: index lookup required
-> resolve platform mapping os/arch/ext
-> resolve download URL
   -> prefer index file URL if available
   -> otherwise render url_template
-> resolve install path
-> download archive to resumable cache file
-> verify optional checksum if index provides checksum
-> extract to temp dir
-> apply strip_components
-> move temp dir to final install path
-> record sdk.installed.json
```

建议使用临时目录，避免安装失败后留下半成品：

```text
{sdk_target}/.eget-tmp/{name}-{version}-{timestamp}
```

成功后再 rename 到最终目录：

```text
{sdk_target}/gosdk/go1.21.1
```

如果最终目录已存在：

- 默认返回错误。
- 后续可增加 `--force` 覆盖。

## 断点续传设计

SDK 压缩包通常较大，首版必须支持断点续传。SDK 下载不要复用当前 `InstallRunner.downloadBody` 的内存 buffer 模式，而应下载到磁盘缓存文件，再从缓存文件解压。

缓存文件建议：

```text
{cache_dir}/sdk-downloads/{sdk}/{version}/{filename}
{cache_dir}/sdk-downloads/{sdk}/{version}/{filename}.part
{cache_dir}/sdk-downloads/{sdk}/{version}/{filename}.meta.json
```

下载状态：

- 完整文件存在且 meta 匹配时，直接复用。
- `.part` 文件存在且服务端支持 Range 时，从当前大小继续下载。
- `.part` 文件存在但服务端不支持 Range 时，删除 `.part` 后重新下载。
- 下载完成后校验大小；如果有 checksum，再校验 checksum。
- 校验通过后将 `.part` 原子 rename 为完整文件。
- 校验失败时保留还是删除 `.part` 需要确认；建议删除，避免下次继续基于错误内容追加。

meta 文件建议结构：

```json
{
  "schema": 1,
  "url": "https://mirrors.aliyun.com/golang/go1.21.1.linux-amd64.tar.gz",
  "filename": "go1.21.1.linux-amd64.tar.gz",
  "size": 66377805,
  "etag": "\"abc123\"",
  "last_modified": "Wed, 01 Nov 2023 12:00:00 GMT",
  "updated_at": "2026-05-14T10:11:12+08:00"
}
```

续传条件：

- 先发 `HEAD` 探测 `Accept-Ranges`、`Content-Length`、`ETag`、`Last-Modified`。
- `.part` size 必须小于远端 `Content-Length`。
- 如果 meta 中的 `ETag` 或 `Last-Modified` 与远端不一致，删除 `.part` 后重新下载。
- 续传请求使用 `Range: bytes={partSize}-`。
- 如果服务端返回 `206 Partial Content`，追加写入 `.part`。
- 如果服务端返回 `200 OK`，说明服务端没有按 Range 续传，必须重建 `.part` 后完整下载。

和分片并发的关系：

- 断点续传优先保证正确性。
- 首版建议续传场景使用单连接追加下载。
- 无 `.part` 文件的新下载仍可使用现有 HTTP Range 分片并发下载。
- 如果要让分片并发也支持断点续传，需要持久化每个 chunk 的完成状态，首版不做。

对现有下载能力的影响：

- 新增 SDK 专用 resumable downloader，不改变 `install/download/update` 的现有行为。
- 后续如果验证稳定，再考虑把 resumable downloader 下沉到通用下载层。

## 文件与模块建议

建议新增：

```text
internal/sdk/model.go
internal/sdk/target.go
internal/sdk/config.go
internal/sdk/template.go
internal/sdk/index.go
internal/sdk/html_index.go
internal/sdk/json_index.go
internal/sdk/download.go
internal/sdk/store.go
internal/sdk/service.go
internal/app/sdk.go
internal/cli/sdk_cmd.go
```

职责：

| 文件 | 职责 |
| --- | --- |
| `internal/sdk/model.go` | SDK 配置、索引、安装记录的数据结构 |
| `internal/sdk/target.go` | `go` / `go@latest` / `go:latest` / `go@1.21` / `go:1.21` / `go@1.21.1` / `go:1.21.1` 解析 |
| `internal/sdk/config.go` | 从全局配置和 `[sdk]` section 合并 SDK 配置 |
| `internal/sdk/template.go` | `{version}`、`{os}`、`{arch}`、`{ext}` 模板渲染 |
| `internal/sdk/index.go` | 索引加载、刷新、版本选择 |
| `internal/sdk/html_index.go` | HTML 链接提取和文件名解析 |
| `internal/sdk/json_index.go` | Go/Node 官方 JSON 解析 |
| `internal/sdk/download.go` | SDK 可续传下载、`.part` 和 meta 文件管理 |
| `internal/sdk/store.go` | `sdk.installed.json` 读写 |
| `internal/sdk/service.go` | install/list/remove/index 用例编排 |
| `internal/app/sdk.go` | app 层 service 包装，统一配置加载和 install options |
| `internal/cli/sdk_cmd.go` | CLI 命令和参数定义 |

首版先放在 `internal/sdk`，保证 API 可以随实现快速调整。待 SDK 下载、索引解析、断点续传和解压流程稳定后，再预留迁移或包装为公开包：

```text
pkg/sdk
```

`pkg/sdk` 的目标不是暴露 CLI 或环境激活能力，而是给其他 Go 程序复用 SDK 获取能力，例如 `kite xenv` 可以在未来直接调用它实现 `tools install`。公开 API 应聚焦：

- 解析 SDK target，例如 `go@1.22.0`、`go:1.22.0`。
- 加载 SDK 下载配置。
- 解析 HTML/JSON 索引。
- 选择版本和平台文件。
- 可续传下载到本地缓存。
- 解压到目标目录并支持 `strip_components`。

不应放入 `pkg/sdk` 的能力：

- 修改 `PATH` 或环境变量。
- shell hook 生成。
- `.xenv.toml` 写入。
- xenv 状态文件读写。
- SDK 激活策略。

长期演进关系：

```text
eget CLI -> internal/app/sdk -> internal/sdk
kite xenv -> pkg/sdk                 # 未来可选
pkg/sdk -> no dependency on xenv
```

只有在 `internal/sdk` 的数据模型稳定后才开放 `pkg/sdk`，避免早期把未定型的配置结构变成公共兼容负担。

需要修改：

```text
internal/config/model.go
internal/config/gookit.go
internal/config/merge.go
internal/cli/app.go
internal/cli/handlers.go
internal/cli/wiring.go
docs/example.eget.toml
docs/DOCS.md
README.md
README.zh-CN.md
docs/TODO.md
```

## 复用现有下载/解压能力

普通下载能力可以复用现有 HTTP client、代理和 Range 探测代码，但 SDK 下载需要新增可续传落盘 downloader。

需要注意：

- 当前 `InstallRunner.downloadBody` 是 runner 私有方法，SDK service 如果直接用会耦合 install runner。
- 当前 `InstallRunner.downloadBody` 会把完整内容读入内存，不适合 SDK 大文件。
- 推荐先在 `internal/sdk/download.go` 实现 SDK 专用 downloader，复用 `install.ClientOptions` 和底层 HTTP getter 选项。
- SDK 解压需要整目录安装，不是选择单个二进制文件，现有 `ExtractAllTo` 可以复用，但需要补 `strip_components`。

`strip_components` 可以这样设计：

- 在 `ArchiveExtractor.ExtractAllTo` 增加 options 会影响现有接口，不建议直接改签名。
- 新增 `ExtractAllToWithOptions(data, output, ArchiveExtractOptions)`。
- `ExtractAllTo` 保持旧行为，内部调用新方法，避免影响现有 install/download。

示例：

```go
type ArchiveExtractOptions struct {
    StripComponents int
}
```

## 测试策略

核心单元测试：

- target parser：
  - `go`
  - `go@latest`
  - `go:latest`
  - `go@1.21`
  - `go:1.21`
  - `go@1.21.1`
  - `go:1.21.1`
  - 非法输入 `go@`
  - 非法输入 `go:`
  - 非法输入 `@1.21.1`
  - 非法输入 `:1.21.1`
  - 非法输入 `go 1.21.1`
- template renderer：
  - Go 默认映射
  - Node `os_map` / `arch_map`
  - `ext_map` 覆盖 global `sdk_ext_map`
- HTML index parser：
  - 阿里云 Go mirror 风格链接
  - npmmirror Node 页面风格链接
  - 相对 URL 转绝对 URL
  - 忽略无关链接
- version resolver：
  - latest 选最大稳定版本
  - prefix `1.21` 选最大 patch
  - exact 找不到时报错
- store：
  - 记录多个版本
  - 删除一个版本不影响其他版本
  - 空 store 自动初始化
- remove safety：
  - 目标路径在 `sdk_target` 下允许删除
  - 目标路径逃逸时报错
- strip components：
  - `go/bin/go` strip 1 后写到 `bin/go`
  - strip 过大时报错或跳过空路径，需明确策略
- resumable download：
  - 已有完整缓存文件时不重新下载
  - `.part` 和 meta 匹配时使用 `Range` 续传
  - meta 的 `ETag` 不匹配时删除 `.part` 后重下
  - 服务端返回 `200 OK` 时重建 `.part` 并完整下载
  - 服务端不支持 Range 时删除 `.part` 后重下

集成测试建议用本地 HTTP server：

- 提供 HTML index。
- 提供 zip/tar.gz SDK 测试包。
- 执行 `sdk install`。
- 断言目标目录结构和 `sdk.installed.json`。

完成 MVP 主链路后需要运行：

```bash
go test ./...
```

## 兼容与迁移

这是 v0 开发期，SDK 功能没有历史兼容负担。

但应避免破坏现有 package 安装功能：

- 不修改现有 `installed.toml` 语义。
- 不改变 `install/download/update` 参数行为。
- `sdk` 命令使用独立 store 和独立 service。

## 不明确项

以下问题需要在实现前或实现中进一步确认：

1. `sdk index` 子命令的完整参数是否按本设计采用 `list/show/refresh/clear`，还是只需要最小的 `refresh/clear`。
2. `eget sdk list` 是否需要显示 `Current` 字段。建议不显示；SDK 激活状态属于 `kite xenv`，不属于 `eget`。
3. `api_cache.enable = false` 时，SDK 索引缓存是否仍写入并允许 `sdk index show` 查看。建议仍写入，但安装时按 TTL 策略决定是否使用。
4. exact 版本安装时，如果 `url_template` 可生成 URL，是否必须校验索引中存在该版本。建议不强制，这样用户能安装索引页面暂未解析到但 URL 存在的版本。
5. HTML 索引是否只支持 `<a href>` 文件链接，还是需要支持页面脚本内嵌 JSON。首版建议只支持 `<a href>`。
6. `filename_pattern` 是否要开放成通用配置。建议首版先内置 Go/Node 两种解析器，等第三个 SDK 出现再抽象。
7. `strip_components` 过大时如何处理。建议如果文件路径被剥离为空则跳过目录项；如果所有文件都被跳过则报错。
8. `remove` 是否默认直接删除，还是增加确认提示。建议 CLI 默认提示，增加 `--yes` 跳过；但这会增加交互逻辑。
9. 安装目录已存在时是否需要 `--force`。建议首版支持 `--force`，否则用户手动清理半成品不方便。
10. 是否需要安装后输出环境变量提示，例如 Go 的 `GOROOT`、Node 的 `bin` 路径。首版可以只输出安装路径和 bin 路径，不修改环境。
11. Windows 下 Node 官方也有 `.msi`，首版是否只支持 zip 可解压包。建议只支持 zip，避免 GUI installer 进入 SDK 主链路。
12. Python 是否在文档中标记为“可通过自定义配置尝试”还是完全不提。建议说明 Python 官方分发不统一，首版不作为内置 SDK。
13. `sdk.installed.json` 是否允许用户手改。建议按机器生成文件处理，不鼓励手改；配置仍在 `eget.toml`。
14. 多进程同时执行 `sdk install` 是否需要文件锁。现有 store 看起来没有锁，首版可不加，但存在并发写覆盖风险。
15. checksum 支持是否纳入首版。Go index 能提供 checksum，Node SHASUMS 需要额外请求；建议首版先不做强校验，后续扩展。
16. 断点续传校验失败时是否保留 `.part` 供排查。建议删除，优先保证下一次安装成功率。
17. 何时开放 `pkg/sdk`。建议等 `internal/sdk` 完成 Go/Node、HTML index、断点续传和 strip components 后，再基于稳定内部接口包装公开 API。
18. CLI 迁移到 `gookit/gcli` 的范围。建议先做现有命令的等价迁移和测试，再实现 SDK 多层级命令，避免 SDK 实现依赖临时硬编码分发。

## 建议的 MVP

最小可交付版本：

- 命令：`sdk install/list/remove/index refresh/index clear/index show`。
- 内置 SDK：Go、Node。
- 前置：CLI 从 `gookit/goutil/cflag/capp` 迁移到 `gookit/gcli`，SDK 命令使用真正的多层级命令注册。
- 支持 target：`go`、`go@latest`、`go:latest`、`go@1.21`、`go:1.21`、`go@1.21.1`、`go:1.21.1`，不支持 `go 1.21.1`。
- 支持多个 target：`eget sdk install go@1.21.1 node:20.11.1`。
- 支持 HTML index。
- 支持 SDK 下载断点续传。
- 支持 `sdk.installed.json`。
- 支持 `sdk_ext_map` / `ext_map`。
- 支持 `os_map` / `arch_map`。
- 支持 `strip_components = 1`。
- 不做 PATH 修改、不做版本切换、不做 `env/script/use`，不做 Python 内置。
- 文档说明与 `kite xenv` 的推荐配合方式：`eget sdk install` 后运行 `kite xenv tools index`，再由 `xenv use` 激活。
