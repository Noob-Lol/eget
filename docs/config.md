# Configuration Reference

This document describes the `eget` configuration file. The README keeps only a short overview; use this file when you need the complete field list and directory semantics.

## Config Lookup

`eget` resolves the config file in this order:

1. `EGET_CONFIG`
2. `~/.config/eget/eget.toml`
3. XDG / LocalAppData fallback path
4. Legacy `~/.eget.toml`

Create the default config:

```bash
eget config init
```

By default, this writes:

```text
~/.config/eget/eget.toml
```

## Sections

Supported sections:

- `[global]`: global defaults and network/cache settings.
- `[api_cache]`: metadata API response cache.
- `[ghproxy]`: GitHub URL rewrite proxy.
- `["owner/repo"]`: legacy direct package section.
- `[packages.<name>]`: named package section.
- `[sdk.<name>]`: SDK download and index section.

## Global Section

Example:

```toml
[global]
target = "~/.local/bin"
gui_target = "~/Applications"
cache_dir = "~/.cache/eget"
proxy_url = ""
system = ""
sys7z_path = ""
chunk_concurrency = 0
batch_concurrency = 0
ignore_update_packages = []
sdk_target = "~/sdks"
sdk_ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }
```

Fields:

- `target`: default install directory for CLI tools.
- `gui_target`: default install directory for portable GUI applications.
- `cache_dir`: default cache root. Raw downloads, API cache files, SDK downloads, and SDK indexes are derived from this directory.
- `proxy_url`: HTTP-layer proxy for remote requests. GitHub lookups and remote downloads both use it.
- `system`: default target platform in `GOOS/GOARCH` form, for example `windows/amd64`.
- `sys7z_path`: optional 7z executable path. When empty, eget searches `PATH` for `7z`, `7zz`, then `7za`.
- `chunk_concurrency`: default remote download chunk concurrency. `0` means the built-in default behavior.
- `batch_concurrency`: default concurrency for batch package operations. `0` means serial or command-specific default behavior.
- `ignore_update_packages`: package names skipped by `list --outdated`, `update --check`, and `update --all`.
- `sdk_target`: SDK installation root. Relative SDK `target` values are resolved under this root.
- `sdk_ext_map`: default SDK archive extension map by Go OS name. SDK-level `ext_map` overrides it.

Directory semantics:

- `download` uses `cache_dir` by default when `--to` is not provided.
- `install` and `download` reuse cached remote download contents from `cache_dir` when available.
- SDK archive downloads are stored under `{cache_dir}/sdk-downloads/`.
- SDK index JSON files are stored under `{cache_dir}/sdk-index/`.

## API Cache

Example:

```toml
[api_cache]
enable = false
cache_time = 300
```

Fields:

- `enable`: whether to cache known provider metadata responses.
- `cache_time`: cache TTL in seconds.

The API cache stores known provider metadata `GET` responses, including GitHub API, GitLab/Gitea release API, and SourceForge files listings. Cache files are stored under `{cache_dir}/api-cache/`.

## GitHub Proxy

Example:

```toml
[ghproxy]
enable = false
host_url = ""
support_api = true
fallbacks = []
```

Fields:

- `enable`: enable GitHub URL rewriting.
- `host_url`: primary proxy host, for example `https://ghfast.top/`.
- `support_api`: also rewrite `api.github.com` requests when enabled.
- `fallbacks`: fallback proxy hosts tried in order when the primary proxy fails.

`proxy_url` and `ghproxy` solve different problems. `proxy_url` is an HTTP-layer proxy, while `ghproxy` rewrites request URLs. They can be enabled together.

## Package Sections

Use `[packages.<name>]` for named package management.

Example:

```toml
[packages.markview]
repo = "inhere/markview"
target = "~/.local/bin"
tag = "nightly"
asset_filters = ["windows"]
```

Common fields:

- `repo`: package source. Supports GitHub-style `owner/repo`, direct URLs, SourceForge, and supported forge prefixes.
- `target`: install directory for this package.
- `system`: package-specific target platform in `GOOS/GOARCH` form.
- `tag`: version tag or release tag preference.
- `source_path`: SourceForge files path filter, for example `stable`.
- `file`: file filter or output filename depending on command context.
- `asset_filters`: substrings used to match release assets.
- `download_source`: download source archives instead of release assets.
- `extract_all`: extract all files from the selected archive.
- `is_gui`: install as GUI package, using `gui_target` semantics.
- `quiet`: reduce output for this package.
- `upgrade_only`: only update when an installed package already exists.

Legacy direct sections are also supported:

```toml
["inhere/markview"]
tag = "nightly"
```

Prefer `[packages.<name>]` for new config because it provides an explicit local package name.

## SDK Sections

Use `[sdk.<name>]` to configure SDK archive downloads.

Example:

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

Fields:

- `aliases`: SDK aliases. For example, `[sdk.go]` with `aliases = ["golang"]` allows `eget sdk install golang@1.22.0`.
- `target`: installation directory template. Relative paths are resolved under `global.sdk_target`.
- `url_template`: archive download URL template.
- `index_url`: remote HTML or JSON index URL.
- `index_format`: index format, usually `html` or `json`.
- `index_parser`: JSON index parser. Currently supported values are `go-json` and `node-json`.
- `index_path_prefix`: path prefix filter for HTML index links.
- `filename_pattern`: archive filename pattern used by HTML index parsing.
- `strip_components`: number of leading archive path segments to remove during extraction.
- `os_map`: map Go OS names to SDK release OS names.
- `arch_map`: map Go arch names to SDK release arch names.
- `ext_map`: map Go OS names to SDK archive extensions. Overrides `global.sdk_ext_map`.

Template variables supported by `target`, `url_template`, and `filename_pattern`:

- `{name}`: SDK name.
- `{version}`: selected version.
- `{os}`: OS value after `os_map`.
- `{arch}`: arch value after `arch_map`.
- `{ext}`: archive extension after `ext_map`.

HTML index parsing supports two common layouts:

- Direct archive links, such as `go1.22.0.linux-amd64.tar.gz`.
- Version directory links, such as `v20.11.1/`. When `url_template` is configured, eget builds the current-platform archive URL from the directory version.

For SDK usage details, see [sdk-usage.md](sdk-usage.md).

## Store Files

Package install records default to:

```text
~/.config/eget/installed.toml
```

SDK install records default to:

```text
~/.config/eget/sdk.installed.json
```

SDK records are separate because SDKs commonly have multiple installed versions, while normal packages are usually managed as one active installed artifact.

## Full Example

See [example.eget.toml](example.eget.toml) for a larger example covering packages, Go, Node, Python, and JDK-style SDK experiments.
