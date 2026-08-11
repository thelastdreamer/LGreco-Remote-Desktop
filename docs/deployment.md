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

## Default Admin

On first boot the server auto-creates a default admin from the environment:

- `DEFAULT_ADMIN_USERNAME`
- `DEFAULT_ADMIN_EMAIL`
- `DEFAULT_ADMIN_PASSWORD`

The default admin is marked with `password_change_required=true`, so after the first login
you must change the password before accessing session controls.

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

## Runtime Image Seeding

Dokploy builds the `api` image plus two lightweight seed services:

- `desktop-image` -> builds and keeps `rd-desktop:latest` available
- `relay-image` -> builds and keeps `rd-relay:latest` available

The API image also bakes `desktop-container/` and `relay-container/` into
`/build/...`. On startup and before creating a session, the API auto-builds any
missing runtime images through the mounted Docker socket.

Important for Dokploy:
1. Redeploy must rebuild the `api` image (not only restart containers)
2. Use the repository `docker-compose.yml` (do not keep an old pasted compose)
3. After deploy, check `/api/runtime` while logged in: `desktop_ready` should become `true`
4. The first image build can take several minutes

## Ports

| Port | Service | Protocol |
|------|---------|----------|
| 8080 | API Server | HTTP |
| 3478 | Coturn (STUN/TURN) | UDP + TCP |
| 49152-49201 | TURN Relay Range (50 ports) | UDP |

## Environment Variables

See `.env.example` for all configuration options.
