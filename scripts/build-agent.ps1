# Build Windows agent binary
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location (Join-Path $root "agent")
go mod tidy
go build -o rd-agent.exe ./cmd/rd-agent
Write-Host "Built: $(Join-Path (Get-Location) 'rd-agent.exe')"
