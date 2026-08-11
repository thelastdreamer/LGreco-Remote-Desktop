const state = {
  token: localStorage.getItem("rd_token") || "",
  user: null,
  sessions: [],
  agents: [],
  activeSession: null,
};

const els = {
  authCard: document.getElementById("authCard"),
  sessionCard: document.getElementById("sessionCard"),
  dashboard: document.getElementById("dashboard"),
  loginForm: document.getElementById("loginForm"),
  loginError: document.getElementById("loginError"),
  username: document.getElementById("username"),
  password: document.getElementById("password"),
  sessionType: document.getElementById("sessionType"),
  relayHostField: document.getElementById("relayHostField"),
  targetHost: document.getElementById("targetHost"),
  createForm: document.getElementById("createForm"),
  agentForm: document.getElementById("agentForm"),
  agentName: document.getElementById("agentName"),
  agentInstallHint: document.getElementById("agentInstallHint"),
  agentList: document.getElementById("agentList"),
  passwordPanel: document.getElementById("passwordPanel"),
  passwordForm: document.getElementById("passwordForm"),
  currentPassword: document.getElementById("currentPassword"),
  newPassword: document.getElementById("newPassword"),
  passwordError: document.getElementById("passwordError"),
  sessionList: document.getElementById("sessionList"),
  sessionMeta: document.getElementById("sessionMeta"),
  runtimeMeta: document.getElementById("runtimeMeta"),
  userBadge: document.getElementById("userBadge"),
  logoutBtn: document.getElementById("logoutBtn"),
  refreshBtn: document.getElementById("refreshBtn"),
  openViewerBtn: document.getElementById("openViewerBtn"),
  stopSessionBtn: document.getElementById("stopSessionBtn"),
  viewerEmpty: document.getElementById("viewerEmpty"),
  viewerFrame: document.getElementById("viewerFrame"),
  resolution: document.getElementById("resolution"),
};

function setAuth(token) {
  state.token = token || "";
  if (state.token) {
    localStorage.setItem("rd_token", state.token);
    document.cookie = `jwt=${state.token}; Path=/; SameSite=Lax; Max-Age=${72 * 60 * 60}`;
  } else {
    localStorage.removeItem("rd_token");
    document.cookie = "jwt=; Path=/; Max-Age=0; SameSite=Lax";
  }
}

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (!headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }
  if (state.token) {
    headers.set("Authorization", `Bearer ${state.token}`);
  }

  const res = await fetch(path, { ...options, headers });
  if (res.status === 401) {
    logout();
    throw new Error("Authentication expired");
  }
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    throw new Error(data?.error || "Request failed");
  }
  return data;
}

function renderAuth() {
  const loggedIn = Boolean(state.token && state.user);
  els.authCard.classList.toggle("hidden", loggedIn);
  els.sessionCard.classList.toggle("hidden", !loggedIn);
  els.dashboard.classList.toggle("hidden", !loggedIn);
  els.userBadge.classList.toggle("hidden", !loggedIn);
  els.logoutBtn.classList.toggle("hidden", !loggedIn);
  if (loggedIn) {
    els.userBadge.textContent = `${state.user.username} · ${state.user.email}`;
  }
  renderPasswordState();
}

function renderPasswordState() {
  const required = Boolean(state.user?.password_change_required);
  els.passwordPanel.classList.toggle("hidden", !required);
  els.createForm.querySelectorAll("input, select, button").forEach((el) => {
    el.disabled = required;
  });
  els.agentForm.querySelectorAll("input, button").forEach((el) => {
    el.disabled = required;
  });
  els.refreshBtn.disabled = false;
  els.openViewerBtn.disabled = required;
  els.stopSessionBtn.disabled = required;
}

function renderAgents() {
  els.agentList.innerHTML = "";
  if (!state.agents.length) {
    els.agentList.innerHTML = `<div class="session-item"><div class="muted">No agents yet. Create a token and run rd-agent on the PC.</div></div>`;
    return;
  }
  for (const agent of state.agents) {
    const item = document.createElement("div");
    item.className = "session-item";
    const online = agent.status === "online" || agent.status === "busy";
    item.innerHTML = `
      <div class="session-row">
        <strong>${agent.name || agent.hostname || "PC"} · ${agent.device_code}</strong>
        <span class="pill">${agent.status}</span>
      </div>
      <div class="muted">${agent.hostname || "waiting for agent"} · ${agent.os || ""}</div>
      <div class="status-actions" style="margin-top:10px">
        <button class="primary connect-agent" type="button" ${online && !requiredPassword() ? "" : "disabled"}>Connect</button>
        <button class="danger delete-agent" type="button">Remove</button>
      </div>
    `;
    item.querySelector(".connect-agent").addEventListener("click", () => connectAgent(agent.id));
    item.querySelector(".delete-agent").addEventListener("click", () => deleteAgent(agent.id));
    els.agentList.appendChild(item);
  }
}

function requiredPassword() {
  return Boolean(state.user?.password_change_required);
}

function renderSessions() {
  els.sessionList.innerHTML = "";
  if (!state.sessions.length) {
    els.sessionList.innerHTML = `<div class="session-item"><div class="muted">No container/agent sessions yet.</div></div>`;
    return;
  }

  for (const session of state.sessions) {
    const item = document.createElement("button");
    item.type = "button";
    item.className = `session-item ${state.activeSession?.id === session.id ? "active" : ""}`;
    item.innerHTML = `
      <div class="session-row">
        <strong>#${session.id} · ${session.type}</strong>
        <span class="pill">${session.status}</span>
      </div>
      <div class="muted">${session.resolution} · expires ${new Date(session.expires_at).toLocaleString()}</div>
    `;
    item.addEventListener("click", () => selectSession(session));
    els.sessionList.appendChild(item);
  }
}

async function refreshBootstrap() {
  const data = await api("/api/bootstrap");
  state.user = data.user;
  state.sessions = data.sessions || [];
  state.agents = await api("/api/agents").catch(() => []);
  if (state.activeSession) {
    state.activeSession = state.sessions.find(s => s.id === state.activeSession.id) || null;
  }
  renderAuth();
  renderAgents();
  renderSessions();
  await refreshRuntime();
  if (state.user?.password_change_required) {
    clearViewer("Change the default password to unlock controls.");
    return;
  }
  if (state.activeSession) {
    await openViewer(state.activeSession.id, false);
  } else {
    clearViewer("Connect to an online agent to control a real machine.");
  }
}

async function refreshRuntime() {
  if (!els.runtimeMeta) return;
  const online = state.agents.filter(a => a.status === "online" || a.status === "busy").length;
  els.runtimeMeta.textContent = `${online} agent(s) online. Install rd-agent on each PC you want to control.`;
}

function clearViewer(message) {
  state.activeSession = null;
  els.sessionMeta.textContent = message;
  els.viewerFrame.src = "about:blank";
  els.viewerFrame.classList.add("hidden");
  els.viewerEmpty.textContent = message;
  els.viewerEmpty.classList.remove("hidden");
  els.openViewerBtn.classList.add("hidden");
  els.stopSessionBtn.classList.add("hidden");
  renderSessions();
}

async function selectSession(session) {
  state.activeSession = session;
  renderSessions();
  await openViewer(session.id, true);
}

async function openViewer(sessionId, refresh = true) {
  const data = await api(`/api/sessions/${sessionId}/viewer`);
  const session = state.sessions.find(s => s.id === Number(sessionId)) || state.activeSession;
  state.activeSession = session;
  els.sessionMeta.textContent = `Session #${sessionId} · ${session?.type || "desktop"} · ${session?.resolution || ""}`;
  els.viewerFrame.src = data.viewer_url;
  els.viewerFrame.classList.remove("hidden");
  els.viewerEmpty.classList.add("hidden");
  els.openViewerBtn.classList.remove("hidden");
  els.stopSessionBtn.classList.remove("hidden");
  if (refresh) {
    renderSessions();
  }
}

async function connectAgent(agentId) {
  const data = await api(`/api/agents/${agentId}/connect`, { method: "POST" });
  state.activeSession = data.session;
  els.sessionMeta.textContent = `Agent session #${data.session.id}`;
  els.viewerFrame.src = data.viewer_url;
  els.viewerFrame.classList.remove("hidden");
  els.viewerEmpty.classList.add("hidden");
  els.openViewerBtn.classList.remove("hidden");
  els.stopSessionBtn.classList.remove("hidden");
  await refreshBootstrap();
  state.activeSession = data.session;
  els.viewerFrame.src = data.viewer_url;
}

async function deleteAgent(agentId) {
  if (!confirm("Remove this agent enrollment?")) return;
  await api(`/api/agents/${agentId}`, { method: "DELETE" });
  await refreshBootstrap();
}

async function login(username, password) {
  const data = await api("/api/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
  setAuth(data.token);
  await refreshBootstrap();
}

function logout() {
  fetch("/api/logout", { method: "POST" }).catch(() => {});
  setAuth("");
  state.user = null;
  state.sessions = [];
  state.agents = [];
  state.activeSession = null;
  renderAuth();
  clearViewer("Signed out.");
}

els.loginForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  els.loginError.classList.add("hidden");
  try {
    await login(els.username.value.trim(), els.password.value);
    els.password.value = "";
  } catch (error) {
    els.loginError.textContent = error.message;
    els.loginError.classList.remove("hidden");
  }
});

els.passwordForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  els.passwordError.classList.add("hidden");
  const submitter = els.passwordForm.querySelector("button[type=submit]");
  submitter.disabled = true;
  try {
    const user = await api("/api/change-password", {
      method: "POST",
      body: JSON.stringify({
        current_password: els.currentPassword.value,
        new_password: els.newPassword.value,
      }),
    });
    state.user = user;
    els.currentPassword.value = "";
    els.newPassword.value = "";
    renderAuth();
    await refreshBootstrap();
  } catch (error) {
    els.passwordError.textContent = error.message;
    els.passwordError.classList.remove("hidden");
  } finally {
    submitter.disabled = false;
  }
});

els.agentForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const submitter = els.agentForm.querySelector("button[type=submit]");
  submitter.disabled = true;
  try {
    const data = await api("/api/agents", {
      method: "POST",
      body: JSON.stringify({ name: els.agentName.value.trim() }),
    });
    els.agentInstallHint.textContent =
      `Device code: ${data.agent.device_code}\n\n` +
      `On the target PC run:\n${data.install_hint}\n\n` +
      `Save the token now — it is shown only once.`;
    els.agentInstallHint.classList.remove("hidden");
    els.agentName.value = "";
    await refreshBootstrap();
  } catch (error) {
    alert(error.message);
  } finally {
    submitter.disabled = false;
  }
});

els.sessionType.addEventListener("change", () => {
  const isRelay = els.sessionType.value === "relay";
  els.relayHostField.classList.toggle("hidden", !isRelay);
  if (!isRelay) {
    els.targetHost.value = "";
  }
});

els.createForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const payload = {
    type: els.sessionType.value,
    resolution: els.resolution.value,
    audio_enabled: true,
    clipboard_sync: true,
    target_host: els.targetHost.value.trim(),
    target_port: 3389,
  };

  const submitter = els.createForm.querySelector("button[type=submit]");
  submitter.disabled = true;
  try {
    const data = await api("/api/sessions", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    await refreshBootstrap();
    await openViewer(data.session.id, true);
  } catch (error) {
    alert(error.message);
  } finally {
    submitter.disabled = false;
  }
});

els.refreshBtn.addEventListener("click", () => refreshBootstrap().catch((error) => alert(error.message)));
els.logoutBtn.addEventListener("click", logout);
els.openViewerBtn.addEventListener("click", () => {
  if (state.activeSession) {
    openViewer(state.activeSession.id, false).catch((error) => alert(error.message));
  }
});
els.stopSessionBtn.addEventListener("click", async () => {
  if (!state.activeSession) return;
  if (!confirm(`Stop session #${state.activeSession.id}?`)) return;
  try {
    await api(`/api/sessions/${state.activeSession.id}`, { method: "DELETE" });
    await refreshBootstrap();
  } catch (error) {
    alert(error.message);
  }
});

renderAuth();
if (state.token) {
  setAuth(state.token);
  refreshBootstrap().catch(() => logout());
}
