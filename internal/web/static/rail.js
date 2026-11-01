const STATUS_WORDS = { running: "Running", finished: "Finished", interrupted: "Stopped" };

function shortTime(iso) {
  const when = new Date(iso);
  const sameDay = when.toDateString() === new Date().toDateString();
  if (sameDay) return when.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  return when.toLocaleDateString([], { month: "short", day: "numeric" });
}

function buildRow() {
  const li = document.createElement("li");
  const link = document.createElement("a");
  link.className = "run";
  const mark = document.createElement("span");
  mark.setAttribute("aria-hidden", "true");
  const title = document.createElement("span");
  title.className = "run-title";
  const meta = document.createElement("span");
  meta.className = "run-meta";
  const status = document.createElement("span");
  const time = document.createElement("span");
  time.className = "t-num";
  meta.append(status, document.createTextNode(" · "), time);
  link.append(mark, title, meta);
  li.append(link);
  return { li, link, mark, title, status, time };
}

function fillRow(row, run, current) {
  row.link.href = `#${run.id}`;
  row.link.dataset.status = run.status;
  row.mark.className = run.status === "running" ? "spinner run-mark" : "run-mark";
  row.title.textContent = run.title || run.id;
  row.title.title = run.title || run.id;
  row.status.textContent = STATUS_WORDS[run.status] || run.status;
  row.time.textContent = shortTime(run.updated);
  if (run.id === current) row.link.setAttribute("aria-current", "page");
  else row.link.removeAttribute("aria-current");
}

export class Rail {
  constructor(list, empty, mark) {
    this.list = list;
    this.empty = empty;
    this.mark = mark;
    this.rows = new Map();
    this.runs = [];
    this.current = null;
    this.watchHover();
  }

  render(runs, current) {
    this.runs = runs;
    this.current = current;
    const order = runs.map((run) => {
      const row = this.rows.get(run.id) || buildRow();
      this.rows.set(run.id, row);
      fillRow(row, run, current);
      return row.li;
    });
    this.place(order);
    this.empty.hidden = runs.length > 0;
  }

  place(order) {
    const same = order.length === this.list.children.length
      && order.every((li, i) => this.list.children[i] === li);
    if (same) return;
    const focused = document.activeElement;
    this.list.replaceChildren(...order);
    if (this.list.contains(focused)) focused.focus();
  }

  select(current) {
    this.render(this.runs, current);
  }

  status(id) {
    const run = this.runs.find((r) => r.id === id);
    return run ? run.status : "";
  }

  light(li) {
    const top = li.offsetTop;
    const wasLit = this.mark.dataset.lit === "true";
    this.mark.dataset.glide = String(wasLit);
    this.mark.style.height = `${li.offsetHeight}px`;
    this.mark.style.translate = `0 ${top}px`;
    this.mark.dataset.lit = "true";
  }

  dim() {
    this.mark.dataset.lit = "false";
  }

  watchHover() {
    const rowOf = (target) => target.closest && target.closest("li");
    let queued = null;
    this.list.addEventListener("mousemove", (event) => {
      if (queued === null) requestAnimationFrame(() => {
        if (queued) this.light(queued);
        queued = null;
      });
      queued = rowOf(event.target);
    });
    this.list.addEventListener("mouseleave", () => this.dim());
    this.list.addEventListener("focusin", (event) => {
      if (event.target.matches(":focus-visible")) this.light(rowOf(event.target));
    });
    this.list.addEventListener("focusout", () => this.dim());
  }
}
