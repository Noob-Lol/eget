param(
    [string]$InstallDir = "$HOME\.local\bin"
)

$ErrorActionPreference = "Stop"

$repo = "inherelab/eget"
$bin = "eget"
$arch = $env:PROCESSOR_ARCHITECTURE

switch ($arch) {
    "AMD64" { $arch = "amd64" }
    default { throw "unsupported arch: $arch" }
}

$asset = "$bin-windows-$arch.zip"
$url = "https://github.com/$repo/releases/latest/download/$asset"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

try {
    $zip = Join-Path $tmp $asset
    Invoke-WebRequest -Uri $url -OutFile $zip
    Expand-Archive -LiteralPath $zip -DestinationPath $tmp -Force

    $src = Join-Path $tmp "$bin-windows-$arch.exe"
    $dst = Join-Path $InstallDir "$bin.exe"
    Copy-Item -LiteralPath $src -Destination $dst -Force

    Write-Host "installed $bin to $dst"
    $paths = [Environment]::GetEnvironmentVariable("Path", "User") -split ";"
    if ($paths -notcontains $InstallDir) {
        Write-Host "add $InstallDir to user PATH to run $bin from anywhere"
    }
}
finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
