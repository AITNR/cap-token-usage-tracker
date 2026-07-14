$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1"
$env:Path = "C:\mingw64\mingw64\bin;" + $env:Path
Set-Location D:\c\cap-token-usage-tracker
go build -buildmode=c-shared -buildvcs=false -o cap-token-usage-tracker.dll .
Write-Output "DLL_BUILD_EXIT=$LASTEXITCODE"
Get-Item cap-token-usage-tracker.dll | Format-List Name, Length, LastWriteTime
