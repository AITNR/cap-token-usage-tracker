$ErrorActionPreference = "Stop"

$rootDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1"
$env:Path = "C:\mingw64\mingw64\bin;" + $env:Path

Push-Location $rootDir
try {
    New-Item -ItemType Directory -Force -Path (Join-Path $rootDir "dist") | Out-Null
    go build -buildmode=c-shared -buildvcs=false -o (Join-Path $rootDir "dist\cap-token-usage-tracker.dll") .
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
    Get-Item (Join-Path $rootDir "dist\cap-token-usage-tracker.dll") | Format-List Name, Length, LastWriteTime
} finally {
    Pop-Location
}
