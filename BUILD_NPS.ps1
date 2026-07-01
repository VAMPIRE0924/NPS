$ErrorActionPreference = 'Stop'

$repo = Split-Path -Parent $MyInvocation.MyCommand.Path
$outRoot = Join-Path $repo 'dist'
$win = Join-Path $outRoot 'windows_amd64'
$linux = Join-Path $outRoot 'linux_amd64'

New-Item -ItemType Directory -Force -Path $win, $linux | Out-Null

go build -trimpath -ldflags='-s -w' -o (Join-Path $win 'nps.exe') ./cmd/nps

$oldGoos = $env:GOOS
$oldGoarch = $env:GOARCH
try {
    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    go build -trimpath -ldflags='-s -w' -o (Join-Path $linux 'nps') ./cmd/nps
} finally {
    if ($null -eq $oldGoos) {
        Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    } else {
        $env:GOOS = $oldGoos
    }
    if ($null -eq $oldGoarch) {
        Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    } else {
        $env:GOARCH = $oldGoarch
    }
}

foreach ($target in @($win, $linux)) {
    $web = Join-Path $target 'web'
    New-Item -ItemType Directory -Force -Path $web | Out-Null
    Copy-Item -Path (Join-Path $repo 'web\static') -Destination $web -Recurse -Force
    Copy-Item -Path (Join-Path $repo 'web\views') -Destination $web -Recurse -Force
}

Get-Item (Join-Path $win 'nps.exe'), (Join-Path $linux 'nps') |
    Select-Object FullName, Length
