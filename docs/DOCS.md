# Eget Documentation

## Overview

当前 CLI 采用显式子命令结构：

```text
eget <command> --options... arguments...
```

命令集合：

- `install`
- `download`
- `add`
- `uninstall`
- `list`
- `update`
- `config`
- `sdk`

根命令不再承担默认安装行为。

## Runtime Layout

- `cmd/eget/main.go`: 进程入口
- `internal/cli`: `gookit/gcli` 命令注册、参数绑定、输出
- `internal/app`: 用例编排层
- `internal/install`: 查找、检测、下载、校验、提取执行链路
- `internal/config`: 配置文件路径、加载、合并、写回
- `internal/installed`: 安装记录读写
- `internal/sdk`: SDK target 解析、索引缓存、断点续传、解压安装和 SDK 安装记录
- `internal/source/github`: GitHub release/source 查找
- `internal/source/sourceforge`: SourceForge 文件发现与最新版本检查
- `internal/source/forge`: GitLab/Gitea/Forgejo release asset 发现与 latest-version 检查

## Install Flow

`install` 的主流程在 `internal/app/install.go` 与 `internal/install/runner.go`：

1. 解析目标类型
2. 选择 finder
3. 枚举候选资产
4. 按 `system` / `asset_filters` 选择资产
5. 下载内容
6. 执行 SHA-256 自动校验（如果有匹配校验文件）
7. 选择 extractor 并提取
8. 写入 installed store

`install --all` 会读取配置文件中的 `[packages]`，按包名排序后复用单包 install 流程；每个包仍按 `CLI > package > repo > global` 的优先级合并安装选项。`--batch N` 或 `global.batch_concurrency` 大于 `1` 时会启用固定 worker 并发调度，并保持返回结果按包名排序。该模式不接收 target，也不能和 `--add` 同时使用。

解压时，`.7z`、`.rar`、`.msi`、`.cab`、`.iso`、以及 `--extract-all` 的 `.exe` 会优先尝试系统 7z。查找顺序为 `global.sys7z_path`、`PATH` 中的 `7z` / `7zz` / `7za`、最后回退内置 Go 解压实现。系统 7z 处理 `--extract-all` 时会直接执行一次 `7z x` 解压到临时目录，再安全复制到目标目录，不再先执行 `7z l` 枚举文件列表。内置 Go 解压处理 `--extract-all` 时会单次遍历 archive 并流式写出匹配文件，避免先把所有文件内容缓存到内存。`.tar.gz` / `.tgz` / `.tar.xz` / `.txz` / `.tar.bz2` / `.tbz` / `.tar.zst` 继续使用内置 Go 解压流程，以保持 tar 成员选择和路径安全校验稳定。

目标类型支持：

- repo 标识符
- GitHub URL
- Forge target，例如 `gitlab:fdroid/fdroidserver`、`gitea:codeberg.org/forgejo/forgejo`
- SourceForge target，例如 `sourceforge:winmerge`
- 直链 URL
- 本地文件

## SourceForge Flow

`sourceforge:<project>` 目标由 `internal/source/sourceforge` 解析。
可选的 `source_path` 配置会把发现范围限制在项目 files 区域下的指定目录。
SourceForge 返回候选下载 URL 后，`system`、`asset_filters`、`file`、下载、校验、提取和 installed store 记录继续复用普通安装链路。

`query sourceforge:<project>` 复用同一套 SourceForge 发现能力，当前只支持 `latest` 和 `assets`：

- `latest` 通过 SourceForge files 目录推断最新版本。
- `assets` 返回 SourceForge 下载 URL，并从 URL 提取可读文件名。
- `info` 和 `releases` 没有稳定的 SourceForge 元数据抽象，当前明确返回不支持。

## Forge Flow

`gitlab:`、`gitea:`、`forgejo:` 目标由 `internal/source/forge` 解析并调用对应公开 release API。
Forge 后端只返回候选下载 URL；`system`、`asset_filters`、`file`、下载、校验、提取和 installed store 记录继续复用普通安装链路。
第一版不支持私有仓库认证、GitLab/Gitea/Forgejo 的 query/search parity，或从任意网页 URL 自动识别 provider。

## Download Flow

`download` 与 `install` 复用同一条执行链路，只是 app 层会强制 `DownloadOnly=true`，并且不写 installed store。

当目标是远程 URL 时，执行链路会优先检查 `cache_dir` 对应的缓存文件：

- 命中缓存时直接复用，不再发起网络下载
- 未命中时正常下载，并在成功后回写缓存

当前缓存策略是最小实现：

- 缓存键使用 URL hash
- 文件名保留原始 URL 的扩展名，缺省时使用 `.bin`
- 目前不做过期策略、ETag 或 Last-Modified 校验

## SDK Flow

`sdk` 命令由 `internal/cli/sdk_cmd.go` 注册为真正的多层级 gcli 子命令，核心能力在 `internal/sdk`：

```text
eget sdk install <target...>
eget sdk list [name]
eget sdk remove <name@version>
eget sdk index list|show|refresh|clear
```

`sdk install` 支持的 target 格式：

- `go`
- `go@latest`
- `go:latest`
- `go@1.22`
- `go:1.22`
- `go@1.22.0`
- `go:1.22.0`

不支持 `go 1.22.0` 这种空格版本格式，保留命令参数位置给后续一次下载多个 SDK 使用。

安装流程：

1. `ParseTarget` 解析 SDK 名称和版本类型。
2. `ResolveConfig` 合并 `[global]` 与 `[sdk.<name>]`，并处理 alias、`os_map`、`arch_map`、`ext_map`。
3. 精确版本优先使用 `url_template` 直接渲染 URL；`latest` 和前缀版本读取规范化后的 index cache。
4. `DownloadArchive` 下载 SDK 归档到 `{cache_dir}/sdk-downloads/{sdk}/{version}/`。
5. 大文件中断后会保留 `.part` 和 `.meta.json`，下次使用 HTTP Range 断点续传；ETag、Last-Modified 或 size 不匹配时删除 `.part` 重下。
6. 解压到 `{sdk_target}/.eget-tmp/...`，按 `strip_components` 剥离归档内顶层目录。
7. rename 到最终 SDK 目录，并写入 `sdk.installed.json`。

SDK index：

- `index_format = "html"` 解析 HTML 页面中的 `<a href>`，配合 `filename_pattern` 提取版本、OS、arch、ext；对于 `v20.11.1/` 这类版本目录链接，会在配置 `url_template` 后生成当前平台归档 URL。
- JSON index 需要设置内置 `index_parser`，当前支持 `go-json` 和 `node-json`。
- `sdk index refresh` 拉取远端并写入 `{cache_dir}/sdk-index/<name>.json`。
- `sdk index show/list/clear` 只操作本地规范化缓存。

边界：

- `eget sdk` 只负责下载和安装 SDK，不负责 `use`、`env`、`PATH`、shell hook 或 `.xenv.toml`。
- 本机 SDK 环境切换交给 `kite xenv` 等专用工具。安装后可由外部工具扫描 `global.sdk_target` 或读取 `sdk.installed.json`。
- 首版默认示例覆盖 Go 和 Node；其它 SDK 只要能通过配置描述归档命名与索引，也可以使用同一套能力。

## Concurrency

`chunk_concurrency` 控制单个 asset 下载的 HTTP Range 分片并发。

- `0`: 自动，当前在服务端支持 Range 且文件足够大时最多使用 5 个分片。
- `1`: 单连接下载。
- `>1`: 请求的最大分片数。

只有服务端 `HEAD` 响应包含 `Accept-Ranges: bytes` 且能获得有效 `Content-Length` 时才会尝试分片。小文件不分片，当前最小分片大小为 `4 MiB`，至少能拆出两个有效分片才启用并发。chunk 并发对用户展示为一个聚合下载进度条，不展示每个分片的子进度。

`batch_concurrency` 控制 `install --all` 和 `update --all` 的包任务并发。

- `0`: 自动，当前等价于串行。
- `1`: 串行。
- `>1`: 请求的 worker 数。

package 和 repo 配置可以设置 `chunk_concurrency`。`batch_concurrency` 只支持 `[global]` 和 CLI `--batch`，因为它控制整个批处理调度器。

`ignore_update_packages` 只支持 `[global]`，值为 package name 列表。`list --outdated`、`update --check` 和 `update --all` 会跳过这些 package，不发起 latest check，也不会在 `update --all` 中安装更新。

## Add Flow

`add` 不执行下载，只把一个可复用的安装描述写入 `[packages.<name>]`。

默认规则：

- `--name` 未提供时，默认使用 repo basename
- 保存 repo、tag、system、target、file、asset_filters、download_source、extract_all、quiet 等可复用字段

## Uninstall Flow

`uninstall` 按 package name 或 repo 解析目标：

- 命中 package name 时，使用 `[packages.<name>]` 中的 repo
- 否则允许直接传 repo
- 从 installed store 读取 `ExtractedFiles`
- 删除记录中的文件路径
- 清理 installed store 对应 entry

当前不会删除 `[packages.<name>]` 配置项。

## List Flow

`list` 默认只展示 installed store 中的已安装包；设置 `--all` / `-a` 时展示 managed packages 与 installed store 的并集：

- 读取 `[packages.<name>]`
- 按 package name 排序
- 通过 repo 键关联 installed store
- 输出已安装状态；`--all` 时同时输出未安装的 managed package 定义

## Update Flow

`update` 由 `internal/app/update.go` 驱动：

- `update <name>` 先查 `[packages.<name>]`
- `update <target>` 必须能在 config 或 installed store 中找到目标；不存在时提示先使用 `install`
- 单目标 update 会先检查 latest version，只有 outdated 时才执行安装链路
- `update --all` 先检查 managed packages 的已安装版本，只更新有新版本的包
- `update --check` 等同于 `list --outdated`，只检查并列出有新版本的已安装包

CLI 当前还保留：

- `--dry-run`
- `--interactive`

其中 `--all` 和 `--check` 已接通；其余行为以当前实现为准。

## Config Flow

`config` 是 gcli 子命令树，支持这些形式：

- `config init`
- `config list`
- `config ls`
- `config get KEY`
- `config set KEY VALUE`

点路径示例：

- `global.target`
- `packages.fzf.repo`

## Config Model

配置模型定义在 `internal/config/model.go`。

支持的主结构：

```toml
[global]

["owner/repo"]

[packages.name]

[sdk.name]
```

兼容旧 repo section，同时新增 managed packages。

`config --init` 当前生成的默认全局配置为：

```toml
[global]
target = "~/.local/bin"
cache_dir = "~/.cache/eget"
proxy_url = "http://127.0.0.1:7890"
system = ""
sys7z_path = ""
```

路径查找优先级：

1. `EGET_CONFIG`
2. `~/.config/eget/eget.toml`
3. 旧路径 `~/.eget.toml`
4. 平台 fallback 路径

安装选项合并优先级：

```text
CLI > package > repo > global > default
```

目录相关语义：

- `target`: 默认安装目录
- `gui_target`: 免安装 GUI 应用的默认安装目录
- `cache_dir`: 默认下载缓存目录
- `proxy_url`: 全局远程请求代理，优先于环境变量代理并同时作用于 GitHub 查询与远程下载
- `sys7z_path`: 可选系统 7z 可执行文件路径。为空时从 `PATH` 依次查找 `7z`、`7zz`、`7za`
- `sdk_target`: SDK 安装根目录
- `sdk_ext_map`: SDK 默认归档扩展名映射，按 Go OS 名称配置
- `download` 未传 `--to` 时，app 层会把输出目录回退到 `cache_dir`
- `api_cache`: 缓存已知 provider 的元数据 GET 响应，包含 GitHub API、GitLab/Gitea release API 和 SourceForge files 列表；不缓存下载文件
- `chunk_concurrency`: 单文件 HTTP Range 分片并发
- `batch_concurrency`: `install --all` / `update --all` 批处理并发
- `ignore_update_packages`: 跳过检查/更新的 package name 列表

SDK 配置字段：

- `sdk.<name>.aliases`: SDK 别名，例如 `go` 可配置 `golang`
- `sdk.<name>.target`: 安装目录模板，支持 `{name}`、`{version}`、`{os}`、`{arch}`、`{ext}`
- `sdk.<name>.url_template`: SDK 归档 URL 模板
- `sdk.<name>.index_url`: SDK HTML/JSON 索引地址
- `sdk.<name>.index_format`: `html` 或 `json`
- `sdk.<name>.index_parser`: JSON 索引解析器，当前支持 `go-json`、`node-json`
- `sdk.<name>.index_path_prefix`: HTML index 链接前缀过滤
- `sdk.<name>.filename_pattern`: HTML index 文件名模板
- `sdk.<name>.strip_components`: 解压时剥离路径前缀层数
- `sdk.<name>.os_map` / `arch_map` / `ext_map`: SDK 级别平台名和归档扩展名映射

默认写入路径：

- 配置文件默认写入 `~/.config/eget/eget.toml`
- installed store 默认写入 `~/.config/eget/installed.toml`
- SDK installed store 默认写入 `~/.config/eget/sdk.installed.json`

## Installed Store

安装记录抽离到 `internal/installed`，用于：

- 记录安装结果
- 为资产回退选择提供历史信息
- 支撑 update 相关流程

SDK 安装记录独立保存在 `sdk.installed.json`，避免污染 package installed store 的单版本工具语义。结构按 SDK 名称和版本存储，支持同一 SDK 多版本并存。

## Option Surface

当前 CLI 已暴露的核心安装选项：

- `--tag`
- `--system`
- `--to`
- `--file`（可选择归档内文件；对 7z 可读取的 `.exe` 安装包使用时需要系统 7z）
- `--asset`
- `--source`
- `--chunk`
- `--all`（仅 `install`，安装 `[packages]` 中的全部托管包）
- `--batch`（仅 `install --all` / `update --all`）
- `--extract-all` / `--ea`
- `--fallback-versions`（仅 `install` / `download`，SourceForge 目标在最新版本目录缺少匹配资产时扫描旧版本目录）
- `--gui`
- `--quiet`

`sdk` 额外支持：

- `sdk install --force`
- `sdk list --json`
- `sdk index list --json`
- `sdk index refresh --all`
- `sdk index clear --all`

GUI 相关配置：

- `global.gui_target`: portable GUI application target directory
- `packages.<name>.is_gui`: marks a package as GUI
- GUI installer mode records `install_mode = "installer"` after process start succeeds

`update` 额外支持：

- `--all`
- `--check`
- `--dry-run`
- `--interactive`

`query` 额外支持：

- GitHub target 的 `latest`、`releases`、`assets`、`info`
- SourceForge target 的 `latest`、`assets`

## Constraints

由于 CLI 解析模型，参数顺序必须遵循：

```text
CMD --OPTIONS... ARGUMENTS...
```

支持：

```text
eget install --tag nightly inhere/markview
```

不支持：

```text
eget install inhere/markview --tag nightly
```

## Verification

常用验证命令：

```bash
go test ./internal/app -v
go test ./internal/cli -v
go test ./...
make build
make test
```
