
## Go

Go 官方源有 HTML 下载页 `https://go.dev/dl/`，当前内置官方配置使用该页面。
Go 也提供 JSON index：`https://go.dev/dl/?mode=json`，可配合 `index_parser = "go-json"` 使用。

## Nodejs

Node.js 官方源有 HTML 目录 `https://nodejs.org/dist/`，当前内置官方配置使用该目录。
Node.js 也提供 JSON index：`https://nodejs.org/dist/index.json`，可配合 `index_parser = "node-json"` 使用。

## JDK

JDK 官方可用 `https://jdk.java.net/archive/` 归档页，当前内置官方配置已支持该 HTML 页面。
Azul Zulu 使用 JSON API，需要配置 `index_parser = "zulu-json"`。

### mirrors.huaweicloud.com openjdk

```toml
[sdk.jdk]
aliases = ["java"]
target = "jdk/openjdk-{version}"
index_url = "https://mirrors.huaweicloud.com/openjdk/"
index_format = "html"
url_template = "https://mirrors.huaweicloud.com/openjdk/{version}/openjdk-{version}_{os}-{arch}_bin.{ext}"
filename_pattern = "openjdk-{version}_{os}-{arch}_bin.{ext}"
strip_components = 1
arch_map = { amd64 = "x64", arm64 = "aarch64"}
os_map = { darwin = "macos" }
ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }
```

### api.azul.com zulu jdk

```toml
[sdk.jdk]
aliases = ["java"]
target = "jdk/zulu-{version}"
index_url = "https://api.azul.com/metadata/v1/zulu/packages?java_package_type=jdk&release_status=ga&availability_type=CA"
index_format = "json"
index_parser = "zulu-json"
strip_components = 1
arch_map = { amd64 = "x64", arm64 = "aarch64" }
os_map = { windows = "win", darwin = "macosx" }
ext_map = { windows = "zip", linux = "tar.gz", darwin = "tar.gz" }
```
