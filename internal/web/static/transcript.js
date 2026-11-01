const FENCE_OPEN = /^ {0,3}```[^`]*$/;
const FENCE_CLOSE = /^ {0,3}```\s*$/;
const MAYBE_FENCE = /^ {0,3}`{0,3}$|^ {0,3}```/;
const SUMMARY_KEYS = ["command", "path", "file_path", "pattern", "query", "url"];
const RESULT_CAP_NOTE = "Result cut to the first 30 KB.";

function make(tag, className, text) {
  const el = document.createElement(tag);
  if (className) el.className = className;
  if (text !== undefined) el.textContent = text;
  return el;
}

function note(className, ...parts) {
  const el = make("p", `note ${className}`.trim());
  for (const part of parts) {
    if (Array.isArray(part)) el.append(make("span", "t-num", part[0]));
    else el.append(document.createTextNode(part));
  }
  return el;
}

function seconds(ms) {
  return `${(ms / 1000).toFixed(ms < 10000 ? 1 : 0)} s`;
}

function clock(iso) {
  return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function codeRegion(text, label, className) {
  const pre = make("pre", className, text);
  pre.tabIndex = 0;
  pre.setAttribute("role", "region");
  pre.setAttribute("aria-label", label);
  return pre;
}

function argsSummary(args) {
  if (!args) return "";
  try {
    const value = JSON.parse(args);
    if (value && typeof value === "object") {
      const key = SUMMARY_KEYS.find((k) => typeof value[k] === "string");
      if (key) return value[key];
      const first = Object.values(value).find((v) => typeof v === "string");
      if (first !== undefined) return first;
    }
  } catch {
    return args;
  }
  return args;
}

function prettyArgs(args) {
  try {
    return JSON.stringify(JSON.parse(args), null, 2);
  } catch {
    return args;
  }
}

function lineCount(text) {
  const n = text === "" ? 0 : text.split("\n").length;
  return n === 1 ? "1 line" : `${n} lines`;
}

function turnStats(ev) {
  const parts = [];
  if (ev.input_tokens || ev.output_tokens) {
    parts.push(`${ev.input_tokens || 0} in`, `${ev.output_tokens || 0} out`);
  }
  if (ev.ttft_ms) parts.push(`ttft ${ev.ttft_ms} ms`);
  if (ev.steps) parts.push(ev.steps === 1 ? "1 step" : `${ev.steps} steps`);
  if (ev.jev) parts.push(`jev ${ev.jev.requests} req`);
  if (parts.length === 0 && ev.reason) parts.push(ev.reason);
  return parts.join(" · ");
}

class TextBlock {
  constructor() {
    this.el = make("div", "answer");
    this.node = null;
    this.code = false;
    this.held = "";
    this.midLine = false;
  }

  append(text) {
    if (!this.node) this.start(this.code);
    this.node.appendData(text);
  }

  start(code) {
    this.node = document.createTextNode("");
    if (code) {
      const pre = codeRegion("", "Code block", "code");
      pre.append(this.node);
      this.el.append(pre);
    } else {
      const prose = make("div", "prose");
      prose.append(this.node);
      this.el.append(prose);
    }
  }

  trimTail() {
    if (!this.node) return;
    if (this.node.data.endsWith("\n")) this.node.deleteData(this.node.length - 1, 1);
    if (this.node.length === 0) this.node.parentNode.remove();
  }

  toggleFence() {
    this.trimTail();
    this.code = !this.code;
    this.node = null;
    if (this.code) this.start(true);
  }

  line(text) {
    const bare = text.endsWith("\n") ? text.slice(0, -1) : text;
    const isFence = !this.midLine && (this.code ? FENCE_CLOSE : FENCE_OPEN).test(bare);
    if (isFence) this.toggleFence();
    else this.append(text);
    this.midLine = false;
  }

  feed(chunk) {
    let rest = this.held + chunk;
    this.held = "";
    for (let cut = rest.indexOf("\n"); cut >= 0; cut = rest.indexOf("\n")) {
      this.line(rest.slice(0, cut + 1));
      rest = rest.slice(cut + 1);
    }
    if (rest === "") return;
    if (!this.midLine && MAYBE_FENCE.test(rest)) {
      this.held = rest;
      return;
    }
    this.append(rest);
    this.midLine = true;
  }

  finish() {
    if (this.held !== "") this.line(this.held);
    this.held = "";
    this.trimTail();
  }
}

class ToolRow {
  constructor(ev) {
    this.el = make("div", "tool");
    this.el.dataset.state = "running";
    this.head = make("button", "tool-head");
    this.head.type = "button";
    this.head.setAttribute("aria-expanded", "false");
    this.mark = make("span", "tool-mark");
    this.mark.setAttribute("aria-hidden", "true");
    this.summary = make("span", "tool-summary");
    this.detail = make("span", "tool-detail", "Running");
    this.head.append(this.mark, make("span", "tool-name", ev.name || "tool"), this.summary, this.detail);
    this.body = make("div", "tool-body");
    this.body.hidden = true;
    this.head.addEventListener("click", () => this.toggle());
    this.el.append(this.head, this.body);
    this.describe(ev);
  }

  describe(ev) {
    if (!ev.args) return;
    this.summary.textContent = argsSummary(ev.args).split("\n")[0];
    this.body.prepend(codeRegion(prettyArgs(ev.args), "Tool arguments", "tool-text"));
  }

  toggle() {
    const open = this.head.getAttribute("aria-expanded") !== "true";
    this.head.setAttribute("aria-expanded", String(open));
    this.body.hidden = !open;
  }

  result(ev) {
    this.el.dataset.state = ev.is_error ? "error" : "done";
    this.detail.textContent = ev.is_error ? "Failed" : lineCount(ev.text || "");
    this.body.append(codeRegion(ev.text || "", "Tool result", "tool-text"));
    if (ev.truncated) this.body.append(note("", RESULT_CAP_NOTE));
  }

  abandon() {
    if (this.el.dataset.state !== "running") return;
    this.el.dataset.state = "stopped";
    this.detail.textContent = "No result";
  }
}

export class Transcript {
  constructor(root, hooks) {
    this.root = root;
    this.hooks = hooks;
    this.working = make("p", "working");
    this.working.append(make("span", "spinner"), document.createTextNode("Working"));
    this.frame = 0;
    this.reset();
  }

  reset() {
    if (this.frame) cancelAnimationFrame(this.frame);
    this.frame = 0;
    this.root.replaceChildren();
    this.lastId = 0;
    this.pending = "";
    this.text = null;
    this.tools = new Map();
  }

  add(el) {
    this.root.insertBefore(el, this.working.isConnected ? this.working : null);
    this.schedule();
  }

  schedule() {
    if (!this.frame) this.frame = requestAnimationFrame(() => this.paint());
  }

  paint() {
    this.frame = 0;
    this.flush();
    this.hooks.onPaint();
  }

  flush() {
    if (this.pending === "") return;
    if (!this.text) {
      this.text = new TextBlock();
      this.add(this.text.el);
    }
    this.text.feed(this.pending);
    this.pending = "";
  }

  closeText() {
    this.flush();
    if (this.text) this.text.finish();
    this.text = null;
  }

  discard() {
    this.pending = "";
    if (this.text) this.text.el.remove();
    this.text = null;
  }

  setWorking(on) {
    if (on) this.root.append(this.working);
    else this.working.remove();
    this.schedule();
  }

  apply(ev) {
    if (ev.id <= this.lastId) return false;
    this.lastId = ev.id;
    if (ev.kind === "text") {
      this.pending += ev.text || "";
      this.schedule();
      return true;
    }
    if (ev.discard) this.discard();
    this.closeText();
    const render = this[`on_${ev.kind}`];
    if (render) render.call(this, ev);
    return true;
  }

  on_user(ev) {
    this.add(make("div", "user", ev.text));
  }

  toolRow(ev) {
    let row = this.tools.get(ev.call_id);
    if (!row) {
      row = new ToolRow(ev);
      this.tools.set(ev.call_id, row);
      this.add(row.el);
    }
    return row;
  }

  on_tool_start(ev) {
    this.toolRow(ev);
  }

  on_tool_result(ev) {
    this.toolRow(ev).result(ev);
    this.schedule();
  }

  on_turn_end(ev) {
    const stats = turnStats(ev);
    if (stats) this.add(make("p", "turn-meta t-meta", stats));
  }

  on_wait(ev) {
    const until = ev.until ? clock(ev.until) : "";
    this.add(note("", "Usage limit on ", [ev.model || "this model"], ", waiting until ", [until]));
  }

  on_switch(ev) {
    this.add(note("", "Switched from ", [ev.from], " to ", [ev.to]));
  }

  on_retry(ev) {
    const attempt = `${ev.attempt} of ${ev.max}`;
    this.add(note("", "Stream failed, retrying in ", [seconds(ev.delay_ms)], ", attempt ", [attempt]));
  }

  on_error(ev) {
    this.add(note("note-error", ev.text || "The turn failed."));
  }

  on_run_status(ev) {
    const running = ev.status === "running";
    if (!running) for (const row of this.tools.values()) row.abandon();
    if (ev.status === "interrupted") this.add(note("", "Stopped. Send a message to continue."));
    this.setWorking(running);
    this.hooks.onStatus(ev.status);
  }
}
