# Remote Desktop - Deployment Guide

## One-Click Deploy (Dokploy)

### Option 1: Via Dokploy Dashboard

1. Open your Dokploy dashboard
2. Click **New Application** → **Docker Compose**
3. Name: `remote-desktop`
4. Paste the contents of `docker-compose.yml`
5. Set the environment variables from `.env.example`
6. Add the `dokploy.json` as application config
7. Click **Deploy**

### Option 2: Via Script

```bash
# On your Dokploy server
chmod +x scripts/deploy.sh
./scripts/deploy.sh
```

### Option 3: Manual Docker Compose

```bash
cp .env.example .env
# Edit .env with your settings
docker compose up -d
```

## First Run - Create Admin User

```bash
curl -X POST http://your-server:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","email":"admin@example.com","password":"yourpassword"}'
```

## Windows Client Setup

1. Run `scripts/build-client.ps1` to build both x86 and x64
2. Distribute the `dist\win-x64\` or `dist\win-x86\` folder to users
3. Users open `RemoteDesktop.Client.exe`
4. Enter server URL, username, password → Connect
5. Click "+ New Desktop" to start a session

## Architecture

```
Client (WPF) <-> rd-bridge (Go/Pion WebRTC) <-> Server (Go/Pion WebRTC) <-> Desktop Container (Xvfb+XFCE)
```

## Ports

| Port | Service | Protocol |
|------|---------|----------|
| 8080 | API Server | HTTP |
| 3478 | Coturn (STUN/TURN) | UDP + TCP |
| 5349 | Coturn (TLS) | TCP |
| 49152-65535 | TURN Relay Range | UDP |

## Environment Variables

See `.env.example` for all configuration options.
