const state = {
  page: 1,
  limit: 20,
  level: "",
  type: "",
  search: "",
  status: "",
  ip: "",
  url: "",
  method: "",
  server: "",
  start_date: "",
  end_date: "",
  live: true,
  socket: null,
  serverName: "",
  availableLevels: [],
  availableTypes: [],
};

const rows = document.getElementById("logRows");
const pageInfo = document.getElementById("pageInfo");
const resultSummary = document.getElementById("resultSummary");

async function fetchMeta() {
  const res = await fetch("/api/meta");
  const data = await res.json();
  state.serverName = data.server || "unknown";
  document.getElementById("serverName").textContent = state.serverName;
  applyFilterMetadata(data.filters || {});
}

async function fetchStats() {
  const res = await fetch("/api/stats");
  const data = await res.json();
  document.getElementById("statTotal").textContent = data.total;
  document.getElementById("statRequests").textContent = data.requests;
  document.getElementById("statWarnings").textContent = data.warnings;
  document.getElementById("statErrors").textContent = data.errors;
}

async function fetchLogs() {
  const params = new URLSearchParams({
    page: state.page,
    limit: state.limit,
    level: state.level,
    type: state.type,
    search: state.search,
    status: state.status,
    ip: state.ip,
    url: state.url,
    method: state.method,
    server: state.server,
    start_date: state.start_date,
    end_date: state.end_date,
  });
  const res = await fetch(`/api/logs?${params.toString()}`);
  const data = await res.json();
  renderLogs(data.items || []);
  const totalPages = Math.max(1, Math.ceil((data.total || 0) / state.limit));
  pageInfo.textContent = `Page ${state.page} / ${totalPages}`;
  resultSummary.textContent = `${data.total || 0} matching logs`;
}

function renderLogs(items) {
  if (!items.length) {
    rows.innerHTML = `<tr><td colspan="5" class="empty-state">No logs match the current filters.</td></tr>`;
    return;
  }
  rows.innerHTML = items.map((item) => `
    <tr>
      <td><span class="level ${item.level}">${item.level}</span></td>
      <td class="time-cell">${new Date(item.timestamp).toLocaleString()}</td>
      <td class="request-cell">
        <div class="request-main">
          ${item.method ? `<span class="chip">${escapeHTML(item.method)}</span>` : ""}
          ${renderStatus(item.status_code)}
          ${item.latency_ms ? `<span class="chip">${item.latency_ms} ms</span>` : ""}
        </div>
        <div class="request-route">${escapeHTML(requestText(item))}</div>
      </td>
      <td class="detail-cell">
        <div class="message-title">${escapeHTML(primaryMessage(item))}</div>
        <div class="meta-chips">
          ${item.ip ? `<span class="chip">IP ${escapeHTML(item.ip)}</span>` : ""}
          ${item.server ? `<span class="chip">Server ${escapeHTML(item.server)}</span>` : ""}
          ${item.type ? `<span class="chip">Type ${escapeHTML(item.type)}</span>` : ""}
          ${item.id ? `<span class="chip">ID ${item.id}</span>` : ""}
        </div>
        ${secondaryMessage(item) ? `<div class="raw-text">${escapeHTML(secondaryMessage(item))}</div>` : ""}
      </td>
      <td>
        <div class="source-stack">
          <span class="source-file">${escapeHTML(item.source || "")}</span>
          ${item.server ? `<span class="chip">Server ${escapeHTML(item.server)}</span>` : ""}
        </div>
      </td>
    </tr>
  `).join("");
}

function prependLog(item) {
  const first = document.createElement("tr");
  first.innerHTML = `
    <td><span class="level ${item.level}">${item.level}</span></td>
    <td class="time-cell">${new Date(item.timestamp).toLocaleString()}</td>
    <td class="request-cell">
      <div class="request-main">
        ${item.method ? `<span class="chip">${escapeHTML(item.method)}</span>` : ""}
        ${renderStatus(item.status_code)}
        ${item.latency_ms ? `<span class="chip">${item.latency_ms} ms</span>` : ""}
      </div>
      <div class="request-route">${escapeHTML(requestText(item))}</div>
    </td>
    <td class="detail-cell">
      <div class="message-title">${escapeHTML(primaryMessage(item))}</div>
      <div class="meta-chips">
        ${item.ip ? `<span class="chip">IP ${escapeHTML(item.ip)}</span>` : ""}
        ${item.server ? `<span class="chip">Server ${escapeHTML(item.server)}</span>` : ""}
        ${item.type ? `<span class="chip">Type ${escapeHTML(item.type)}</span>` : ""}
        ${item.id ? `<span class="chip">ID ${item.id}</span>` : ""}
      </div>
      ${secondaryMessage(item) ? `<div class="raw-text">${escapeHTML(secondaryMessage(item))}</div>` : ""}
    </td>
    <td>
      <div class="source-stack">
        <span class="source-file">${escapeHTML(item.source || "")}</span>
        ${item.server ? `<span class="chip">Server ${escapeHTML(item.server)}</span>` : ""}
      </div>
    </td>
  `;
  if (rows.querySelector(".empty-state")) {
    rows.innerHTML = "";
  }
  rows.prepend(first);
  while (rows.children.length > state.limit) {
    rows.removeChild(rows.lastChild);
  }
}

function connectLive() {
  if (!state.live) {
    if (state.socket) state.socket.close();
    state.socket = null;
    return;
  }
  const protocol = location.protocol === "https:" ? "wss" : "ws";
  const socket = new WebSocket(`${protocol}://${location.host}/api/logs/live`);
  socket.onmessage = (event) => {
    if (state.page !== 1) return;
    const item = JSON.parse(event.data);
    prependLog(item);
    fetchStats();
  };
  socket.onclose = () => {
    if (state.live) setTimeout(connectLive, 1500);
  };
  state.socket = socket;
}

function bindFilters() {
  bindPillGroup("levelFilters", "data-level", "level");
  bindPillGroup("typeFilters", "data-type", "type");
}

function wireInputs() {
  const bindings = [
    ["search", "search"],
    ["statusInput", "status"],
    ["ipInput", "ip"],
    ["urlInput", "url"],
    ["methodInput", "method"],
    ["serverInput", "server"],
    ["startDate", "start_date"],
    ["endDate", "end_date"],
  ];
  bindings.forEach(([id, key]) => {
    document.getElementById(id).addEventListener(id.endsWith("Input") && (id === "methodInput" || id === "serverInput") ? "change" : "input", (e) => {
      state[key] = e.target.value.trim();
      state.page = 1;
      fetchLogs();
    });
  });
  document.getElementById("rowsPerPage").addEventListener("change", (e) => {
    state.limit = Number(e.target.value);
    state.page = 1;
    fetchLogs();
  });
  document.getElementById("prevBtn").addEventListener("click", () => {
    state.page = Math.max(1, state.page - 1);
    fetchLogs();
  });
  document.getElementById("nextBtn").addEventListener("click", () => {
    state.page += 1;
    fetchLogs();
  });
  document.getElementById("liveToggle").addEventListener("change", (e) => {
    state.live = e.target.checked;
    connectLive();
  });
  document.getElementById("themeToggle").addEventListener("change", (e) => {
    document.body.classList.toggle("dark", e.target.checked);
  });
  document.getElementById("exportBtn").addEventListener("click", () => {
    const params = new URLSearchParams({
      format: "json",
      level: state.level,
      type: state.type,
      search: state.search,
      status: state.status,
      ip: state.ip,
      url: state.url,
      method: state.method,
      server: state.server,
      start_date: state.start_date,
      end_date: state.end_date,
    });
    window.location.href = `/api/logs/export?${params.toString()}`;
  });
  document.getElementById("clearBtn").addEventListener("click", async () => {
    await fetch("/api/logs", { method: "DELETE" });
    await fetchStats();
    await fetchLogs();
  });
}

function bindPillGroup(containerId, dataKey, stateKey) {
  const container = document.getElementById(containerId);
  container.addEventListener("click", (event) => {
    const button = event.target.closest("button");
    if (!button) return;
    container.querySelectorAll("button").forEach((b) => b.classList.remove("active"));
    button.classList.add("active");
    state[stateKey] = button.getAttribute(dataKey) || "";
    state.page = 1;
    fetchLogs();
  });
}

function applyFilterMetadata(filters) {
  renderPills("levelFilters", "data-level", filters.levels && filters.levels.length ? filters.levels : ["LOG", "INFO", "WARN", "ERROR"]);
  renderPills("typeFilters", "data-type", filters.types && filters.types.length ? filters.types : ["ACCESS", "ERROR", "CONSOLE"]);
  populateSelect("methodInput", filters.methods || [], "All methods");
  populateSelect("serverInput", filters.servers || [], "All servers");
  toggleGroup("statusFilterGroup", !!filters.has_data?.status);
  toggleGroup("ipFilterGroup", !!filters.has_data?.ip);
  toggleGroup("urlFilterGroup", !!filters.has_data?.url);
  toggleGroup("methodFilterGroup", !!filters.has_data?.method);
  toggleGroup("serverFilterGroup", !!filters.has_data?.server && (filters.servers || []).length > 1);
  toggleGroup("dateFilterGroup", !!filters.has_data?.date);
}

function renderPills(containerId, dataAttr, values) {
  const container = document.getElementById(containerId);
  const key = dataAttr === "data-level" ? "level" : "type";
  const current = state[key];
  const buttons = [`<button ${dataAttr}="" class="pill ${current === "" ? "active" : ""}">ALL</button>`];
  values.forEach((value) => {
    buttons.push(`<button ${dataAttr}="${escapeHTML(value)}" class="pill ${current === value ? "active" : ""}">${escapeHTML(value)}</button>`);
  });
  container.innerHTML = buttons.join("");
}

function populateSelect(id, values, placeholder) {
  const select = document.getElementById(id);
  const selected = state[id === "methodInput" ? "method" : "server"];
  select.innerHTML = [`<option value="">${placeholder}</option>`]
    .concat(values.map((value) => `<option value="${escapeHTML(value)}" ${selected === value ? "selected" : ""}>${escapeHTML(value)}</option>`))
    .join("");
}

function toggleGroup(id, visible) {
  document.getElementById(id).hidden = !visible;
}

function renderStatus(status) {
  if (!status) return "";
  const tone = status >= 500 ? "error" : status >= 400 ? "warn" : "ok";
  return `<span class="chip status-chip ${tone}">${status}</span>`;
}

function requestText(item) {
  return item.url || item.route || extractRequestFromRaw(item.raw) || "-";
}

function primaryMessage(item) {
  if (item.type === "ACCESS" && item.method) {
    return `${item.method} ${requestText(item)}`.trim();
  }
  return item.message || item.raw || "";
}

function secondaryMessage(item) {
  if (!item.raw) return "";
  if (item.type === "ACCESS") return "";
  return item.raw !== item.message ? item.raw : "";
}

function extractRequestFromRaw(raw) {
  const match = String(raw || "").match(/"([A-Z]+)\s+([^"\s]+)[^"]*"\s+\d{3}/);
  return match ? match[2] : "";
}


function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

async function initialize() {
  await fetchMeta();
  bindFilters();
  wireInputs();
  await fetchStats();
  await fetchLogs();
  connectLive();
}

initialize();
