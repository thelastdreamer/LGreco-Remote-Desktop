param(
    [ValidateSet("x86", "x64", "both")]
    [string]$Architecture = "both",
    [string]$OutputDir = "dist",
    [string]$Version = "1.0.0"
)

$ErrorActionPreference = "Stop"
$ProjectDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ClientDir = Join-Path $ProjectDir "windows-client\RemoteDesktop.Client"
$BridgeDir = Join-Path $ProjectDir "client-bridge"
$RootOutput = Join-Path $ProjectDir $OutputDir

Write-Host "====================================" -ForegroundColor Blue
Write-Host "  Remote Desktop Client - Build" -ForegroundColor Blue
Write-Host "====================================" -ForegroundColor Blue
Write-Host ""

# Step 1: Build Go bridge
Write-Host "[1/3] Building rd-bridge.exe..." -ForegroundColor Green

if (-not (Get-Command "go" -ErrorAction SilentlyContinue)) {
    Write-Host "  Go not installed. Install from https://go.dev/dl/" -ForegroundColor Yellow
    Write-Host "  Skipping bridge build (must be pre-built)" -ForegroundColor Yellow
} else {
    Push-Location $BridgeDir
    try {
        if ($Architecture -eq "x86" -or $Architecture -eq "both") {
            Write-Host "  Building bridge x86..." -ForegroundColor Gray
            $env:GOOS = "windows"; $env:GOARCH = "386"; $env:CGO_ENABLED = "0"
            & go build -ldflags="-s -w" -o "$RootOutput\rd-bridge-x86.exe" .\cmd\bridge\
            Write-Host "    -> dist\rd-bridge-x86.exe" -ForegroundColor Gray
        }
        if ($Architecture -eq "x64" -or $Architecture -eq "both") {
            Write-Host "  Building bridge x64..." -ForegroundColor Gray
            $env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
            & go build -ldflags="-s -w" -o "$RootOutput\rd-bridge-x64.exe" .\cmd\bridge\
            Write-Host "    -> dist\rd-bridge-x64.exe" -ForegroundColor Gray
        }
    } finally {
        Pop-Location
    }
}

# Step 2: Build WPF client
Write-Host "[2/3] Building WPF client..." -ForegroundColor Green

if (-not (Get-Command "dotnet" -ErrorAction SilentlyContinue)) {
    Write-Host "  .NET SDK not found. Install from https://dotnet.microsoft.com/download" -ForegroundColor Red
    exit 1
}

Push-Location $ClientDir
try {
    if ($Architecture -eq "x86" -or $Architecture -eq "both") {
        Write-Host "  Building WPF client x86..." -ForegroundColor Gray
        dotnet publish -c Release -r win-x86 --self-contained true `
            -p:Version=$Version `
            -o "$RootOutput\win-x86" `
            /p:PublishSingleFile=false /p:IncludeNativeLibrariesForSelfExtract=true
        Copy-Item "$RootOutput\rd-bridge-x86.exe" "$RootOutput\win-x86\rd-bridge.exe" -Force -ErrorAction SilentlyContinue
        Write-Host "    -> dist\win-x86\" -ForegroundColor Gray
    }
    if ($Architecture -eq "x64" -or $Architecture -eq "both") {
        Write-Host "  Building WPF client x64..." -ForegroundColor Gray
        dotnet publish -c Release -r win-x64 --self-contained true `
            -p:Version=$Version `
            -o "$RootOutput\win-x64" `
            /p:PublishSingleFile=false /p:IncludeNativeLibrariesForSelfExtract=true
        Copy-Item "$RootOutput\rd-bridge-x64.exe" "$RootOutput\win-x64\rd-bridge.exe" -Force -ErrorAction SilentlyContinue
        Write-Host "    -> dist\win-x64\" -ForegroundColor Gray
    }
} finally {
    Pop-Location
}

# Step 3: Create installer copy script
Write-Host "[3/3] Creating launcher..." -ForegroundColor Green

@"
@echo off
echo Starting Remote Desktop Client...
start "" "%~dp0RemoteDesktop.Client.exe"
"@ | Out-File -FilePath "$RootOutput\win-x64\launch.bat" -Encoding ASCII -ErrorAction SilentlyContinue
@"
@echo off
echo Starting Remote Desktop Client (32-bit)...
start "" "%~dp0RemoteDesktop.Client.exe"
"@ | Out-File -FilePath "$RootOutput\win-x86\launch.bat" -Encoding ASCII -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "====================================" -ForegroundColor Blue
Write-Host "  Build Complete! v$Version" -ForegroundColor Green
Write-Host "====================================" -ForegroundColor Blue
Write-Host "  x64: dist\win-x64\RemoteDesktop.Client.exe" -ForegroundColor White
Write-Host "  x86: dist\win-x86\RemoteDesktop.Client.exe" -ForegroundColor White
Write-Host ""
