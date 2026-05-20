# URL Template Package Source 设计

## 目标

为 `eget` 增加一类一等 package 来源，用于支持不发布在 GitHub、GitLab、Gitea/Forgejo、SourceForge 上的独立下载站。

典型目标是 Claude Code 安装脚本使用的发布模型：

```text
latest version:  https://downloads.claude.ai/claude-code-releases/latest
manifest:        https://downloads.claude.ai/claude-code-releases/{version}/manifest.json
windows binary:  https://downloads.claude.ai/claude-code-releases/{version}/win32-x64/claude.exe
linux binary:    https://downloads.claude.ai/claude-code-releases/{version}/linux-x64/claude
linux musl:      https://downloads.claude.ai/claude-code-releases/{version}/linux-x64-musl/claude
darwin binary:   https://downloads.claude.ai/claude-code-releases/{version}/darwin-arm64/claude
```

用户配置一次后，应能继续使用普通 package 管理命令：

```bash
eget install claude
eget list --outdated
eget update claude
eget update --all
eget show claude
eget uninstall claude
```

## 非目标

首版不实现通用脚本运行器，也不让 package 配置变成构建系统。

不新增：

- `post_install`、`pre_install`、shell hook、PowerShell 脚本执行或任意 shell 命令执行。
- PATH、shell profile、系统环境变量修改。
- 多版本 SDK 语义。SDK 仍属于 `eget sdk`；URL template package 是普通单当前版本 package。
- 任意网页自动抓取和解析。
- 新的 lock file。
- 新的独立命令族。

像 Claude Code 这种脚本最后会执行下载后的临时二进制：

```bash
claude install latest
```

因此如果目标是完整安装 Claude Code，首版需要支持一个受控的 `run-asset` installer action。它不是通用 `post_install`：只能执行刚下载并校验通过的 asset 本身，参数使用数组传递，不经过 shell，不允许任意命令字符串。

## 推荐方案

新增 package source kind：`template`。

不建议把它放到 `sdk`，因为 Claude Code 这类包不是多版本 SDK，而是普通工具包，只需要一个当前安装版本，并应参与现有 `install/list/update/uninstall` 主链路。

不建议继续使用固定直接 URL 表达，因为固定 URL 没有“最新版本”查询能力，`update` 无法可靠判断是否需要重新安装。

该来源复用 SDK 已经使用过的模板和平台映射概念：

- `url_template`
- `os_map`
- `arch_map`
- `ext_map`
- `{name}`、`{version}`、`{os}`、`{arch}`、`{ext}`

在此基础上只增加普通 package 所需的 latest 发现和 checksum manifest 字段。

### Source Kind 命名

推荐使用：

```toml
repo = "template:claude"
```

候选名称对比：

| 前缀 | 评价 |
| --- | --- |
| `template:` | 推荐。短、清晰，表达“来源由配置模板渲染”，且不和直接 URL 混淆。 |
| `url-template:` | 语义准确但偏长，配置里已经有 `url_template` 字段，重复度较高。 |
| `site:` | 可读性好，但更像“任意网站抓取”，容易暗示后续支持网页解析。 |
| `custom:` | 太宽泛，不说明机制。 |
| `url:` | 容易和真实 URL scheme 混淆，例如 `https://...`。 |
| `tpl:` | 太缩写，不适合作为用户配置主入口。 |

因此首版以 `template:<id>` 作为规范写法，不提供别名前缀，避免配置风格发散。

## 配置模型

Claude Code 跨平台示例：

```toml
[packages.claude]
name = "claude"
repo = "template:claude"
target = "~/.local/bin"

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

普通归档包示例：

```toml
[packages.example]
name = "example"
repo = "template:example"
target = "~/.local/bin"

latest_url = "https://example.com/tool/latest.json"
latest_format = "json"
latest_json_path = "version"

url_template = "https://example.com/tool/{version}/tool-{os}-{arch}.{ext}"
os_map = { windows = "windows", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64", 386 = "x86" }
ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }
file = "tool"
```

### 为什么使用 `repo = "template:<id>"`

现有 managed package 模型要求每个 package 有一个 `repo` 字符串作为来源身份。`template:<id>` 保持这个不变量，并和已有前缀来源一致：

```text
sourceforge:...
gitlab:...
gitea:...
forgejo:...
template:...
```

规则：

- `repo` 必须以 `template:` 开头。
- 后缀 id 必须非空，用作稳定来源标识。
- `[packages.<name>]` 的 `<name>` 仍是本地 package 名。
- installed store 记录 `repo = "template:<id>"`。
- 渲染后的下载 URL 记录在 installed store 的 `url` 字段。

这样不需要让 `repo` 变成可选字段，也不会破坏 `list/show/uninstall/update` 的目标解析。

## 新增配置字段

在 package section 上新增字段：

```toml
url_template = ""
latest_url = ""
latest_format = "text"
latest_json_path = ""
version_regex = ""

os_map = {}
arch_map = {}
ext_map = {}
libc_map = {}

checksum_url_template = ""
checksum_format = ""
checksum_json_path = ""
checksum_regex = ""

install_action = ""
install_args = []
```

字段语义：

- `url_template`: URL template package 必填。渲染最终下载 URL。
- `latest_url`: 支持安装 latest 和 update 必填。用于获取最新版本。
- `latest_format`: latest 响应格式，支持 `text`、`json`，默认 `text`。
- `latest_json_path`: `latest_format = "json"` 时使用的点路径。
- `version_regex`: 可选。用于从 text 或 JSON 字符串值中提取版本；有命名分组 `version` 时优先使用，否则使用第一个捕获分组。
- `os_map`: Go OS 到上游发布命名的映射。
- `arch_map`: Go arch 到上游发布命名的映射。
- `ext_map`: Go OS 到上游文件扩展名或后缀的映射。URL template package 中该值是字面量；如果模板写 `claude{ext}`，Windows 可配置为 `.exe`，Linux/Darwin 可配置为空字符串。SDK 配置仍可继续用 `.{ext}` 拼接无点扩展名。
- `libc_map`: 可选。Linux libc variant 到模板后缀的映射。支持 `glibc` 和 `musl`；非 Linux 平台 `{libc}` 为空。
- `checksum_url_template`: 可选。checksum 元数据 URL，用同一套变量渲染。
- `checksum_format`: checksum 响应格式，支持 `json`、`text`、`sha256sum`。设置 `checksum_url_template` 时必填。
- `checksum_json_path`: `checksum_format = "json"` 时使用的点路径。
- `checksum_regex`: 可选。用于从 text 或 sha256sum 内容中提取 checksum。
- `install_action`: 可选。为空时走默认下载/提取/放置流程；`run-asset` 表示校验后执行下载到本地的 asset。
- `install_args`: `run-asset` 的参数数组，不经过 shell。支持同一套模板变量。

首版建议完整支持：

- latest: `text`、`json`
- checksum: `json`、`text`、`sha256sum`

原因是 JSON path 解析对 latest 和 checksum 都有用；`sha256sum` 是很多独立下载站常见格式，实现成本低且复用 SHA-256 verifier。

## 模板变量

支持变量：

| 变量 | 含义 |
| --- | --- |
| `{name}` | package 名，例如 `claude` |
| `{version}` | 选中的版本 |
| `{os}` | 应用 `os_map` 后的 OS |
| `{arch}` | 应用 `arch_map` 后的 arch |
| `{ext}` | 应用 `ext_map` 后的扩展名 |
| `{libc}` | Linux libc variant 应用 `libc_map` 后的值；非 Linux 为空 |

映射流程：

1. 从 `--system`、package `system`、global `system` 或当前运行时得到目标平台。
2. 拆分为 Go 风格 `GOOS/GOARCH`。
3. 应用 `os_map[GOOS]`；没有配置时保留 `GOOS`。
4. 应用 `arch_map[GOARCH]`；没有配置时保留 `GOARCH`。
5. 应用 `ext_map[GOOS]`；没有配置时 `{ext}` 为空字符串。
6. 如果目标 OS 是 Linux 且配置了 `libc_map`，检测当前系统 libc。musl 时使用 `libc_map["musl"]`，否则使用 `libc_map["glibc"]`。没有匹配项时 `{libc}` 为空字符串。
7. 用 `{name}`、`{version}`、`{os}`、`{arch}`、`{ext}`、`{libc}` 渲染 `url_template`、checksum template 和 JSON path。

Claude Code Windows amd64 渲染：

```toml
url_template = "https://downloads.claude.ai/claude-code-releases/{version}/{os}-{arch}{libc}/claude{ext}"
os_map = { windows = "win32", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64" }
ext_map = { windows = ".exe", linux = "", darwin = "" }
libc_map = { glibc = "", musl = "-musl" }
```

渲染结果：

```text
{os}       = win32
{arch}     = x64
{libc}     =
{ext}      = .exe
url path   = win32-x64/claude.exe
```

Claude Code Linux musl amd64 渲染：

```text
{os}       = linux
{arch}     = x64
{libc}     = -musl
{ext}      =
url path   = linux-x64-musl/claude
```

上游需要 `win32-x64`、`linux-x64-musl` 这类组合时，直接在模板中写 `{os}-{arch}{libc}`：

```toml
url_template = "https://downloads.claude.ai/claude-code-releases/{version}/{os}-{arch}{libc}/claude{ext}"
checksum_json_path = "platforms.{os}-{arch}{libc}.checksum"
```

这沿用 SDK 的 `os_map` / `arch_map` 风格，不新增 `platform_map` 或 `platform_template`。

### 平台检测补充

默认平台来自现有 `system` 解析。只有用户没有显式配置 `--system` 或 package/global `system` 时，才允许进行 runtime 平台修正。

首版建议补充两类检测：

- Linux libc：检测 musl/glibc，用于渲染 `{libc}`。这能覆盖 Claude Code 的 `linux-x64` 与 `linux-x64-musl` 分发差异。
- macOS Rosetta：如果当前进程是 amd64，但运行在 Apple Silicon 的 Rosetta 2 下，默认应优先使用 native arm64。用户显式设置 `--system darwin/amd64` 时不做该修正。

如果平台检测失败，应回退到 Go runtime 的 `GOOS/GOARCH`，不要阻塞非 Claude 类 package。

## 版本选择

安装行为：

- 如果命令传入 `--tag`，使用 `--tag` 作为 `{version}`，不请求 `latest_url`。
- 如果 package 配置了 `tag`，使用 package `tag` 作为 `{version}`，不请求 `latest_url`。
- 否则请求 `latest_url` 并解析 latest version。

更新行为：

- `update` 总是请求 `latest_url`。
- 将 latest version 和 installed store 中记录的 `tag` 对比。
- installed tag 为空时，返回 check failure，提示需要重新安装一次以写入版本记录。
- latest 等于 installed tag 时跳过。
- latest 不同时，渲染新 URL 并走普通安装流程。

installed store 记录要求：

```text
repo:    template:<id>
target:  template:<id>
tag:     selected version
version: selected version
url:     rendered asset URL
asset:   path base of rendered asset URL
options: 可重复安装所需的 package options
```

## Checksum 校验

Checksum 应复用现有 SHA-256 verifier，不新增独立校验分支。

Claude Code 示例：

```toml
checksum_url_template = "https://downloads.claude.ai/claude-code-releases/{version}/manifest.json"
checksum_format = "json"
checksum_json_path = "platforms.{os}-{arch}{libc}.checksum"
```

流程：

1. 版本和平台确定后渲染 `checksum_url_template`。
2. 用现有 HTTP client 栈请求 checksum 元数据，继承 `proxy_url`、`disable_ssl`、API cache 策略。
3. 按 `checksum_format` 提取 checksum。
4. 将 checksum 传给现有 SHA-256 verifier。
5. checksum 缺失、格式错误或校验失败时，在提取前失败。

如果用户同时配置了 `verify_sha256`，显式 checksum 优先，跳过 `checksum_url_template` 请求。

## Installer Action

为了完整支持 Claude Code 这类“下载 installer binary，再让它自安装”的独立站包，需要新增受控 installer action：

```toml
install_action = "run-asset"
install_args = ["install", "latest"]
```

`run-asset` 的语义：

1. 先完成 latest 解析、URL 渲染、下载和 checksum 校验。
2. 将下载到的 asset 写入临时文件或复用安全缓存文件。
3. 如果目标平台需要可执行权限，在运行前设置 executable bit。
4. 使用 `exec.Command(assetPath, installArgs...)` 运行，不经过 shell。
5. 命令 stdout/stderr 直连当前进程 stdout/stderr，方便用户看到 installer 输出。
6. 命令退出码非 0 时，整个 install 失败。
7. 运行结束后不由 package 配置决定清理策略：如果 asset 来自现有下载 cache，则继续遵循 cache 保留语义；如果实现必须创建临时执行副本，则只清理该临时副本。
8. installed store 记录 source、version、asset URL 和 `install_mode = "run-asset"`。

安全约束：

- 只能执行当前下载并校验通过的 asset 本身。
- `install_args` 必须是数组，不能是 shell 字符串。
- 不支持 `cmd /c`、`sh -c`、管道、重定向、环境变量展开等 shell 行为。
- `run-asset` 默认要求存在 checksum：`verify_sha256`、`checksum_url_template` 或后续显式允许的等价校验来源。没有 checksum 时应报错，而不是执行未校验下载产物。
- `update --all` 可以执行 `run-asset`，但输出应清晰显示正在运行 installer action，避免用户误以为只是解压文件。

Claude Code 的完整配置因此是：

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

这会复刻官方脚本的核心流程：

```text
latest -> manifest checksum -> download binary -> verify -> run `claude install latest` -> keep existing cache semantics
```

它不会由 eget 自己修改 shell profile；这些副作用由 Claude installer binary 自己承担。

`run-asset` 不提供 `install_cleanup` 配置项。eget 已有下载 cache 语义，package 不应额外决定是否删除缓存文件；临时执行副本属于实现细节，执行后应由实现自行清理。

## 安装数据流

现有主链路保持不变：

```text
source target -> candidate asset URLs -> select one asset -> download -> verify -> extract -> record
```

URL template package 的 source 阶段：

```text
template:<id>
  -> 解析版本
  -> 渲染 URL
  -> 返回单个 candidate asset URL
  -> 现有 detector 选择唯一 candidate
  -> 现有 download/cache/proxy/chunk 路径
  -> 可选 checksum manifest 校验
  -> install_action 为空时走现有 extractor/file/rename 路径
  -> install_action = run-asset 时执行校验后的 asset
  -> installed store 记录 selected version
```

新增独立 source 包：

```text
internal/source/urltemplate
```

职责：

- 解析 `template:<id>`。
- 解析 package source 配置。
- 根据 `latest_url` 获取 latest version。
- 渲染变量和 URL template。
- 获取并解析 checksum metadata。
- 为 list/update 返回 latest info。

`internal/install` runner 仍负责下载、校验、提取、GUI installer、输出路径等通用安装行为。

## Runtime Options 传递

现有 `install.Runner.Run(target, opts)` 只接收 target 和 runtime options。URL template 所需字段来自 package config，因此 app 层需要在合并配置时把 source 配置放入 install options。

建议新增一个嵌套 options：

```go
type URLTemplateOptions struct {
    URLTemplate         string
    LatestURL           string
    LatestFormat        string
    LatestJSONPath      string
    VersionRegex        string
    OSMap               map[string]string
    ArchMap             map[string]string
    ExtMap              map[string]string
    LibcMap             map[string]string
    ChecksumURLTemplate string
    ChecksumFormat      string
    ChecksumJSONPath    string
    ChecksumRegex       string
    InstallAction       string
    InstallArgs         []string
}

type Options struct {
    // existing fields...
    URLTemplate URLTemplateOptions
}
```

这样 `install.Service.SelectFinder(target, &opts)` 可以在 `target` 是 `template:<id>` 时使用 `opts.URLTemplate` 创建 finder，不需要让 install 层读取全局配置文件。

## 更新检查数据流

`list --outdated`、`update --check`、`update <name>`、`update --all` 都应把 URL template package 当成一等 managed package。

当前 `LatestInfo(repo, sourcePath)` callback 不够表达 URL template，因为 latest URL 和 template 字段在 package section 里。建议改成显式输入结构：

```go
type LatestCheckTarget struct {
    Name       string
    Repo       string
    SourcePath string
    Package    cfgpkg.Section
}
```

分发逻辑：

```text
sourceforge:*     -> SourceForge latest
gitlab/gitea:*    -> Forge latest
owner/repo        -> GitHub latest
template:*        -> URL template latest using Package fields
```

这样不会把 URL template 配置藏进全局状态，也方便后续继续增加 provider。

为降低改动面，可以先提供 adapter：

```go
type LatestInfoFunc func(target LatestCheckTarget) (LatestInfo, error)
```

然后在 list/update 内部统一构造 `LatestCheckTarget`。测试中已有的简化回调可以用 helper 包装，减少一次性改动范围。

## CLI 行为

首版优先支持配置化使用。

支持：

```bash
eget install claude
eget list --outdated
eget update claude
eget update --all
eget show claude
eget uninstall claude
```

首版不新增：

```bash
eget add --template ...
eget install --template ...
```

原因：`os_map`、`arch_map`、JSON path、checksum template 用 CLI flags 表达会很笨重，也不利于稳定 API。先把配置格式和 update 主链路做稳，再考虑增加 CLI helper。

`eget add` 对 `template:<id>` 不做自动推导。用户应手动写 `[packages.<name>]`，或后续通过专门 helper 生成配置。

对于 `install_action = "run-asset"` 的 package，CLI 输出应明确区分普通提取和 installer action：

```text
Running installer asset: claude install latest
```

## 错误处理

需要给出明确错误：

- `repo = "template:<id>"` 但缺少 `url_template`。
- install latest 或 update 时缺少 `latest_url`。
- 不支持的 `latest_format` 或 `checksum_format`。
- `system` 格式非法。
- Linux libc 检测失败时不报错，除非模板使用了 `{libc}` 且 `libc_map` 没有可用默认值。
- template 引用了未知变量。
- `install_action` 不为空且不是 `run-asset`。
- `run-asset` 缺少 checksum 来源。
- `run-asset` 的 asset 运行失败或退出码非 0。
- latest 响应解析后版本为空。
- `version_regex` 不匹配 latest 响应。
- JSON path 不存在或不是字符串。
- checksum 元数据中找不到 checksum。
- HTTP 失败需要包含请求角色和 URL，例如 `latest_url`、`checksum_url_template`。

固定直接 URL 保持当前行为。直接 URL package 做 update 检查时，仍应报告无法确定 latest info，而不是自动套用 URL template 逻辑。

## 安全边界

URL template package 默认只做：

```text
fetch metadata -> download asset -> verify -> extract/place file
```

如果显式配置：

```toml
install_action = "run-asset"
```

则允许执行“当前下载并校验通过的 asset 本身”。这仍不等价于通用 `post_install`，因为它不允许任意命令字符串、不经过 shell，也不允许执行非当前下载产物。

未来如要支持 `post_install`，必须另起设计，并至少包含：

- 显式 opt-in flag 或全局设置。
- 执行前可见的命令预览。
- `update --all` 默认不隐式执行脚本，除非用户明确允许。
- installed store 记录执行过的 action。

这个边界能让独立站可更新下载源先安全落地。

## 文档更新

实现时需要更新：

- `README.zh-CN.md`
- `README.md`
- `docs/config.zh-CN.md`
- `docs/config.md`
- `docs/example.eget.toml`
- `docs/architecture.md`

文档需要说明：

- 直接 URL 和 URL template package 的区别。
- 为什么 URL template package 可以 update，固定直接 URL 通常不能。
- `os_map` / `arch_map` 与 SDK 配置语义一致。
- Claude Code 示例直接在 `url_template` 和 `checksum_json_path` 中使用 `{os}-{arch}{libc}`。
- Linux musl 使用 `{libc}` 表达，不新增 `platform_template`。
- Claude Code 使用 `install_action = "run-asset"` 完整复刻官方 installer binary 流程。
- 首版不执行通用 post-install 脚本。

## 测试策略

优先测试 `internal/source/urltemplate`：

- 解析合法和非法 `template:<id>`。
- 使用 `os_map`、`arch_map`、`ext_map`、`libc_map` 渲染变量。
- 解析 text latest。
- 解析 JSON latest。
- 使用带 `{os}-{arch}{libc}` 的 JSON path 提取 checksum。
- Linux glibc 渲染为空 `{libc}`，Linux musl 渲染为 `-musl`。
- Windows 渲染 `.exe` 文件后缀，Linux/Darwin 渲染空文件后缀。
- 解析 sha256sum 文本。
- latest version 为空时报错。
- checksum path 缺失时报错。
- `run-asset` 缺少 checksum 来源时报错。
- `run-asset` 使用参数数组运行下载 asset，不经过 shell。
- `run-asset` 命令失败时 install 失败。

App 层测试：

- managed package install 能解析 URL template source。
- installed store 记录 selected version 到 `tag`。
- `list --outdated` 能识别 URL template 新版本。
- `update <name>` 在 latest 不同时重新安装。
- `update --all` 包含 URL template managed packages。
- 直接 URL package 不获得 URL template update 行为。
- `run-asset` 安装成功后记录 `install_mode = "run-asset"` 和 selected version。

Install 层测试：

- 渲染后的 URL 进入现有 download/extract 路径。
- 显式 `verify_sha256` 优先于 checksum manifest。
- checksum manifest 失败时不会执行提取。
- `run-asset` 在 checksum 成功后执行临时 asset。
- `run-asset` 在 checksum 失败时不会执行 asset。

因为该改动触达 MVP install/update 主链路，完成实现后必须运行：

```bash
go test ./...
```

## 兼容性

现有配置不受影响：

- GitHub `owner/repo` 保持现有行为。
- GitHub URL 保持现有行为。
- SourceForge 和 forge 前缀保持现有行为。
- 直接 URL install/download 保持现有行为。
- SDK 配置和 `eget sdk` 行为保持不变。

新行为只在以下配置中启用：

```toml
repo = "template:<id>"
```

不会把任何已有配置自动解释成 URL template package。

## 默认决策

如果评审没有调整，实现按以下默认值执行：

- `latest_format = "text"`。
- 缺失 `os_map` / `arch_map` 条目时回退到 Go 原始名称。
- 缺失 `ext_map` 条目时 `{ext}` 渲染为空字符串。
- 缺失 `libc_map` 时 `{libc}` 渲染为空字符串。
- `--tag` 在 install 时覆盖 `latest_url`。
- package `tag` 在 install 时覆盖 `latest_url`。
- `update` 总是使用 `latest_url` 对比 latest，即使 package 配置了 `tag`。
