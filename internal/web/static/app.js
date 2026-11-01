import { Rail } from "/rail.js";
import { Transcript } from "/transcript.js";

const RUN_ID = /^[0-9]{8}-[0-9]{6}-[0-9a-f]{4}$/;
const FOLLOW_SLACK = 80;
const RAIL_POLL_MS = 5000;
const RECOVER_MS = 2000;
const NARROW = window.matchMedia("(max-width: 639px)");

const MSG_RESTARTED = "The server restarted. Open the new link it printed and this page reconnects.";
const MSG_OFFLINE = "Could not reach the server. Your text is still here.";
const MSG_BUSY = "This run is busy. Wait for the turn to end or stop it, then send again.";
const MSG_GONE = "This run no longer exists. Start a new run.";

const $ = (id) => document.getElementById(id);
const app = $("app");
const thread = $("thread");
const prompt = $("prompt");
const model = $("model");
const send = $("send");
const dock = $("dock");
const rail = new Rail($("runs"), $("runs-empty"), $("hover-mark"));
const transcript = new Transcript($("transcript"), {
  onPaint: followTail,
  onStatus: statusChanged,
});

let currentId = null;
let source = null;
let generation = 0;
let running = false;
let dropped = false;
let resumed = false;
let following = true;
let sending = false;
let defaultModel = "";

async function call(path, body) {
  const init = body === undefined ? undefined : {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  };
  try {
    const res = await fetch(path, init);
    return { ok: res.ok, status: res.status, data: await res.json().catch(() => ({})) };
  } catch {
    return { ok: false, status: 0, data: {} };
  }
}

function failure(res) {
  if (res.status === 0) return MSG_OFFLINE;
  if (res.status === 401) return MSG_RESTARTED;
  if (res.status === 409) return MSG_BUSY;
  const reason = res.data.error || `Request failed with status ${res.status}`;
  return `${reason}. Your text is still here.`;
}

function showNotice(text) {
  const notice = $("notice");
  notice.textContent = text;
  notice.hidden = text === "";
  if (text) announce(text);
}

function announce(text) {
  $("live").textContent = text;
}

function showConn(on) {
  const conn = $("conn");
  if (conn.hidden === !on) return;
  conn.hidden = !on;
  announce(on ? "Reconnecting" : "Reconnected");
}

function atBottom() {
  return thread.scrollHeight - thread.scrollTop - thread.clientHeight <= FOLLOW_SLACK;
}

function setFollowing(on) {
  following = on;
  $("jump").hidden = on;
}

function followTail() {
  if (following) thread.scrollTop = thread.scrollHeight;
  else $("jump").hidden = false;
}

function setEmpty(empty) {
  const was = app.dataset.empty === "true";
  if (was === empty) return;
  const before = dock.getBoundingClientRect().top;
  app.dataset.empty = String(empty);
  const moved = before - dock.getBoundingClientRect().top;
  if (empty || !moved) return;
  dock.style.transition = "none";
  dock.style.translate = `0 ${moved}px`;
  dock.getBoundingClientRect();
  dock.style.transition = "";
  dock.style.translate = "";
}

function syncSend() {
  const stopping = running;
  send.dataset.mode = stopping ? "stop" : "send";
  send.setAttribute("aria-label", stopping ? "Stop turn" : "Send message");
  send.disabled = !stopping && (sending || prompt.value.trim() === "");
}

function setRunning(on) {
  running = on;
  syncSend();
  transcript.setWorking(on);
  model.disabled = on;
}

function statusChanged(status) {
  const on = status === "running";
  if (on !== running) announce(on ? "Turn running" : status === "finished" ? "Turn finished" : "Turn stopped");
  running = on;
  syncSend();
  model.disabled = on;
  refreshRuns();
}

function closeSource() {
  generation++;
  if (source) source.close();
  source = null;
  dropped = false;
  resumed = false;
}

function connect(id) {
  closeSource();
  const gen = generation;
  const es = new EventSource(`/api/runs/${id}/events`);
  source = es;
  es.onmessage = (message) => {
    if (gen === generation) receive(JSON.parse(message.data));
  };
  es.onopen = () => {
    if (gen === generation && dropped) reopened();
  };
  es.onerror = () => {
    if (gen !== generation) return;
    dropped = true;
    showConn(true);
    if (es.readyState === EventSource.CLOSED) recover(gen);
  };
}

function receive(ev) {
  const fresh = transcript.apply(ev);
  if (!fresh && resumed) rebuild();
}

function rebuild() {
  transcript.reset();
  connect(currentId);
}

async function reopened() {
  dropped = false;
  resumed = true;
  showConn(false);
  showNotice("");
  await refreshRuns();
  const known = rail.status(currentId);
  if (known && (known === "running") !== running) rebuild();
}

async function probe(id) {
  const stop = new AbortController();
  try {
    const res = await fetch(`/api/runs/${id}/events`, { signal: stop.signal });
    stop.abort();
    return res.status;
  } catch {
    return 0;
  }
}

async function recover(gen) {
  if (gen !== generation) return;
  const status = await probe(currentId);
  if (gen !== generation) return;
  if (status === 200) {
    showConn(false);
    showNotice("");
    rebuild();
    return;
  }
  if (status === 404) {
    showConn(false);
    showNotice(MSG_GONE);
    return;
  }
  if (status === 401) showNotice(MSG_RESTARTED);
  setTimeout(() => recover(gen), RECOVER_MS);
}

function openRun(id) {
  if (id === currentId) return;
  currentId = id;
  closeSource();
  transcript.reset();
  showConn(false);
  showNotice("");
  setFollowing(true);
  setRunning(false);
  setEmpty(id === null);
  rail.select(id);
  if (id) connect(id);
  else model.value = defaultModel;
  syncModel();
  prompt.focus();
}

function syncModel() {
  const run = rail.runs.find((r) => r.id === currentId);
  if (run && document.activeElement !== model) model.value = run.model;
}

function hashRun() {
  const id = location.hash.slice(1);
  return RUN_ID.test(id) ? id : null;
}

function goHome() {
  history.pushState(null, "", location.pathname);
  openRun(null);
  closeRail();
}

async function refreshRuns() {
  const res = await call("/api/runs");
  if (!res.ok) return;
  rail.render(res.data, currentId);
  fillModels();
  syncModel();
}

function fillModels() {
  const specs = new Set(defaultModel ? [defaultModel] : []);
  for (const run of rail.runs) specs.add(run.model);
  for (const preset of presets) specs.add(`${preset}/`);
  $("models").replaceChildren(...[...specs].map((spec) => {
    const option = document.createElement("option");
    option.value = spec;
    return option;
  }));
}

let presets = [];

async function loadState() {
  const res = await call("/api/state");
  if (!res.ok) {
    if (res.status === 401) showNotice(MSG_RESTARTED);
    return;
  }
  defaultModel = res.data.model || "";
  presets = res.data.presets || [];
  $("dir").textContent = res.data.dir || "";
  $("dir").title = res.data.dir || "";
  if (!currentId) model.value = defaultModel;
  fillModels();
}

function chosenModel() {
  const spec = model.value.trim();
  return spec === defaultModel ? "" : spec;
}

async function submit() {
  const text = prompt.value;
  if (text.trim() === "" || sending) return;
  sending = true;
  syncSend();
  showNotice("");
  const res = currentId
    ? await call(`/api/runs/${currentId}/messages`, { prompt: text })
    : await call("/api/runs", { prompt: text, model: chosenModel() });
  sending = false;
  syncSend();
  if (!res.ok) {
    showNotice(failure(res));
    return;
  }
  prompt.value = "";
  fitPrompt();
  syncSend();
  setFollowing(true);
  if (!currentId) {
    openRun(res.data.id);
    location.hash = res.data.id;
    setRunning(true);
    refreshRuns();
  }
}

async function stop() {
  if (!currentId) return;
  const res = await call(`/api/runs/${currentId}/stop`, {});
  if (!res.ok) showNotice(failure(res));
}

async function changeModel() {
  const spec = model.value.trim();
  if (!currentId || !spec) return;
  const run = rail.runs.find((r) => r.id === currentId);
  if (run && run.model === spec) return;
  const res = await call(`/api/runs/${currentId}/model`, { model: spec });
  if (!res.ok) {
    showNotice(failure(res));
    if (run) model.value = run.model;
    return;
  }
  showNotice("");
  model.value = res.data.model;
  refreshRuns();
}

function fitPrompt() {
  prompt.style.height = "auto";
  prompt.style.height = `${prompt.scrollHeight}px`;
}

function openRail() {
  app.dataset.rail = "open";
  $("menu").setAttribute("aria-expanded", "true");
  $("main").inert = true;
  $("new-run").focus();
}

function closeRail() {
  if (app.dataset.rail !== "open") return false;
  app.dataset.rail = "closed";
  $("menu").setAttribute("aria-expanded", "false");
  $("main").inert = false;
  return true;
}

function onPromptKey(event) {
  if (event.key !== "Enter" || event.shiftKey || event.isComposing || event.keyCode === 229) return;
  event.preventDefault();
  submit();
}

function onModelKey(event) {
  if (event.key !== "Enter") return;
  event.preventDefault();
  prompt.focus();
}

function onEscape(event) {
  if (event.key !== "Escape") return;
  if (closeRail()) {
    $("menu").focus();
    return;
  }
  if (running) stop();
}

function wire() {
  $("composer").addEventListener("submit", (event) => {
    event.preventDefault();
    if (running) stop();
    else submit();
  });
  prompt.addEventListener("keydown", onPromptKey);
  prompt.addEventListener("input", () => {
    fitPrompt();
    syncSend();
  });
  model.addEventListener("change", changeModel);
  model.addEventListener("keydown", onModelKey);
  thread.addEventListener("scroll", () => setFollowing(atBottom()), { passive: true });
  $("jump").addEventListener("click", () => {
    setFollowing(true);
    thread.scrollTop = thread.scrollHeight;
    prompt.focus();
  });
  $("new-run").addEventListener("click", goHome);
  $("new-run-bar").addEventListener("click", goHome);
  $("menu").addEventListener("click", openRail);
  $("scrim").addEventListener("click", closeRail);
  $("runs").addEventListener("click", closeRail);
  NARROW.addEventListener("change", closeRail);
  document.addEventListener("keydown", onEscape);
  window.addEventListener("hashchange", () => openRun(hashRun()));
  window.addEventListener("popstate", () => openRun(hashRun()));
  setInterval(() => {
    if (!document.hidden) refreshRuns();
  }, RAIL_POLL_MS);
}

async function init() {
  wire();
  app.dataset.empty = String(hashRun() === null);
  await loadState();
  await refreshRuns();
  const id = hashRun();
  if (id) openRun(id);
  else {
    openRun(null);
    prompt.focus();
  }
}

init();
