(() => {
  const params = new URLSearchParams(location.search);
  const sessionId = params.get("session");
  const key = params.get("key");
  const hud = document.getElementById("hud");
  const screen = document.getElementById("screen");

  if (!sessionId || !key) {
    hud.textContent = "Missing session or key";
    return;
  }

  const iceServers = [];
  let pc;
  let control;
  let objectUrl;
  let signalWS;

  function setHud(text) {
    hud.textContent = text;
  }

  function wsURL(path) {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    return `${proto}//${location.host}${path}`;
  }

  async function loadICE() {
    try {
      const token = localStorage.getItem("rd_token") || "";
      const res = await fetch(`/api/sessions/${sessionId}/ice-servers`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) return;
      const data = await res.json();
      for (const s of data) {
        const entry = { urls: s.urls };
        if (s.username) {
          entry.username = s.username;
          entry.credential = s.credential;
        }
        iceServers.push(entry);
      }
    } catch (_) {
      iceServers.push({ urls: "stun:stun.l.google.com:19302" });
    }
  }

  function sendInput(ev) {
    if (!control || control.readyState !== "open") return;
    control.send(JSON.stringify(ev));
  }

  function bindInput() {
    const rect = () => screen.getBoundingClientRect();
    const norm = (e) => {
      const r = rect();
      return {
        x: Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)),
        y: Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)),
      };
    };

    screen.addEventListener("mousemove", (e) => {
      const p = norm(e);
      sendInput({ type: "mousemove", x: p.x, y: p.y });
    });
    screen.addEventListener("mousedown", (e) => {
      e.preventDefault();
      const p = norm(e);
      sendInput({ type: "mousedown", x: p.x, y: p.y, button: e.button === 2 ? 2 : 0 });
    });
    screen.addEventListener("mouseup", (e) => {
      e.preventDefault();
      const p = norm(e);
      sendInput({ type: "mouseup", x: p.x, y: p.y, button: e.button === 2 ? 2 : 0 });
    });
    screen.addEventListener("contextmenu", (e) => e.preventDefault());
    screen.addEventListener("wheel", (e) => {
      e.preventDefault();
      sendInput({ type: "wheel", delta: e.deltaY < 0 ? 120 : -120 });
    }, { passive: false });

    window.addEventListener("keydown", (e) => {
      sendInput({ type: "keydown", key: e.key, keycode: e.keyCode });
    });
    window.addEventListener("keyup", (e) => {
      sendInput({ type: "keyup", key: e.key, keycode: e.keyCode });
    });
  }

  async function start() {
    await loadICE();
    pc = new RTCPeerConnection({ iceServers });

    pc.ondatachannel = (ev) => {
      const dc = ev.channel;
      if (dc.label === "screen") {
        dc.binaryType = "arraybuffer";
        dc.onmessage = (msg) => {
          const blob = new Blob([msg.data], { type: "image/jpeg" });
          if (objectUrl) URL.revokeObjectURL(objectUrl);
          objectUrl = URL.createObjectURL(blob);
          screen.src = objectUrl;
          setHud(`Session #${sessionId} · live`);
        };
      }
      if (dc.label === "control") {
        control = dc;
      }
    };

    pc.onicecandidate = (ev) => {
      if (!ev.candidate || !signalWS || signalWS.readyState !== WebSocket.OPEN) return;
      signalWS.send(JSON.stringify({
        type: "ice-candidate",
        candidate: {
          candidate: ev.candidate.candidate,
          sdpMid: ev.candidate.sdpMid,
          sdpMLineIndex: ev.candidate.sdpMLineIndex,
        },
      }));
    };

    signalWS = new WebSocket(wsURL(`/ws/signal?session=${encodeURIComponent(sessionId)}&role=viewer&key=${encodeURIComponent(key)}`));
    signalWS.onopen = () => setHud("Waiting for agent offer…");
    signalWS.onmessage = async (ev) => {
      const msg = JSON.parse(ev.data);
      if (msg.type === "offer") {
        await pc.setRemoteDescription({ type: "offer", sdp: msg.sdp });
        const answer = await pc.createAnswer();
        await pc.setLocalDescription(answer);
        signalWS.send(JSON.stringify({ type: "answer", sdp: answer.sdp }));
        setHud("Negotiating…");
      }
      if (msg.type === "ice-candidate" && msg.candidate) {
        try {
          await pc.addIceCandidate(msg.candidate);
        } catch (_) {}
      }
    };
    signalWS.onerror = () => setHud("Signaling error");
    signalWS.onclose = () => setHud("Signaling closed");

    bindInput();
  }

  start().catch((err) => {
    setHud(String(err));
  });
})();
