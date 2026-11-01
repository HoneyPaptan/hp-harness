const FENCE_OPEN = /^ {0,3}```[^`]*$/;
const FENCE_CLOSE = /^ {0,3}```\s*$/;
const MAYBE_FENCE = /^ {0,3}`{0,3}$|^ {0,3}```/;
const MAYBE_BLOCK = /^ *(`{0,3}|#{1,6}[ \t]*|[-*+][ \t]*|\d{1,9}([.)][ \t]*)?|>[ \t]?)$|^ {0,3}```/;
const HEADING = /^ {0,3}(#{1,6})[ \t]+/;
const BULLET = /^ *[-*+][ \t]+/;
const ORDERED = /^ *(\d{1,9})[.)][ \t]+/;
const QUOTE = /^ {0,3}> ?/;
const MARKER = /[*_`]/;
const WORD = /[\p{L}\p{N}]/u;
const SPACE = /\s/;
const SUMMARY_KEYS = ["command", "path", "file_path", "pattern", "query", "url"];
const OUTCOME_WORDS = { step_limit: "Step limit reached", interrupted: "Stopped" };
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
  const outcome = OUTCOME_WORDS[ev.reason];
  if (ev.steps === undefined) return outcome || "";
  const parts = [
    `${ev.input_tokens} in`,
    `${ev.output_tokens} out`,
    `ttft ${ev.ttft_ms} ms`,
    ev.steps === 1 ? "1 step" : `${ev.steps} steps`,
  ];
  if (ev.jev) parts.push(`jev ${ev.jev.requests} req`);
  if (outcome) parts.push(outcome);
  return parts.join(" · ");
}

function runLength(s, at) {
  let end = at;
  while (s[end] === s[at]) end++;
  return end - at;
}

function matchingTicks(s, from, size) {
  for (let at = s.indexOf("`", from); at >= 0; at = s.indexOf("`", at)) {
    const run = runLength(s, at);
    if (run === size) return at;
    at += run;
  }
  return -1;
}

function codeSpan(s, at, dead) {
  const size = runLength(s, at);
  const key = `code${size}`;
  if (dead.has(key)) return null;
  const close = matchingTicks(s, at + size, size);
  if (close < 0) {
    dead.add(key);
    return null;
  }
  let body = s.slice(at + size, close);
  if (/^ .*\S.* $/s.test(body)) body = body.slice(1, -1);
  return { node: make("code", "", body), end: close + size };
}

function closesEmphasis(s, at, run, delim) {
  if (delim.length === 1 ? run !== 1 : run < 2) return false;
  if (SPACE.test(s[at - 1])) return false;
  return delim[0] !== "_" || !WORD.test(s[at + run] || "");
}

function emphasisEnd(s, from, delim) {
  for (let at = from; at < s.length;) {
    const c = s[at];
    const run = c === "`" || c === delim[0] ? runLength(s, at) : 1;
    if (c === "`") {
      const close = matchingTicks(s, at + run, run);
      at = close < 0 ? at + run : close + run;
    } else if (c === delim[0] && closesEmphasis(s, at, run, delim)) {
      return at + run - delim.length;
    } else {
      at += run;
    }
  }
  return -1;
}

function emphasis(s, at, dead) {
  const run = runLength(s, at);
  const delim = s.slice(at, at + Math.min(run, 2));
  const next = s[at + run];
  if (next === undefined || SPACE.test(next) || dead.has(delim)) return null;
  if (delim[0] === "_" && at > 0 && WORD.test(s[at - 1])) return null;
  const close = emphasisEnd(s, at + run, delim);
  if (close < 0) {
    dead.add(delim);
    return null;
  }
  const node = make(delim.length === 2 ? "strong" : "em");
  node.append(inlineNodes(s.slice(at + delim.length, close)));
  return { node, end: close + delim.length };
}

function inlineNodes(s) {
  const out = document.createDocumentFragment();
  const dead = new Set();
  let text = "";
  for (let at = 0; at < s.length;) {
    const c = s[at];
    if (c !== "`" && c !== "*" && c !== "_") {
      text += c;
      at++;
      continue;
    }
    const span = c === "`" ? codeSpan(s, at, dead) : emphasis(s, at, dead);
    if (span) {
      if (text) out.append(text);
      text = "";
      out.append(span.node);
      at = span.end;
    } else {
      const run = runLength(s, at);
      text += s.slice(at, at + run);
      at += run;
    }
  }
  if (text) out.append(text);
  return out;
}

class Leaf {
  constructor(el, kind) {
    this.el = el;
    this.kind = kind;
    this.raw = "";
    this.shown = 0;
  }

  add(text) {
    this.raw += text;
  }

  sync() {
    if (this.shown === this.raw.length) return;
    const delta = this.raw.slice(this.shown);
    const last = this.el.lastChild;
    const plain = this.shown > 0 && last && last.nodeType === Node.TEXT_NODE
      && !MARKER.test(delta) && !MARKER.test(this.raw[this.shown - 1]);
    if (plain) last.appendData(delta);
    else this.el.replaceChildren(inlineNodes(this.raw));
    this.shown = this.raw.length;
  }
}

class TextBlock {
  constructor() {
    this.el = make("div", "answer");
    this.node = null;
    this.code = false;
    this.held = "";
    this.midLine = false;
    this.leaf = null;
    this.list = null;
    this.gap = false;
  }

  closeLeaf() {
    if (this.leaf) this.leaf.sync();
    this.leaf = null;
  }

  openLeaf(tag, kind, parent = this.el) {
    this.closeLeaf();
    const el = make(tag);
    parent.append(el);
    this.leaf = new Leaf(el, kind);
    this.gap = false;
  }

  listItem(tag, match, line) {
    if (!this.list || this.list.localName !== tag) {
      this.closeLeaf();
      this.list = make(tag);
      if (tag === "ol") this.list.start = Number(match[1]);
      this.el.append(this.list);
    }
    this.openLeaf("li", "item", this.list);
    this.leaf.add(line.slice(match[0].length));
  }

  heading(match, line) {
    this.list = null;
    this.openLeaf(match[1].length <= 2 ? "h3" : "h4", "heading");
    this.leaf.add(line.slice(match[0].length));
  }

  quote(match, line) {
    const text = line.slice(match[0].length);
    if (this.leaf && this.leaf.kind === "quote" && !this.gap) {
      this.leaf.add(`\n${text}`);
      return;
    }
    this.list = null;
    this.openLeaf("blockquote", "quote");
    this.leaf.add(text);
  }

  plain(line) {
    if (this.leaf && !this.gap) {
      this.leaf.add(`\n${line}`);
      return;
    }
    this.list = null;
    this.openLeaf("p", "paragraph");
    this.leaf.add(line);
  }

  block(line) {
    if (line.trim() === "") {
      this.closeLeaf();
      this.gap = true;
      return;
    }
    const bullet = BULLET.exec(line);
    const ordered = ORDERED.exec(line);
    const heading = HEADING.exec(line);
    const quote = QUOTE.exec(line);
    if (bullet) this.listItem("ul", bullet, line);
    else if (ordered) this.listItem("ol", ordered, line);
    else if (heading) this.heading(heading, line);
    else if (quote) this.quote(quote, line);
    else this.plain(line);
  }

  toggleFence() {
    this.closeLeaf();
    this.list = null;
    this.gap = false;
    if (this.node && this.node.data.endsWith("\n")) this.node.deleteData(this.node.length - 1, 1);
    this.code = !this.code;
    this.node = null;
    if (!this.code) return;
    this.node = document.createTextNode("");
    const pre = codeRegion("", "Code block", "code");
    pre.append(this.node);
    this.el.append(pre);
  }

  line(text) {
    const bare = text.replace(/\r?\n$/, "");
    if (this.code) {
      if (!this.midLine && FENCE_CLOSE.test(bare)) this.toggleFence();
      else this.node.appendData(text);
    } else if (this.midLine && this.leaf) {
      this.leaf.add(bare);
    } else if (FENCE_OPEN.test(bare)) {
      this.toggleFence();
    } else {
      this.block(bare);
    }
    this.midLine = false;
    if (this.leaf && this.leaf.kind === "heading") this.closeLeaf();
  }

  partial(text) {
    const start = !this.midLine;
    if (start && (this.code ? MAYBE_FENCE : MAYBE_BLOCK).test(text)) {
      this.held = text;
      return;
    }
    if (this.code) this.node.appendData(text);
    else if (!start && this.leaf) this.leaf.add(text);
    else this.block(text);
    this.midLine = true;
  }

  feed(chunk) {
    let rest = this.held + chunk;
    this.held = "";
    for (let cut = rest.indexOf("\n"); cut >= 0; cut = rest.indexOf("\n")) {
      this.line(rest.slice(0, cut + 1));
      rest = rest.slice(cut + 1);
    }
    if (rest !== "") this.partial(rest);
    if (this.leaf) this.leaf.sync();
  }

  finish() {
    if (this.held !== "") this.line(this.held);
    this.held = "";
    this.closeLeaf();
    if (this.code) this.toggleFence();
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
    this.workingLabel = document.createTextNode("Working");
    this.working.append(make("span", "spinner"), this.workingLabel);
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

  setActivity(label) {
    this.workingLabel.data = label;
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
      this.setActivity("Working");
      this.pending += ev.text || "";
      this.schedule();
      return true;
    }
    this.setActivity(ev.kind === "thinking" ? "thinking…" : "Working");
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
