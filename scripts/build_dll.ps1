$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1"
$env:Path = "C:\mingw64\mingw64\bin;" + $env:Path
Set-Location D:\c\cap-token-usage-tracker
New-Item -ItemType Directory -Force -Path dist | Out-Null
go build -buildmode=c-shared -buildvcs=false -o dist\cap-token-usage-tracker.dll .
Write-Output "DLL_BUILD_EXIT=$LASTEXITCODE"
Get-Item dist\cap-token-usage-tracker.dll | Format-List Name, Length, LastWriteTime
