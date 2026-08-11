# Real-PC agents (AnyDesk-style)

This mode connects to **real machines**, not Docker desktops.

## Flow

1. Sign in to the control panel.
2. **Add Real PC (Agent)** → copy the one-time token / install hint.
3. On the target Windows PC, run:

```powershell
.\rd-agent.exe --server http://YOUR-PANEL --token YOUR_TOKEN
```

4. When the agent shows **online**, click **Connect**.
5. The panel opens a WebRTC viewer of that PC's real screen (mouse/keyboard forwarded).

## Build the agent (on a machine with Go)

```powershell
cd agent
go mod tidy
go build -o rd-agent.exe ./cmd/rd-agent
```

## Roles

| Action | How |
|--------|-----|
| Server → client | Operator uses the web panel → Connect to an online agent |
| Client → client | Install agent on PC A; from PC B open the same panel (or later a viewer mode) and Connect to A |

## Notes

- MVP streams ~10 FPS JPEG over a WebRTC datachannel (not hardware H.264 yet).
- Signaling requires a valid session key (`/ws/signal?...&key=`).
- Coturn (TURN) is used when direct P2P fails (NAT).
- Container desktop/relay remains available as an optional legacy mode.
