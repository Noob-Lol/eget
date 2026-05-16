# TODO

<!--
简单的直接使用一行 checklist 说明即可。
需要附带较长说明的，使用标题+说明方式新建。使用emoji 表情状态图标(wait: ⏳|ing: 🔄|done: ✅)
-->

- [x] 增强 list --outdated 用于显示有更新的工具
- [ ] 新增 eget 自身管理命令，eg: clean 用于清理缓存
- [x] 增强功能：参考自 https://github.com/marwanhawari/stew
  - [x] 新增命令 query 用于浏览 GitHub repository 的 releases
  - [x] 新增命令 search 用于搜索 GitHub 上的 repository
- [x] 新增配置 global.gui_target 用于指定 GUI 应用的安装目录
  - 同时 package 新增 isgui 字段用于指定是否为 GUI 应用, 如果是 msi, setup exe, 如何启动应用安装？
  - list 支持 --gui 选项用于显示 GUI 应用
- [ ] 新增命令 run 用于运行已安装的工具，即使它没有在 PATH 中
  - 如果是 GUI 应用，需要启动应用安装目录下的可执行文件
  - 如果是命令行工具，直接运行可执行文件
- [x] 增强 install/download/update 支持并发下载
  - `--chunk N` / `global.chunk_concurrency` 控制单文件 HTTP Range 分片并发
  - `--batch N` / `global.batch_concurrency` 控制 `install --all` / `update --all` 批处理并发
- [x] 优化 `list --outdated / update --check` 查询处理。
  - [x] 支持并发查询多个包信息 API
  - [x] 复用 `api_cache.cache_time`，`update --check` 后在缓存时间内执行 `update` 不会重复检查 GitHub API
- [x] 优化 新增 global.sys7z_path 用于指定 7z 可执行文件路径
  - 解压文件处理时优先使用系统环境中的 7z 可执行文件进行处理(优先从sys7z_path获取, 再从环境变量PATH中获取)
  - 如果系统环境中没有, 则使用 go mod 包进行解压处理
  - `--extract-all` 使用系统 7z 时直接一次性解压，不再先 list 文件列表
  - `--extract-all` 使用 Go 内置解压时单次遍历并流式写出，避免先缓存所有文件内容
- [ ] 新增 global.group_packages 用于配置 package 分组（详情见下面）
- [x] 全局配置 新增 global.ignore_update_packages 用于配置忽略检查/更新的 packages
- [ ] 新增支持 sdk 下载安装，需要支持多版本。例如 go, node, python 等 sdk（详情见下面）
- [x] 增强 install/update 的 target 参数支持多个目标。eg: `install name1 name2 ...`
  - 只输入一个参数时，也支持使用逗号分隔，例如: `install name1,name2,name3`
- [ ] package config 新增 desc 字段用于指定 package 的描述，可以手动设置，为空时默认从 repository 中获取
  - 没有在config, 但是 installed 里的 package 也会记录描述信息，方便查看

## search 结果展示 ✅

```txt
<info>owner/repo</> ⭐{stars} language: {language} update: {update_time}
{description}
---
```

## 新增 global.group_packages 用于配置 package 分组 ⏳

配置新增 `global.group_packages` 用于配置 package 分组, 可以配置多个分组,在需要恢复时指定分组快速安装。
例如:

- `required` 分组用于指定必须安装的 package names
- `optional` 分组用于指定可选安装的 package names
- `dev` 分组用于指定开发环境的 package names

通过 `eget install --group <group-names>` 选项可以安装指定分组的 packages. 可以多个分组名称, 用逗号分隔.
例如：`eget install --group required,dev` 需要安装 `required` 和 `dev` 分组的 packages.

config example:

```toml
[group_packages]
required = ["required1", "required2"]
optional = ["optional1", "optional2"]
dev = ["dev1", "dev2"]
```

## 新增支持 sdk 下载安装，需要支持多版本 ⏳

新增支持 sdk 下载安装，需要支持多版本。例如 go, node, python 等 sdk（详情见下面）
- 支持指定版本安装, 安装到每个sdk的目录下
- 支持自动检测并安装最新版本

设想是通过配置 url template 来指定 sdk 下载地址，例如：`https://go.dev/dl/go1.21.1.windows-amd64.zip` 来指定 go 下载地址

```toml
[global]
sdk_target = "path/to/sdk"
# 从远程下载的 sdk 工具包默认格式
sdk_download_ext = {
  windows = "zip",
  linux = "tar.gz",
  darwin = "tar.gz"
}

[sdk]
[sdk.go]
aliases = ["golang"]
# 如果是相对路径，则是基于 global.sdk_target 目录
target = "gosdk/go{version}"
# mirror https://mirrors.aliyun.com/golang/
# eg: https://mirrors.aliyun.com/golang/go1.21.1.windows-amd64.zip
url_template = "https://mirrors.aliyun.com/golang/go{version}.{os}-{arch}.{download_ext}"
# eg: https://golang.org/dl/go1.21.1.windows-amd64.zip
# url_template = "https://golang.org/dl/go{version}.{os}-{arch}.{download_ext}"

[sdk.node]
# index_url 用于指定 nodejs 版本/下载地址的索引页面（html,json 格式都支持）
# 配置了 index_url 时
#  1. 支持版本搜索
#  2. 会从 index_url 中获取 nodejs 下载地址, 而不是从 url_template 构建下载地址
# 无 index_url 时, 必须指定完整的 {version} 版本号才能下载
index_url = "https://registry.npmmirror.com/binary.html"
aliases = ["nodejs"]
# 如果是相对路径，则是基于 global.sdk_target 目录
target = "nodejs/node{version}"
download_ext = {windows = "zip", linux = "tar.gz"}

# mirror, see https://registry.npmmirror.com/binary.html 查看二进制文件列表 node,python,bun,deno
url_template = "https://cdn.npmmirror.com/binaries/node/v{version}/node-v{version}-{os}-{arch}.{download_ext}"
# eg: https://nodejs.org/dist/v18.16.0/node-v18.16.0-x64.msi
# url_template = "https://nodejs.org/dist/v{version}/node-v{version}-{os}-{arch}.{download_ext}"
```

url template 中的占位符有
- `{version}`
- `{os}`
- `{arch}`
- `{download_ext}`

target 目标目录结构示例：

```txt
{sdk_target}/
|- gosdk/go{version}
|- nodejs/node{version}
```
