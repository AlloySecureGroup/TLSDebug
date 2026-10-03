$ErrorActionPreference = "Stop"

$GuiDir = Split-Path -Parent $PSScriptRoot
$RepoDir = Split-Path -Parent $GuiDir
$BinDir = Join-Path $GuiDir "bin"
$ProxyPath = Join-Path $BinDir "tlsproxy.exe"

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
go build -o $ProxyPath (Join-Path $RepoDir "tlsproxy.go")

$env:TLSDEBUG_PROXY_BIN = $ProxyPath
$env:TLSDEBUG_ROOT = $RepoDir
Push-Location $GuiDir
try {
    go run github.com/wailsapp/wails/v2/cmd/wails@v2.16.0 dev
}
finally {
    Pop-Location
}
