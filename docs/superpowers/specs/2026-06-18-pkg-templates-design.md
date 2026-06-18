# pkg_templates 设计

## 背景

`eget` 已经支持 `template:<id>` package source，用于描述不在 GitHub、GitLab/Gitea、SourceForge 等内置 provider 上发布的独立下载站。它通过 package 配置里的 `latest_url`、`url_template`、平台映射和 checksum 字段解析最新版本、渲染下载 URL，并复用现有 `install/update/list/show/uninstall` 主链路。

当前痛点是内部工具数量变多后，很多工具发布规则完全一致，只是工具名不同。例如：

```toml
[packages.markview]
repo = "template:markview"
latest_url = "http://mydev.lan/tools/markview/latest.yaml"
url_template = "http://mydev.lan/tools/markview/markview-{os}-{arch}{ext}"

[packages.foo]
repo = "template:foo"
latest_url = "http://mydev.lan/tools/foo/latest.yaml"
url_template = "http://mydev.lan/tools/foo/foo-{os}-{arch}{ext}"
```

这些配置本质上是一套可参数化的 package 模板。用户希望只配置一次模板源，然后通过短 target 安装或添加内部工具。

## 命名决策

本功能使用配置块：

```toml
[pkg_templates.<name>]
```

持久化 package 引用使用：

```toml
repo = "pkg-template:<template>:<package>"
```

CLI 输入支持短别名：

```bash
eget add mydev:markview
eget install mydev:markview
eget install --add mydev:markview
```

其中 `mydev:markview` 在存在 `[pkg_templates.mydev]` 时，等价解析为 `pkg-template:mydev:markview`。

不使用 `registry` / `registries`。原因是 registry 更适合留给后续真正的索引/仓库服务：可搜索、可解析版本、可返回 source metadata/checksum，甚至可在内网离线替代第三方 provider。当前功能只是一套 package URL template 的命名空间复用，不提供 registry 级能力。

不使用 `source`。原因是 source 过宽，GitHub、SourceForge、直接 URL、template、SDK index 都可以叫 source，无法说明这里是“package 模板”。

## 目标

- 新增 `[pkg_templates.<name>]` 配置块，复用现有 template package source 字段。
- 支持 `{name}` 出现在 `latest_url`、`url_template`、checksum URL/path 等模板值中。
- 支持 `pkg-template:<template>:<package>` 作为规范 repo target。
- 支持 `mydev:markview` 这种 CLI 短别名；短别名只在 `mydev` 匹配已配置 `pkg_templates` 时生效。
- `eget add mydev:markview` 写入轻量 package 引用，而不是展开成完整 URL 配置。
- `eget install mydev:markview` 可以不落盘直接安装。
- `eget install --add mydev:markview` 安装成功后写入轻量 package 引用。
- 后续 `install/update/list --outdated/show/uninstall` 继续复用现有 package 主链路。

## 非目标

首版不做真正 registry。

不新增：

- 远端 package index schema。
- `eget registry search/list` 或 `eget pkg-template search/list`。
- 上传、发布、账号、权限或 server 端接口。
- 不依赖第三方 provider 的 cache registry 解析能力。
- 独立下载引擎。
- 对任意网页的自动抓取解析。

本功能也不替代 `template:<id>`。`template:<id>` 仍用于单个独立站点工具；`pkg-template:<template>:<package>` 用于同一套模板规则下的多个工具。

## 配置格式

推荐配置：

```toml
[pkg_templates.mydev]
latest_url = "http://mydev.lan/tools/{name}/latest.yaml"
url_template = "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"
latest_format = "yaml"
os_map = { windows = "win", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64" }
ext_map = { windows = ".exe", linux = "", darwin = "" }
```

字段直接沿用现有 package template 字段：

| 字段 | 说明 |
| --- | --- |
| `latest_url` | 最新版本 metadata URL。这里允许包含 `{name}`。 |
| `latest_format` | `text`、`json` 或 `yaml`。 |
| `latest_json_path` | `latest_format = "json"` 时提取版本的路径。允许模板变量。 |
| `version_regex` | 从 latest metadata 中提取版本的正则。 |
| `url_template` | 下载 URL 模板。这里允许包含 `{name}`。 |
| `os_map` | Go OS 到发布站 OS 名称的映射。 |
| `arch_map` | Go arch 到发布站 arch 名称的映射。 |
| `ext_map` | Go OS 到文件扩展名的映射。 |
| `libc_map` | Linux libc 名称映射。 |
| `checksum_url_template` | checksum metadata URL 模板。允许 `{name}`。 |
| `checksum_format` | `text` 或 `json`。 |
| `checksum_json_path` | JSON checksum 路径。允许模板变量。 |
| `checksum_regex` | checksum 提取正则。 |
| `install_action` | 复用现有 `run-asset` 语义。 |
| `install_args` | `run-asset` 参数数组。 |

首版不新增 `latest_url_template` 或 `pkg_template` 字段。`latest_url` 和 `url_template` 本身就是模板字符串，只是现有单 package 场景通常不用 `{name}`。

## Package 引用

通过 `add` 写入配置时，默认生成轻量引用：

```toml
[packages.markview]
repo = "pkg-template:mydev:markview"
name = "markview"
```

不把 `latest_url` 和 `url_template` 展开写入 package。这样后续统一调整 `[pkg_templates.mydev]` 后，所有引用该模板的 package 都会跟随生效。

如果某个 package 需要覆盖个别字段，可以在 package section 中配置：

```toml
[packages.markview]
repo = "pkg-template:mydev:markview"
desc = "Markdown preview tool"
asset_filters = ["windows:amd64"]
ext_map = { windows = ".zip" }
```

覆盖规则沿用现有配置合并思路：

```text
CLI 参数 > packages.<name> > pkg_templates.<template> > global
```

其中 `packages.<name>` 的字段只覆盖当前 package，不修改模板定义。

## Target 解析

新增规范 target：

```text
pkg-template:<template>:<package>
```

解析规则：

- `<template>` 是 `pkg_templates` 下的模板名。
- `<package>` 是模板内的工具名，会作为 `{name}` 变量值。
- 两段都不能为空。
- 规范 target 不允许多余段，例如 `pkg-template:a:b:c` 应报错。
- 模板不存在时报错：`pkg template "mydev" is not configured`。

CLI 短别名：

```text
<template>:<package>
```

只有在 `<template>` 匹配已配置 `pkg_templates.<template>` 时，才解析为 `pkg-template:<template>:<package>`。

这避免影响现有 target：

- `sourceforge:project/path`
- `gitea:host/owner/repo`
- `gitlab:host/owner/repo`
- `owner/repo`
- `template:<id>`
- URL target

如果短别名和现有已支持 provider 前缀冲突，内置 provider 优先，或者显式要求用户使用规范写法 `pkg-template:<template>:<package>`。首版建议保守处理：已知 provider 前缀不参与短别名解析。

## 安装解析流程

以 `eget install mydev:markview` 为例：

```text
CLI target: mydev:markview
-> 发现 mydev 匹配 pkg_templates.mydev
-> 规范化 target 为 pkg-template:mydev:markview
-> 构造临时 package section:
   repo = "pkg-template:mydev:markview"
   name = "markview"
-> 合并 global + pkg_templates.mydev + package override + CLI
-> 将 pkg-template target 转换给现有 urltemplate Finder:
   template id/name = "markview"
   latest_url = render package template latest_url with {name}
   url_template 仍由 Finder 在已解析 version/os/arch/ext 后渲染
-> 继续现有 install/download/checksum/cache mirror/installed store 流程
```

这里不新增新的下载执行器。`pkg-template` 只负责把模板配置和 package name 解析成现有 URL template package source 可以消费的 options。

## add / install --add 行为

`eget add mydev:markview`：

```text
1. 解析短别名。
2. 校验 pkg_templates.mydev 存在。
3. 推导 package name = markview，除非用户显式传 --name。
4. 写入 packages.<name>.repo = "pkg-template:mydev:markview"。
5. 保存用户通过 add flags 指定的 package 覆盖字段。
```

`eget install --add mydev:markview`：

```text
1. 按临时 pkg-template 安装。
2. 安装成功后写入轻量 package 引用。
3. 如果安装失败，不写入配置。
```

如果 `packages.<name>` 已存在，沿用当前 add 行为：覆盖写入或按现有错误策略处理，不为 pkg-template 单独创造交互逻辑。

## update / list --outdated

已配置 package：

```toml
[packages.markview]
repo = "pkg-template:mydev:markview"
```

执行：

```bash
eget list --outdated
eget update markview
eget update --all
```

应按 package name 找到 `repo`，再通过 `pkg_templates.mydev` 解析 latest metadata。版本比较、已安装记录、cache mirror 和 checksum 校验继续复用现有主链路。

如果模板配置被删除，已配置 package 在 update/list outdated 时应返回清晰错误，而不是退化为 GitHub repo target。

## installed 记录

安装记录中的 `Repo` 和 `Target` 应保留规范 repo：

```text
pkg-template:mydev:markview
```

这样 `show`、`list`、`update` 能看出 package 来源，也避免多个模板里同名工具互相混淆。

记录 options 时，可以保留实际解析后的 URL template 字段，沿用现有 template package installed metadata 记录策略。显示层优先展示 package repo 和 resolved version，详细模式可以展示 template source。

## 与 cache registry 化的关系

`pkg_templates` 不是 cache registry。

`pkg_templates`：

- 客户端本地配置。
- 用 name 渲染 latest URL 和下载 URL。
- 仍依赖远端 latest metadata 或下载站。
- 不提供搜索/index/server。

后续真正 registry：

- 可能由 `cache serve` 或独立服务提供。
- 能搜索 package/sdk。
- 能解析 name/version/os/arch。
- 返回 checksum/source metadata/cache metadata。
- 可以在内网离线替代第三方 provider。

两者可以长期共存。未来如果 registry 返回的 metadata 中也包含 URL template，不应复用 `pkg_templates` 的配置块命名。

## 错误处理

首版应覆盖以下错误：

- `pkg-template` target 格式错误。
- 短别名 target 的 template 不存在时，不应误判为其它 target。
- `pkg_templates.<name>` 存在但缺少 `latest_url` 或 `url_template`。
- 模板字段渲染时出现未知变量。
- latest metadata 获取失败。
- latest metadata 解析失败。
- checksum metadata 获取或解析失败。

错误信息应包含 template 名和 package 名，例如：

```text
pkg template "mydev" for package "markview" has no url_template
```

## 测试策略

单元测试重点：

- target parser：
  - `pkg-template:mydev:markview`
  - 空 template/package
  - 多余段
  - 短别名匹配已配置 `pkg_templates`
  - 已知 provider 前缀不被短别名劫持
- config load/dump：
  - `[pkg_templates.mydev]` 字段 round-trip
  - `packages.markview.repo = "pkg-template:mydev:markview"` round-trip
- install option merge：
  - `pkg_templates` 字段被合并到 URL template options
  - package section 覆盖模板字段
  - CLI 覆盖 package 字段
- add / install --add：
  - 写入轻量 repo 引用
  - 不展开 URL 到 package section
  - 安装失败不写配置
- update/list outdated：
  - 能通过 installed/config 中的 `pkg-template` 找到 latest
  - 模板缺失时报错清晰

集成测试建议用本地 HTTP server：

- 提供 `/tools/markview/latest.yaml`。
- 提供 `/tools/markview/markview-windows-amd64.exe` 或测试归档。
- 执行 install/download 路径，断言最终下载 URL、installed 记录和配置写入。

完成 MVP 主链路改动后需要运行：

```bash
go test ./...
```

## 实施范围建议

本功能会涉及配置模型、配置读写、target 解析、install resolve、add/install --add、update/list outdated 和文档。实现时预计会修改超过 3 个逻辑文件，因此按项目规范，实施前需要再次确认范围。

建议分期：

1. 配置模型和 target parser。
2. install/download 临时解析支持。
3. add / install --add 轻量引用写入。
4. update/list outdated/show 展示链路补齐。
5. README、config docs、TODO 示例更新。

## 自查

- 命名使用 `pkg_templates` 和 `pkg-template`，没有占用 registry。
- 配置字段沿用现有 template package source，没有新增重复的 `latest_url_template` 或 `pkg_template`。
- `{name}` 只是扩展现有模板变量使用场景，不新增下载引擎。
- CLI 短别名只在匹配已配置模板时生效，降低和现有 provider target 冲突的风险。
- package 配置保留轻量引用，便于统一调整内部工具发布规则。

## 实施确认

实施计划见 [2026-06-18-pkg-templates.md](../plans/2026-06-18-pkg-templates.md)。实现保持 `pkg_templates` 只做本地模板复用，不扩展为 registry/index/search。
