$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$bin = Join-Path $root "work\bin"

New-Item -ItemType Directory -Force -Path $bin | Out-Null

$env:GOBIN = $bin

go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/bufbuild/buf/cmd/buf@latest

Write-Host "Tools installed into $bin"
