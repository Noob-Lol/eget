# Eget

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/inherelab/eget?style=flat-square)
[![GitHub tag (latest SemVer)](https://img.shields.io/github/tag/inherelab/eget)](https://github.com/inherelab/eget)
[![Unit-Tests](https://github.com/inherelab/eget/actions/workflows/go.yml/badge.svg)](https://github.com/inherelab/eget)

---

[English](./README.md) | [简体中文](./README.zh-CN.md)

`eget` helps locate, download, and extract prebuilt binaries from GitHub, GitLab, Gitea/Forgejo, and SourceForge.

> Forked from https://github.com/zyedidia/eget Refactored and enhanced the tool's functionality.

## Features

- Multi-source installs: install or download binaries from GitHub, GitLab, Gitea/Forgejo, SourceForge, direct download URLs, and local files.
- Automatic selection and extraction: filter release assets by OS/arch, keyword, or regex, with SHA-256 verification and common archive extraction.
- Managed package workflow: use `add`, `list`, `update`, and `uninstall` to manage frequently used tools, record install state, and check batch updates.
- SDK downloads: install versioned SDK archives such as Go and Node with index cache, resumable downloads, and a separate SDK installed store.
- Concurrent downloads: automatically split large files into HTTP Range chunks for parallel download, and run package downloads concurrently when installing or updating all packages.
- Query and search: query GitHub release info, query SourceForge latest/assets, and search repositories with native GitHub search qualifiers.
- Cache and proxy support: use download cache, API response cache, `proxy_url`, and `ghproxy` for restricted networks or repeated installs.
- Config-driven usage: configure global defaults, repo-level options, and `packages.<name>` managed packages; config and installed store default to `~/.config/eget/`.

## Install

- Download from Releases [https://github.com/inherelab/eget/releases](https://github.com/inherelab/eget/releases)
- Install by `go install` command (requires a local Go SDK)

```bash
go install github.com/inherelab/eget/cmd/eget@latest
```

## Command Style

```bash
eget <command> --options... arguments...
```

## Usage Examples

### Install Examples

**Install from GitHub**:

```bash
# quickly
eget i ORG/REPO
# install with tag
eget install --tag nightly inhere/markview
# Install and override the executable name
eget install --name chlog gookit/gitw
# Install and override the asset name
eget install --asset zip windirstat/windirstat
# Filter assets with regex
eget install --asset "REG:\\.deb$" owner/repo
# Install to a custom directory
eget install --to ~/.local/bin/fzf junegunn/fzf
```

**Install a SourceForge project**:

```bash
# Install a SourceForge project directly
eget install --asset x64,PerUser,setup sourceforge:winmerge
```

**Install a GitLab/Gitea/Forgejo project**:

```bash
# Install from GitLab releases
eget install gitlab:fdroid/fdroidserver
eget install gitlab:gitlab.gnome.org/GNOME/gtk
# Install from Gitea/Forgejo-compatible releases
eget install --asset linux,amd64 gitea:codeberg.org/forgejo/forgejo
```

**Install and record**:

```bash
# Install and record the package definition
eget install --add junegunn/fzf
eget install --add --name rg BurntSushi/ripgrep
# Add a SourceForge project as a managed package
eget add --name winmerge --system windows/amd64 --asset x64,PerUser,setup sourceforge:winmerge
# Install every package configured under [packages]
eget install --all
```

**Install GUI apps**:

```bash
# Install a GUI app; portable GUI apps use global.gui_target by default
eget install --gui sipeed/picoclaw
eget add --gui --name picoclaw sipeed/picoclaw
```

### Download Examples

```bash
# download
eget download ip7z/7zip
eget download --file go --to ~/go1.17.5 https://go.dev/dl/go1.17.5.linux-amd64.tar.gz
eget download --file README.md,LICENSE --to ./dist owner/repo
eget download --file "*.txt" owner/repo
eget download --file "bin/*" owner/repo
eget download --extract-all --to ./dist windirstat/windirstat
```

### SDK Examples

```bash
eget sdk install go@1.22.0
eget sdk install go:1.22 node:20.11.1
eget sdk install --force go@1.22.0
eget sdk list
eget sdk list --json
eget sdk remove go@1.22.0
eget sdk index refresh go
eget sdk index show go
```

`eget sdk` only downloads and extracts SDK archives. It does not modify `PATH`, write shell hooks, or manage active SDK versions. For environment switching, use a dedicated tool such as `kite xenv` after installation.

### Query Examples

**Query repository info**:

```bash
# query repo info
eget query owner/repo
eget query --action releases --limit 5 owner/repo
eget query --action assets --tag v1.2.3 owner/repo

# query SourceForge latest version or assets
eget query sourceforge:winmerge
eget query --action assets sourceforge:winmerge/stable
eget query --action assets --tag 2.16.44 sourceforge:winmerge/stable
```

**Search GitHub repositories**:

```bash
eget search ripgrep
eget search skillc language:go user:inhere
eget search --limit 5 --sort stars --order desc terminal ui
eget search --json picoclaw user:sipeed
```

### Other Examples

```bash
# uninstall
eget uninstall fzf
# list installed packages
eget list|ls
# list all managed and installed packages
eget list --all
# list GUI packages
eget list --gui
# update fzf
eget update fzf
eget update --all
```

### Config Examples

```bash
# config
eget add --name fzf --to ~/.local/bin junegunn/fzf
eget config init
eget config list|ls
eget config get global.target
eget config set global.target ~/.local/bin
```

### Supported Targets

The target argument accepted by `install` and `download` can be:

- `name` configured in packages
- GitHub repository, for example `owner/repo`
- GitHub repository URL, for example `https://github.com/owner/repo`
- GitLab target, for example `gitlab:fdroid/fdroidserver` or `gitlab:gitlab.gnome.org/GNOME/gtk`
- Gitea/Forgejo target, for example `gitea:codeberg.org/forgejo/forgejo`
- SourceForge target, for example `sourceforge:winmerge` or `sourceforge:winmerge/stable`
- Direct download URL, for example `https://example.com/file.tar.gz`
- Local file path, for example `file:///path/to/file`

> Note: GitLab and Gitea/Forgejo support currently covers `install`, `download`, and `update` for release assets. SourceForge also supports `query latest` and `query assets`. Search parity, private repository authentication, and automatic provider detection from arbitrary web URLs are not provided.

## Available Commands

`install` (aliases: `i`, `ins`)

- Resolve, download, verify, and extract a target, then record installation state.
- `--name` can be used to override the installed executable name; without `--to`, it also acts as the rename hint for single-file assets.
- `--gui` marks the target as a GUI application. Portable GUI apps use `global.gui_target` by default, while GUI installers such as `.msi` or `setup.exe` are launched and do not record a final install directory. Without `--gui`, installer-like assets prompt before launch; with `--add`, a confirmed installer is persisted with `is_gui = true`.
- With `--add`, a successful install also writes the repo target to `[packages.<name>]`; use `--name` to override the package name.
- Use `--all` without a target to install every package configured under `[packages]`. Package-level options are merged the same way as single package installs.

`download` (alias: `dl`)

- Reuses the install pipeline without recording installed state.
- Downloads the raw asset by default; archive extraction only happens when `--file` or `--extract-all` is set.

`add`

- Writes a managed package definition to `[packages.<name>]` in the config file.

`uninstall` (aliases: `uni`, `rm`)

- Removes installed files and clears the installed store entry without deleting `[packages.<name>]`.

`list` (alias: `ls`)

- Lists installed packages by default.
- Use `--all` / `-a` to list the union of local managed packages and installed-store entries.
- Use `--gui` to filter the current list view to GUI applications.

`query` (alias: `q`)

- Queries GitHub repository release metadata and SourceForge latest/assets without installing anything or touching local state.
- Defaults to the `latest` action, and can switch to `info`, `releases`, or `assets` with `--action`.

`search`

- Searches GitHub repositories without installing anything or touching local state.
- Uses the first argument as the keyword and passes remaining arguments through as GitHub search qualifiers, for example `language:go`, `user:inhere`, or `topic:cli`.

`update` (alias: `up`)

- Updates a configured or installed target after checking that a newer version exists, or all managed packages with `--all`.

`sdk`

- Downloads and installs versioned SDK archives into `global.sdk_target`, with resumable `.part` downloads and independent records in `sdk.installed.json`.
- Supported install target formats are `name`, `name@latest`, `name:latest`, `name@1.22`, `name:1.22`, `name@1.22.0`, and `name:1.22.0`. Space-separated version forms such as `go 1.22.0` are intentionally not supported.
- `sdk install` accepts one or more SDK targets and installs them serially.
- `sdk list` reads installed SDK records. Use `--json` for machine-readable output.
- `sdk remove <name@version>` removes only paths recorded in the SDK installed store and verified under the configured SDK root.
- `sdk index list/show/refresh/clear` manages normalized SDK index cache files.
- The first built-in examples target Go and Node. Other SDKs can also work when their archive names can be described by `url_template`, `filename_pattern`, `os_map`, `arch_map`, `ext_map`, and optional HTML/JSON index settings.

`config` (alias: `cfg`)

- Supports `init`, `list` / `ls`, `get KEY`, and `set KEY VALUE`.

## Main Options

`install`, `download`, and `add` share these installation-related options:

- `--tag`: Select a release tag; defaults to `latest` when omitted.
- `--system`: Override the target OS/arch, for example `windows/amd64` or `linux/arm64`.
- `--to`: Set the install or download output path; accepts either a directory or a full file path.
- `--file`: Select file(s) to extract from an archive; supports comma-separated file names or glob patterns such as `README.md,LICENSE`. For 7z-readable `.exe` installers, system 7z is required.
- `--asset`: Filter release assets by keyword; multiple filters can be separated by commas. Regex is also supported with the `REG:` prefix, for example `REG:\\.deb$`, and exclusions can use `^REG:...`.
- `--source`: Download the source archive instead of a prebuilt binary release.
- `--extract-all`, `--ea`: Extract all files from the archive instead of selecting a single target file.
- `--chunk N`: Control HTTP Range chunk concurrency for one downloaded file. `0` means auto, `1` means single-connection download, and values greater than `1` request up to that many chunks.
- `--quiet`: Reduce normal command output for scripting or batch use.

`install` and `download` also support `--fallback-versions N` for SourceForge targets. When the latest version folder does not contain a matching asset, eget scans up to `N` older version folders and uses the first folder where the current `--asset` / `--system` filters produce a single match.

> Cache behavior is configured via `config set global.cache_dir ...` or the `cache_dir` field in the config file.

`install` additionally supports:

- `--add`: After a successful install, append the repo target to `[packages.<name>]` managed config.
- `--all`: Install every package configured under `[packages]`; cannot be combined with a target or `--add`.
- `--batch N`: Control package task concurrency for `install --all`. `0` means auto, `1` means serial, and values greater than `1` process up to that many packages at once.
- `--gui`: Install as a GUI application; with `--add`, persist `is_gui = true`. Installer-like assets selected without `--gui` prompt before launch and also persist `is_gui = true` when confirmed with `--add`.
- `--name`: Override the managed package name; for single executable assets, it also acts as the default output-name hint.

`update` options:

- `--all`: Check managed packages and update only outdated installed packages.
- Single-target `update <target>` requires the target to already exist in config or the installed store. Use `install` for new targets.
- `--batch N`: Control package task concurrency for `update --all`. `0` means auto, `1` means serial, and values greater than `1` process up to that many packages at once.
- `--chunk N`: Control HTTP Range chunk concurrency for downloads triggered by update.
- `--check`: Check and list outdated installed packages, same as `list --outdated`.
- `--dry-run`: Preview the update plan without performing installation changes.
- `--interactive`: Interactively select which managed packages to update.

`query` options:

- `--action`, `-a`: Query action. Supported values: `latest`, `releases`, `assets`, `info`.
- `--tag`, `-t`: Select the release tag for the `assets` action; defaults to latest when omitted.
- `--limit`, `-l`: Limit the number of rows returned by the `releases` action. Default: `10`.
- `--json`, `-j`: Output JSON for scripting or automation.
- `--prerelease`, `-p`: Include prerelease entries for `latest` and `releases`.

SourceForge query targets use `sourceforge:<project>` or `sourceforge:<project>/<path>` and currently support only `latest` and `assets`. `info`, `releases`, `--limit`, and `--prerelease` remain GitHub-only.

`search` options:

- `--limit`, `-l`: Limit the number of repositories returned. Default: `10`.
- `--sort`: Sort search results. Supported values: `stars`, `updated`.
- `--order`: Sort order. Supported values: `desc`, `asc`.
- `--json`, `-j`: Output JSON for scripting or automation.

`sdk` options:

- `sdk install --force`: Remove an existing SDK target directory after safety checks, then reinstall.
- `sdk list --json`: Output installed SDK records as JSON.
- `sdk index list --json`: Output cached SDK index summaries as JSON.
- `sdk index refresh --all`: Refresh all configured SDK indexes with `index_url`.
- `sdk index clear --all`: Delete all cached SDK index JSON files.

Global options:

- `-v`, `--verbose`: Show more execution details such as API requests, response summaries, asset selection, cache hits, and key workflow steps.

Notes:

- `install --name` can rename a single executable asset, for example installing `chlog-windows-amd64.exe` as `chlog.exe`.
- `install --add` only applies to repo targets and appends the managed package definition after a successful install.
- `global.gui_target` is used only for portable GUI applications. GUI installers such as `.msi` or `setup.exe` are launched and do not record a final install directory.
- `download` stores the raw downloaded asset by default; extraction only happens when `--file` or `--extract-all` is provided.
- `sdk` uses `global.sdk_target` for install directories and `{cache_dir}/sdk-downloads` for archive downloads. Resumable state is stored as `.part` plus `.meta.json`.
- Archive extraction currently supports `zip`, `tar.*`, and `7z`. System 7z is preferred for `.7z`, `.rar`, `.msi`, `.cab`, `.iso`, and `--extract-all` `.exe` archives when `global.sys7z_path` or `PATH` provides `7z`, `7zz`, or `7za`; `tar.*` archives continue to use the built-in Go extractor.
- Argument order follows the CLI parser constraint and must be `CMD --OPTIONS... ARGUMENTS...`.

## Configuration

The config file is resolved in this order:

1. `EGET_CONFIG`
2. `~/.config/eget/eget.toml`
3. XDG / LocalAppData fallback path
4. Legacy `~/.eget.toml`

Supported config sections:

- `[global]`
- `["owner/repo"]`
- `[packages.<name>]`
- `[sdk.<name>]`

Example:

```toml
[global]
target = "~/.local/bin"
cache_dir = "~/.cache/eget"
proxy_url = "http://127.0.0.1:7890"
system = "windows/amd64"
sys7z_path = ""
chunk_concurrency = 0
batch_concurrency = 0
ignore_update_packages = []
sdk_target = "~/sdks"
sdk_ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }

[api_cache]
enable = false
cache_time = 300

[ghproxy]
enable = false
host_url = ""
support_api = true
fallbacks = []

["inhere/markview"]
tag = "nightly"

[packages.markview]
repo = "inhere/markview"
target = "~/.local/bin"
tag = "nightly"
asset_filters = ["windows"]

[packages.winmerge]
repo = "sourceforge:winmerge"
source_path = "stable"
system = "windows/amd64"
asset_filters = ["x64", "PerUser", "setup"]

[packages.forgejo]
repo = "gitea:codeberg.org/forgejo/forgejo"
system = "linux/amd64"
asset_filters = ["linux", "amd64"]

[sdk.go]
aliases = ["golang"]
target = "gosdk/go{version}"
url_template = "https://mirrors.aliyun.com/golang/go{version}.{os}-{arch}.{ext}"
index_url = "https://mirrors.aliyun.com/golang/"
index_format = "html"
filename_pattern = "go{version}.{os}-{arch}.{ext}"
strip_components = 1
ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }

[sdk.node]
aliases = ["nodejs"]
target = "nodejs/node{version}"
url_template = "https://cdn.npmmirror.com/binaries/node/v{version}/node-v{version}-{os}-{arch}.{ext}"
index_url = "https://registry.npmmirror.com/binary.html"
index_format = "html"
index_path_prefix = "/binaries/node/"
filename_pattern = "node-v{version}-{os}-{arch}.{ext}"
strip_components = 1
os_map = { windows = "win", linux = "linux", darwin = "darwin" }
arch_map = { amd64 = "x64", arm64 = "arm64", 386 = "x86" }
ext_map = { windows = "zip", linux = "tar.xz", darwin = "tar.gz" }
```

Common fields:

- `target`
- `gui_target`
- `cache_dir`
- `proxy_url`
- `sys7z_path`
- `chunk_concurrency`
- `batch_concurrency`
- `sdk_target`
- `sdk_ext_map`
- `api_cache.enable`
- `api_cache.cache_time`
- `ghproxy.enable`
- `ghproxy.host_url`
- `ghproxy.support_api`
- `ghproxy.fallbacks`
- `system`
- `tag`
- `source_path`
- `file`
- `asset_filters`
- `download_source`
- `extract_all`
- `is_gui`
- `quiet`
- `upgrade_only`
- `sdk.<name>.aliases`
- `sdk.<name>.target`
- `sdk.<name>.url_template`
- `sdk.<name>.index_url`
- `sdk.<name>.index_format`
- `sdk.<name>.index_parser`
- `sdk.<name>.index_path_prefix`
- `sdk.<name>.filename_pattern`
- `sdk.<name>.strip_components`
- `sdk.<name>.os_map`
- `sdk.<name>.arch_map`
- `sdk.<name>.ext_map`

Default initialization:

```bash
eget config init
```

This writes:

- `global.target = "~/.local/bin"`
- `global.cache_dir = "~/.cache/eget"`
- `global.proxy_url = ""`
- `global.sys7z_path = ""`
- `global.chunk_concurrency = 0`
- `global.batch_concurrency = 0`
- `global.ignore_update_packages = []`
- `api_cache.enable = false`
- `api_cache.cache_time = 300`
- `ghproxy.enable = false`
- `ghproxy.host_url = ""`
- `ghproxy.support_api = true`

By default, the file is created at `~/.config/eget/eget.toml`.

Directory semantics:

- `target` is the default install directory
- `cache_dir` is the default download cache directory
- `proxy_url` is the global proxy for remote requests; both GitHub lookups and remote downloads use it
- `sys7z_path` is an optional 7z executable path. When empty, eget searches `PATH` for `7z`, `7zz`, then `7za`
- `source_path` narrows SourceForge discovery under a project's files area, for example `stable`
- `api_cache` caches known provider metadata `GET` responses, including GitHub API, GitLab/Gitea release API, and SourceForge files listings; the cache file directory is derived as `{cache_dir}/api-cache/`
- `cache_time` is measured in seconds; expired cache entries are refreshed from the network
- `ghproxy` rewrites GitHub asset download URLs; when `support_api = true`, it also rewrites `api.github.com` requests
- `ghproxy.fallbacks` are tried in order when the primary ghproxy host fails
- `proxy_url` is the HTTP-layer proxy, while `ghproxy` rewrites request URLs; both can be enabled together
- `download` uses `cache_dir` by default when `--to` is not provided
- `install` and `download` will reuse cached remote download contents from `cache_dir` when available
- `ignore_update_packages` skips named packages during `list --outdated`, `update --check`, and `update --all`
- `sdk_target` is the root directory for SDK installations. Relative SDK `target` values are resolved under this root
- `sdk_ext_map` is the default archive extension map by Go OS name; SDK-level `ext_map` overrides it
- `sdk.<name>.target` is the install directory template. Supported variables are `{name}`, `{version}`, `{os}`, `{arch}`, and `{ext}`
- `sdk.<name>.url_template` is the archive URL template for exact-version installs
- `sdk.<name>.index_url` points to an HTML or JSON index used for `latest` and prefix versions such as `go:1.22`
- `sdk.<name>.index_format = "html"` parses links from `<a href>`. JSON indexes require a supported `index_parser`, currently `go-json` or `node-json`
- `sdk.<name>.filename_pattern` describes archive filenames when parsing HTML indexes
- `sdk.<name>.strip_components` removes leading path segments during archive extraction, useful for archives that contain a top-level `go/` or `node-v.../` directory

The installed-state store also defaults to `~/.config/eget/installed.toml`.
The SDK installed-state store defaults to `~/.config/eget/sdk.installed.json`.

## Build and Test

```bash
make build
make test
```

## Project Structure

The current version has been restructured into an explicit subcommand CLI, with the entry point in `cmd/eget/main.go` and business logic concentrated under `internal/`.

- `cmd/eget`: command entry point
- `internal/cli`: `gcli` command registration and argument binding
- `internal/app`: install/add/list/update/config use-case orchestration
- `internal/install`: find, download, verify, and extract execution pipeline
- `internal/config`: config loading, merging, and persistence
- `internal/installed`: installed-state storage
- `internal/sdk`: SDK target parsing, index cache, resumable downloads, extraction, and SDK installed-state storage
- `internal/source/github`: GitHub asset discovery
- `internal/source/forge`: GitLab/Gitea/Forgejo asset discovery

> For more details, see [docs/DOCS.md](docs/DOCS.md).

## References

- [https://github.com/zyedidia/eget](https://github.com/zyedidia/eget)
- [https://github.com/gmatheu/eget](https://github.com/gmatheu/eget)
