$ErrorActionPreference = "Stop"

$rootDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$version = if ($env:VERSION) { $env:VERSION } else { "v1.0.0" }
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1"
$env:Path = "C:\mingw64\mingw64\bin;" + $env:Path

Push-Location $rootDir
try {
    New-Item -ItemType Directory -Force -Path (Join-Path $rootDir "dist") | Out-Null
    go build -buildmode=c-shared -buildvcs=false -ldflags="-s -w -X github.com/AITNR/cap-token-usage-tracker/internal/plugin.version=$version" -o (Join-Path $rootDir "dist\cap-token-usage-tracker.dll") .
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
    Get-Item (Join-Path $rootDir "dist\cap-token-usage-tracker.dll") | Format-List Name, Length, LastWriteTime
} finally {
    Pop-Location
}
