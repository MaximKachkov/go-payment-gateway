$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$bin = Join-Path $root "work\bin"
$buf = Join-Path $bin "buf.exe"

if (-not (Test-Path $buf)) {
    throw "buf.exe was not found. Run .\scripts\install-tools.ps1 first."
}

$env:PATH = "$bin;$env:PATH"

Push-Location $root
try {
    & $buf generate
}
finally {
    Pop-Location
}
