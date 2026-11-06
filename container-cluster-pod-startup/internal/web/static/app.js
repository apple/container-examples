// The page: the pods measured, two headline percentiles, the database status, and the
// first percentile by namespace over time beside a table of every namespace. It renders
// whatever /api/events sends, and redraws every second so the chart's "now" keeps moving.
"use strict";

const RANGES = { "3m": 180, "15m": 900, "1h": 3600 }; // seconds
const POINTS = 30; // samples across the chart
const OWN = "#FF9F0A";
const OTHERS = ["#8E8E93", "#64D2FF", "#BF5AF2", "#30D158", "#FF6482", "#FFD60A"];
const $ = (id) => document.getElementById(id);
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
const fmt = (s) => s.toFixed(1) + " s";

let st = null; // the state shown
let held = null; // the latest state while paused
const asked = new URLSearchParams(location.search).get("range");
let range = RANGES[asked] ? asked : "3m";
// The server sends the state whenever it changes, so while the stream is open the page is
// up to date; lostAt is when it closed.
let live = true, connected = false, lostAt = 0, frozenAt = 0;

// Colours stay with a namespace for as long as the page is open: the app's own is orange,
// the others take the next free one.
const colours = new Map();
function colour(ns) {
  if (st && ns === st.namespace) return OWN;
  if (!colours.has(ns)) colours.set(ns, OTHERS[colours.size % OTHERS.length]);
  return colours.get(ns);
}

// percentile matches tracker.Percentile: interpolated between the closest ranks.
function percentile(values, p) {
  if (!values.length) return 0;
  const v = [...values].sort((a, b) => a - b);
  const rank = (p / 100) * (v.length - 1), lo = Math.floor(rank);
  if (lo >= v.length - 1) return v[v.length - 1];
  return v[lo] + (v[lo + 1] - v[lo]) * (rank - lo);
}

function render() {
  if (!st) return;
  const own = st.namespaces.find((n) => n.name === st.namespace) || st.namespaces[0];
  const other = st.namespaces.find((n) => n !== own);
  const first = st.percentiles[0], last = st.percentiles[st.percentiles.length - 1];

  $("brand").textContent = st.namespace;
  $("total").textContent = st.rows.length;
  card($("card-own"), own, first && first.p, 0);
  card($("card-other"), other, last && last.p, st.percentiles.length - 1);

  const up = st.database === "connected";
  $("db-dot").className = "db-dot " + (up ? "up" : "down");
  $("db").className = "db" + (up ? "" : " down");
  $("db").textContent = up ? "Connected" : "Unreachable";

  $("chart-title").textContent = first ? `Pod startup time, p${first.p} by namespace` : "Pod startup time";
  table();
  $("pending").textContent = st.pending;
  chart(first ? first.p : 50);
}

// card fills a headline card with namespace ns at the i-th percentile p.
function card(el, ns, p, i) {
  const [label, value] = el.children;
  el.classList.toggle("empty", !ns || p === undefined);
  if (!ns || p === undefined) {
    label.textContent = "No pods yet";
    value.children[0].textContent = "–";
    value.children[2].textContent = "";
    return;
  }
  label.textContent = `${ns.name} p${p}`;
  value.children[0].textContent = ns.percentiles[i].seconds.toFixed(1);
  value.children[2].textContent = ns.pods === 1 ? "1 pod" : `${ns.pods} pods`;
}

function table() {
  const head = "<tr><th>Namespace</th><th>Pods</th>" + st.percentiles.map((v) => `<th>p${v.p}</th>`).join("") + "</tr>";
  const rows = st.namespaces.map((n) =>
    `<tr><td title="${esc(n.name)}"><span class="swatch" style="background:${colour(n.name)}"></span><span class="ns" data-t>${esc(n.name)}</span></td>` +
    `<td data-t>${n.pods}</td>` + n.percentiles.map((v) => `<td data-t>${fmt(v.seconds)}</td>`).join("") + "</tr>").join("");
  $("table").innerHTML = head + rows;
}

// niceTop is a round top for the y axis, in at most four steps.
function niceTop(v) {
  for (const step of [0.5, 1, 2, 5, 10, 15, 20, 30, 60, 120, 300]) if (Math.ceil((v * 1.05) / step) <= 4) return [Math.max(step, Math.ceil((v * 1.05) / step) * step), step];
  return [Math.ceil(v / 300) * 300, Math.ceil(v / 1200) * 300];
}
const ago = (s) => (s === 0 ? "now" : s % 3600 === 0 ? `${s / 3600}h ago` : `${Math.round(s / 60)}m ago`);

// chart draws, for every namespace, the p-th percentile of its pods Ready by each moment,
// from when the controller started (it knows nothing before) to now.
function chart(p) {
  const el = $("chart");
  const W = el.clientWidth, H = el.clientHeight;
  if (!W || !H) return;
  const rem = parseFloat(getComputedStyle(document.documentElement).fontSize);
  const L = 3.5 * rem, R = W - 0.6 * rem, T = 0.6 * rem, B = H - 2.4 * rem;
  const span = RANGES[range] * 1000, now = live ? Date.now() : frozenAt, t0 = now - span;
  const started = st.started ? Date.parse(st.started) : 0;
  const from = Math.max(t0, started > 0 ? started : t0);

  const times = [];
  if (from < now) {
    times.push(from);
    for (let i = 1; i <= POINTS; i++) { const t = t0 + (span * i) / POINTS; if (t > from) times.push(t); }
  }
  const byNS = new Map();
  for (const r of st.rows) {
    if (!byNS.has(r.namespace)) byNS.set(r.namespace, []);
    byNS.get(r.namespace).push([Date.parse(r.readyAt), r.ready]);
  }
  // Draw the app's own namespace last, on top.
  const order = st.namespaces.map((n) => n.name).reverse();
  const series = order.map((ns) => {
    const rows = byNS.get(ns) || [];
    const pts = times.map((t) => { const v = rows.filter(([at]) => at <= t).map(([, s]) => s); return v.length ? [t, percentile(v, p)] : null; }).filter(Boolean);
    return { ns, pts };
  }).filter((s) => s.pts.length);

  const [top, step] = niceTop(Math.max(1, ...series.flatMap((s) => s.pts.map(([, v]) => v))));
  const x = (t) => L + ((t - t0) / span) * (R - L);
  const y = (v) => B - (v / top) * (B - T);
  const out = [];
  out.push("<defs>" + series.map((s, i) => `<linearGradient id="g${i}" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${colour(s.ns)}" stop-opacity="${s.ns === st.namespace ? 0.3 : 0.16}"/><stop offset="1" stop-color="${colour(s.ns)}" stop-opacity="0.02"/></linearGradient>`).join("") + "</defs>");
  for (let v = 0; v <= top + 1e-9; v += step) {
    out.push(`<line x1="${L}" x2="${R}" y1="${y(v)}" y2="${y(v)}" stroke="${v === 0 ? "#3A3A3F" : "#26262A"}"/>`);
    out.push(`<text x="${L - 0.6 * rem}" y="${y(v)}" text-anchor="end" dominant-baseline="middle" data-t>${Number.isInteger(v) ? v : v.toFixed(1)} s</text>`);
  }
  for (let i = 0; i <= 3; i++) {
    const s = (RANGES[range] * (3 - i)) / 3, xi = L + ((R - L) * i) / 3;
    if (i === 1 || i === 2) out.push(`<line x1="${xi}" x2="${xi}" y1="${T}" y2="${B}" stroke="#26262A"/>`);
    out.push(`<text x="${xi}" y="${B + 1.5 * rem}" text-anchor="${i === 0 ? "start" : i === 3 ? "end" : "middle"}" data-t>${ago(s)}</text>`);
  }
  if (started > t0 && started < now) {
    const xs = x(started);
    out.push(`<line x1="${xs}" x2="${xs}" y1="${T}" y2="${B}" stroke="#8E8E93" stroke-width="1.5" stroke-dasharray="4 4"/>`);
    if (st.namespace) out.push(`<text x="${xs + 0.6 * rem}" y="${B - 0.9 * rem}" data-t>${esc(st.namespace)} started</text>`);
  }
  series.forEach((s, i) => {
    const c = colour(s.ns), line = s.pts.map(([t, v]) => `${x(t).toFixed(1)},${y(v).toFixed(1)}`).join(" L");
    const [lt, lv] = s.pts[s.pts.length - 1];
    out.push(`<path d="M${line} L${x(lt).toFixed(1)},${B} L${x(s.pts[0][0]).toFixed(1)},${B} Z" fill="url(#g${i})"/>`);
    out.push(`<path d="M${line}" fill="none" stroke="${c}" stroke-width="2.5" stroke-linejoin="round"/>`);
    for (const [t, v] of s.pts.slice(0, -1)) out.push(`<circle cx="${x(t).toFixed(1)}" cy="${y(v).toFixed(1)}" r="2.6" fill="${c}"/>`);
    if (s.ns === st.namespace) out.push(`<circle cx="${x(lt).toFixed(1)}" cy="${y(lv).toFixed(1)}" r="${0.75 * rem}" fill="${c}" opacity=".28"/>`);
    out.push(`<circle cx="${x(lt).toFixed(1)}" cy="${y(lv).toFixed(1)}" r="${0.36 * rem}" fill="${c}"/>`);
  });
  el.innerHTML = `<svg viewBox="0 0 ${W} ${H}">${out.join("")}</svg>`;
}

function status() {
  const s = Math.round((Date.now() - lostAt) / 1000);
  $("updated").textContent = !live ? "Paused" : connected ? "Updated just now" : !st ? "Connecting…" :
    s < 60 ? `Updated ${s}s ago` : `Updated ${Math.floor(s / 60)}m ago`;
  $("live").classList.toggle("on", live && connected);
  $("live-word").textContent = live ? "Live" : "Paused";
  for (const b of $("ranges").children) b.classList.toggle("on", b.dataset.range === range);
}

$("ranges").addEventListener("click", (e) => {
  if (!e.target.dataset.range) return;
  range = e.target.dataset.range;
  status();
  render();
});
$("live").addEventListener("click", () => {
  live = !live;
  if (!live) frozenAt = Date.now();
  else if (held) { st = held; held = null; }
  status();
  render();
});
addEventListener("resize", render);
setInterval(() => { status(); if (live) render(); }, 1000);
status();

const es = new EventSource("api/events");
es.addEventListener("open", () => { connected = true; status(); });
es.addEventListener("error", () => { if (connected) lostAt = Date.now(); connected = false; status(); });
es.addEventListener("state", (m) => {
  connected = true;
  if (live) { st = JSON.parse(m.data); render(); } else held = JSON.parse(m.data);
  status();
});
