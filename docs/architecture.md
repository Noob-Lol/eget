# Eget Architecture

This document describes the internal execution model for contributors. User-facing configuration is documented in [config.md](config.md) and [config.zh-CN.md](config.zh-CN.md). SDK usage is documented in [sdk-usage.md](sdk-usage.md).

## CLI Shape

`eget` uses explicit subcommands:

```text
eget <command> --options... arguments...
```

Current commands:

- `install`
- `download`
- `add`
- `uninstall`
- `list`
- `update`
- `config`
- `sdk`
- `query`
- `search`

The root command does not perform a default install action. Because of the CLI parser model, flags must appear before positional arguments:

```text
eget install --tag nightly inhere/markview
```

## Runtime Layout

- `cmd/eget/main.go`: process entry point.
- `internal/cli`: `gookit/gcli` command registration, argument binding, handlers, and rendering.
- `internal/app`: use-case orchestration for install/add/list/update/config/search/query.
- `internal/install`: target detection, asset discovery, download, checksum, extraction, and runner pipeline.
- `internal/config`: config paths, loading, merging, and persistence.
- `internal/installed`: package installed-state store.
- `internal/sdk`: SDK target parsing, index cache, resumable downloads, extraction, and SDK installed-state store.
- `internal/source/github`: GitHub release/source discovery.
- `internal/source/sourceforge`: SourceForge file discovery and latest-version checks.
- `internal/source/forge`: GitLab/Gitea/Forgejo release asset discovery and latest-version checks.

## Install Flow

`install` is orchestrated by `internal/app/install.go` and `internal/install/runner.go`:

1. Parse the target type.
2. Select a finder.
3. Enumerate candidate assets.
4. Match assets by `system`, `asset_filters`, `file`, and command options.
5. Download content, reusing cache when possible.
6. Run SHA-256 auto verification when a matching checksum file exists.
7. Select an extractor and extract the requested files.
8. Write the package installed store.

`install --all` reads `[packages]`, sorts package names, and reuses the single-package install flow for each entry. `--batch N` or `global.batch_concurrency > 1` enables fixed-worker scheduling while preserving stable result ordering.

## Download Flow

`download` reuses the install execution pipeline with `DownloadOnly=true`.

Differences from install:

- It does not write the installed store.
- It stores the raw downloaded asset by default.
- Extraction only happens when `--file` or `--extract-all` is provided.

Remote URL downloads use a cache key derived from the URL hash. Cached files keep the original URL extension when possible and fall back to `.bin`.

## Source Backends

GitHub is handled by `internal/source/github`.

SourceForge targets use:

```text
sourceforge:<project>
```

`source_path` can restrict discovery to a project files subdirectory such as `stable`.

Forge targets use:

```text
gitlab:<owner>/<repo>
gitea:<host>/<owner>/<repo>
forgejo:<host>/<owner>/<repo>
```

Forge backends return candidate download URLs; asset filtering, download, checksum, extraction, and installed-store recording continue through the shared install pipeline.

## Extraction

Archive extraction supports `zip`, `tar.*`, and system-7z-backed formats.

System 7z is preferred for `.7z`, `.rar`, `.msi`, `.cab`, `.iso`, and `--extract-all` `.exe` archives when `global.sys7z_path` or `PATH` provides `7z`, `7zz`, or `7za`.

`tar.gz`, `.tgz`, `.tar.xz`, `.txz`, `.tar.bz2`, `.tbz`, and `.tar.zst` continue to use the built-in Go extractor so tar member selection and path safety checks remain stable.

## SDK Flow

`sdk` is a real multi-level `gcli` command tree:

```text
eget sdk install <target...>
eget sdk list [name]
eget sdk remove <name@version>
eget sdk index list|show|refresh|clear
```

The core implementation lives in `internal/sdk`:

1. `ParseTarget` parses SDK name and version semantics.
2. `ResolveConfig` merges `[global]` and `[sdk.<name>]`, including aliases and platform maps.
3. Exact versions can render `url_template` directly.
4. `latest` and prefix versions read normalized index cache.
5. `DownloadArchive` downloads to `{cache_dir}/sdk-downloads/{sdk}/{version}/`.
6. `.part` and `.meta.json` files support HTTP Range resume for large archives.
7. Extraction runs in `{sdk_target}/.eget-tmp/...`.
8. The final directory is renamed into place and recorded in `sdk.installed.json`.

`eget sdk` intentionally stops at download/install/index management. Version activation, `PATH`, shell hooks, and project-level environment switching belong to dedicated tools such as `kite xenv`.

## Managed Package Flows

`add` writes a reusable package definition to `[packages.<name>]` without downloading.

`uninstall` resolves package name or repo target, reads installed records, deletes recorded extracted files, and removes the installed-store entry. It does not remove `[packages.<name>]`.

`list` defaults to installed packages. `list --all` merges managed package definitions and installed records. `list --no-installed` filters that merged view down to configured packages that have no installed-store entry.

`update` checks latest version information before installing. `update --check` is equivalent to the outdated-check path; `update --all` updates managed packages with newer versions.

## Config Command

`config` is a `gcli` subcommand tree:

```text
eget config init
eget config list
eget config ls
eget config get KEY
eget config set KEY VALUE
```

Config data structures live in `internal/config/model.go`. Full user-facing field semantics live in [config.md](config.md) and [config.zh-CN.md](config.zh-CN.md).

## Concurrency

`chunk_concurrency` controls HTTP Range chunking for a single asset.

- `0`: automatic behavior.
- `1`: single connection.
- `>1`: requested max chunk count.

Chunking only starts when the server supports Range requests and returns a usable `Content-Length`.

`batch_concurrency` controls package-level task concurrency for `install --all` and `update --all`. It is global/CLI-scoped because it controls the whole batch scheduler.

## Stores

Package install records live in:

```text
~/.config/eget/installed.toml
```

SDK install records live in:

```text
~/.config/eget/sdk.installed.json
```

The stores are separate because SDKs commonly have multiple installed versions, while normal packages usually represent one active installed artifact.

## Verification

Common checks:

```bash
go test ./internal/app -v
go test ./internal/cli -v
go test ./...
make build
make test
```
