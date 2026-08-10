param(
    [string]$Domain = "localhost",
    [string]$PublicIP = "",
    [switch]$DokployMode
)

$ErrorActionPreference = "Stop"
$ProjectDir = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host "====================================" -ForegroundColor Blue
Write-Host "  Remote Desktop - Deploy to Dokploy" -ForegroundColor Blue
Write-Host "====================================" -ForegroundColor Blue
Write-Host ""

# Generate .env for Dokploy
Write-Host "[1/4] Generating Dokploy configuration..." -ForegroundColor Green

$JwtSecret = -join ((48..57) + (65..90) + (97..122) | Get-Random -Count 64 | ForEach-Object { [char]$_ })
$DbPass = -join ((48..57) + (65..90) + (97..122) | Get-Random -Count 24 | ForEach-Object { [char]$_ })
$TurnPass = -join ((48..57) + (65..90) + (97..122) | Get-Random -Count 24 | ForEach-Object { [char]$_ })

@"
# Dokploy Environment Variables
DOMAIN=$Domain
PUBLIC_IP=$PublicIP

# Database
DB_USER=rduser
DB_PASSWORD=$DbPass
DB_NAME=remote_desktop

# API
API_PORT=8080
JWT_SECRET=$JwtSecret

# TURN / STUN
STUN_SERVER=stun:stun.l.google.com:19302
TURN_USERNAME=rdturnuser
TURN_PASSWORD=$TurnPass

# Docker
DOCKER_NETWORK=rd-network
"@ | Out-File -FilePath "$ProjectDir\.env.dokploy" -Encoding UTF8

Write-Host "  Generated .env.dokploy with secure random secrets" -ForegroundColor Gray

# Create Dokploy application config
Write-Host "[2/4] Creating Dokploy application config..." -ForegroundColor Green

@"
{
  "name": "remote-desktop",
  "type": "docker-compose",
  "description": "Self-hosted Remote Desktop solution with WebRTC streaming",
  "domains": [
    {
      "host": "${DOMAIN}",
      "port": 8080,
      "serviceName": "api",
      "https": true
    }
  ],
  "volumes": [
    {
      "name": "postgres_data",
      "mountPath": "/var/lib/postgresql/data",
      "size": "10Gi"
    },
    {
      "name": "redis_data",
      "mountPath": "/data",
      "size": "1Gi"
    }
  ]
}
"@ | Out-File -FilePath "$ProjectDir\dokploy.json" -Encoding UTF8

Write-Host "  Created dokploy.json" -ForegroundColor Gray

# Build instructions
Write-Host "[3/4] Build instructions for Dokploy:" -ForegroundColor Green
Write-Host ""
Write-Host "  1. Go to your Dokploy dashboard" -ForegroundColor White
Write-Host "  2. Create a new application" -ForegroundColor White
Write-Host "  3. Select 'Docker Compose' as the type" -ForegroundColor White
Write-Host "  4. Point to this repository or paste the docker-compose.yml" -ForegroundColor White
Write-Host "  5. Configure environment variables from .env.dokploy" -ForegroundColor White
Write-Host "  6. Deploy!" -ForegroundColor White
Write-Host ""

# For direct deploy
if (-not $DokployMode) {
    Write-Host "[4/4] Running local deployment..." -ForegroundColor Green
    docker compose up -d --build
    Write-Host "  Waiting for services..." -ForegroundColor Gray
    Start-Sleep -Seconds 5

    try {
        $health = Invoke-RestMethod -Uri "http://localhost:8080/health" -Method Get -ErrorAction Stop
        Write-Host "  Server healthy: $($health.status)" -ForegroundColor Green
    } catch {
        Write-Host "  Server not ready - check: docker compose logs api" -ForegroundColor Yellow
    }
}

Write-Host ""
Write-Host "====================================" -ForegroundColor Blue
Write-Host "  Deployment ready!" -ForegroundColor Green
Write-Host "  API: http://$Domain`:$([int]8080)" -ForegroundColor White
Write-Host "====================================" -ForegroundColor Blue
