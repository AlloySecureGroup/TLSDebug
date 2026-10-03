$ErrorActionPreference = "Stop"

$GuiDir = Split-Path -Parent $PSScriptRoot
$RepoDir = Split-Path -Parent $GuiDir
$BuildDir = Join-Path $GuiDir "build/bin"

Push-Location $GuiDir
try {
    go run github.com/wailsapp/wails/v2/cmd/wails@v2.16.0 build @args
    go build -o (Join-Path $BuildDir "tlsproxy.exe") (Join-Path $RepoDir "tlsproxy.go")
}
finally {
    Pop-Location
}

Write-Host "Built TLSDebug desktop application and proxy in $BuildDir"
