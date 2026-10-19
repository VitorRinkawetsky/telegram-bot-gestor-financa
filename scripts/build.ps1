param([ValidateSet('amd64','arm64','all')][string]$Arch = 'all')
$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
    $env:GOOS = 'linux'
    $env:CGO_ENABLED = '0'
    $targets = if ($Arch -eq 'all') { @('amd64','arm64') } else { @($Arch) }
    foreach ($target in $targets) {
        $env:GOARCH = $target
        New-Item -ItemType Directory -Path "bin/linux-$target" -Force | Out-Null
        go build -trimpath -ldflags='-s -w' -o "bin/linux-$target/finance-bot" ./cmd/bot
        if ($LASTEXITCODE -ne 0) { throw "Falha no build do bot: $target" }
        go build -trimpath -ldflags='-s -w' -o "bin/linux-$target/finance-dbtool" ./cmd/dbtool
        if ($LASTEXITCODE -ne 0) { throw "Falha no build do dbtool: $target" }
        Get-FileHash -Algorithm SHA256 "bin/linux-$target/finance-bot", "bin/linux-$target/finance-dbtool"
    }
} finally {
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
