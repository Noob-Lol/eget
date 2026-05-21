# show 命令与 package desc 实施计划

## 目标

- `packages.<name>.desc` 支持手动配置 package 描述。
- `installed` 记录保存描述信息，未加入 config 的已安装 package 也能查看描述。
- 新增 `eget show <package|repo>` 显示配置与安装详情。

## 设计

- 写入时补全元数据，`show` 默认只读本地数据，不联网。
- `config desc` 优先级高于 repository 获取到的描述。
- installed 记录新增 `desc`、`homepage`、`repo_url` 字段。
- 未记录 `homepage` / `repo_url` 时，`show` 根据 repo 类型推断：
  - GitHub: `https://github.com/owner/repo`
  - SourceForge: `https://sourceforge.net/projects/<project>/`
  - GitLab/Gitea/Forgejo: 由 forge target 的 host/namespace/project 推断。

## 阶段

- [ ] config / installed model 增加 desc 元数据字段。
- [ ] install/add 写入时补全 repository metadata。
- [ ] app 层新增 ShowService，合并 config 和 installed 信息。
- [ ] CLI 新增 `eget show <target>` 命令并渲染详情。
- [ ] 更新 README / README.zh-CN / TODO，并运行 `go test ./...`。
