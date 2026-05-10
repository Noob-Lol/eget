# TODO

<!--
简单的直接使用一行 checklist 说明即可。
需要附带较长说明的，使用标题+说明方式新建。使用emoji 表情状态图标(wait: ⏳|ing: 🔄|done: ✅)
-->

- [x] 增强 list --outdated 用于显示有更新的工具
- [ ] 新增命令 clean 用于清理缓存
- [x] 增强功能：参考自 https://github.com/marwanhawari/stew
  - [x] 新增命令 query 用于浏览 GitHub repository 的 releases
  - [x] 新增命令 search 用于搜索 GitHub 上的 repository
- [ ] 配置新增 global.restore_packages 用于指定 `eget install` 需要恢复的 package names
- [x] 新增配置 global.gui_target 用于指定 GUI 应用的安装目录
  - 同时 package 新增 isgui 字段用于指定是否为 GUI 应用, 如果是 msi, setup exe, 如何启动应用安装？
  - list 支持 --gui 选项用于显示 GUI 应用
- [ ] 新增命令 run 用于运行已安装的工具，即使它没有在 PATH 中
  - 如果是 GUI 应用，需要启动应用安装目录下的可执行文件
  - 如果是命令行工具，直接运行可执行文件
- [x] 增强 install/download/update 支持并发下载
  - `--chunk N` / `global.chunk_concurrency` 控制单文件 HTTP Range 分片并发
  - `--batch N` / `global.batch_concurrency` 控制 `install --all` / `update --all` 批处理并发
- [ ] 优化 `list --outdated / update --check` 缓存查询结果，2分钟内执行 `update` 不会重新检查更新

## search 结果展示 ✅

```txt
<info>owner/repo</> ⭐{stars} language: {language} update: {update_time}
{description}
---
```
