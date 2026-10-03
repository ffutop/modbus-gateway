// ModMux management console. Plain JS, no build step: everything it shows
// comes from the /api/v1 endpoints served by the same process.
'use strict';

const $ = (s) => document.querySelector(s);
// desktop is the API the Electron shell exposes (desktop/preload.js); null
// in a browser, where desktop-only controls are not shown.
const desktop = window.modmuxDesktop ?? null;
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

const state = {
  config: null, // GET /api/v1/config response; config is the file as saved
  sel: null, // {kind: 'gw'|'ds'|'sim', gw, ds, sim} indexes into the config tree
  edits: new Map(), // pathKey -> {op:'set', path, value}: unsaved changes
  problems: [], // from POST /api/v1/config/validate for the current edits
  validating: false,
  stale: false, // the file changed on disk since it was loaded (409)
  dockTab: 'log', // 'log' | 'problems' | 'output' (desktop only)
  output: [], // desktop only: the gateway process's recent output lines
  log: [], // request events from /api/v1/events, newest first
  metrics: new Map(), // series key ("gw" or "gw/ds") -> latest Counts
  rates: new Map(), // series key -> req/s between the last two polls
  history: new Map(), // series key -> last 40 req/s samples
  status: null, // GET /api/v1/status
  view: 'detail', // editor tab below the wide tier: 'detail' | 'topo' 
  reg: { table: 'holding_registers', start: 0, page: 0, values: [], prev: new Map() },
};

const TABLES = [['holding_registers', '保持寄存器 4x'], ['input_registers', '输入寄存器 3x'], ['coils', '线圈 0x'], ['discrete_inputs', '离散输入 1x']];
const isBits = (table) => table === 'coils' || table === 'discrete_inputs';

const LOG_CAP = 600;
// K is the size scale from app.css (--k); pixel geometry computed here must
// match the CSS sizes it lays out.
const K = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--k')) || 1;
const ROW_H = Math.round(24 * K);
const FC = { 1: '读线圈', 2: '读离散输入', 3: '读保持', 4: '读输入', 5: '写单线圈', 6: '写单寄存器', 15: '写多线圈', 16: '写多寄存器' };
const hex = (n, w) => n.toString(16).toUpperCase().padStart(w, '0');
const fmtMs = (v) => (v < 1 ? v.toFixed(2) : v.toFixed(1)) + ' ms';
// fit is how many rows of height h fit in el below a header of height h:
// lists show the newest rows that fit instead of scrolling.
const fit = (el, h) => (el ? Math.max(0, Math.floor(el.clientHeight / h) - 1) : 0);

async function api(path, init) {
  const res = await fetch(path, init);
  const body = await res.json().catch(() => ({}));
  if (!res.ok && res.status !== 422 && res.status !== 409) throw new Error(`${path}: ${res.status} ${body.error ?? ''}`);
  return { status: res.status, body };
}

const pathKey = (path) => JSON.stringify(path);
const getIn = (tree, path) => path.reduce((n, k) => (n == null ? n : n[k]), tree);

// cfg is the config as the user currently sees it: the saved file with the
// unsaved edits applied.
function cfg() {
  const draft = structuredClone(state.config.config);
  for (const { path, value } of state.edits.values()) {
    const parent = getIn(draft, path.slice(0, -1));
    if (parent) parent[path[path.length - 1]] = value;
  }
  return draft;
}

const problemsUnder = (prefix) =>
  state.problems.filter((p) => prefix.every((k, i) => p.path[i] === k));
const sameSel = (a, b) => JSON.stringify(a) === JSON.stringify(b);

// selKey names the telemetry series of the selection: "gateway" or
// "gateway/downstream"; null when the selection has no series.
function selKey() {
  const s = state.sel;
  if (!s || s.kind === 'sim') return null;
  const gw = cfg().gateways[s.gw];
  return s.kind === 'ds' ? `${gw.name}/${gw.downstreams[s.ds].name}` : gw.name;
}

function renderTree() {
  const item = (label, depth, sel, right = '', invalid = false) =>
    `<div class="node" role="treeitem" aria-level="${depth + 1}" aria-selected="${sameSel(sel, state.sel)}"${invalid ? ' aria-invalid="true"' : ''} style="--d:${depth}" data-sel='${JSON.stringify(sel)}'>${label}<span class="r">${right}</span></div>`;
  let h = '<div class="sec">网关</div>';
  (cfg().gateways || []).forEach((gw, gi) => {
    h += item(`<b>${esc(gw.name)}</b>`, 0, { kind: 'gw', gw: gi });
    (gw.downstreams || []).forEach((ds, di) => {
      const bad = problemsUnder(['gateways', gi, 'downstreams', di]).length > 0;
      h += item(`<span class="muted">→</span> ${esc(ds.name)}`, 1, { kind: 'ds', gw: gi, ds: di },
        bad ? '<span class="err-txt" title="有校验错误">●</span>' : `<span class="faint mono">${esc(ds.slave_ids)}</span>`, bad);
    });
  });
  h += '<div class="sec">模拟从站</div>';
  (cfg().simulations || []).forEach((sim, si) => {
    h += item(esc(sim.name), 0, { kind: 'sim', sim: si }, `<span class="faint">${esc(sim.persistence?.type)}</span>`);
  });
  $('#side').innerHTML = h;
  for (const n of document.querySelectorAll('#side [data-sel]')) {
    n.onclick = () => select(JSON.parse(n.dataset.sel));
  }
}

let fieldSeq = 0;
// field renders one input; with a path it is editable and records a "set"
// edit at that path in the config file.
function field(label, value, path) {
  const id = `f${++fieldSeq}`;
  const bind = path ? `data-path='${JSON.stringify(path)}'` : 'readonly';
  return `<div class="field"><label for="${id}">${label}</label><input id="${id}" value="${esc(value)}" ${bind}></div>` +
    (path ? `<div class="field-err" data-err-for='${JSON.stringify(path)}'></div>` : '');
}

function renderEditor() {
  const s = state.sel;
  let h = '';
  if (s?.kind === 'ds') {
    const gw = cfg().gateways[s.gw];
    const ds = gw.downstreams[s.ds];
    const p = (...rest) => ['gateways', s.gw, 'downstreams', s.ds, ...rest];
    const target = ds.simulation
      ? field('模拟从站', ds.simulation.ref, p('simulation', 'ref'))
      : ds.tcp ? field('目标地址', ds.tcp.address, p('tcp', 'address'))
      : ds.serial ? field('串口', ds.serial.device, p('serial', 'device')) : '';
    h = `<div class="h"><h2>${esc(ds.name)}</h2><span class="badge">${esc(ds.type)}</span><span class="muted">属于网关 ${esc(gw.name)}</span></div>
      <div class="cols"><div class="box"><h4>属性</h4>
        ${field('名称', ds.name, p('name'))}${field('类型', ds.type)}${field('Slave IDs', ds.slave_ids, p('slave_ids'))}${target}
      </div></div>`;
  } else if (s?.kind === 'gw') {
    h = `<div class="h"><h2>${esc(cfg().gateways[s.gw].name)}</h2></div>`;
  } else if (s?.kind === 'sim') {
    const sim = cfg().simulations[s.sim];
    const r = state.reg;
    h = `<div class="h"><h2>${esc(sim.name)}</h2><span class="badge" id="sim-status"></span>
        <span class="muted">${esc(sim.persistence?.type)}${sim.persistence?.path ? ' · ' + esc(sim.persistence.path) : ''}</span><span class="muted mono" id="sim-version"></span></div>
      <div class="regbar">
        <div class="seg">${TABLES.map(([t, l]) => `<button aria-pressed="${t === r.table}" data-table="${t}">${l}</button>`).join('')}</div>
        <span class="sp"></span>
        <button class="btn" data-step="-1" aria-label="上一屏">‹</button>
        <label class="muted" for="jump">起始地址 0x</label><input id="jump" class="mono jump" value="${hex(r.start, 4)}">
        <button class="btn" data-step="1" aria-label="下一屏">›</button>
        <span class="muted mono" id="reg-range"></span>
      </div>
      <div class="box grid-box"><div id="grid" role="grid" aria-label="寄存器"></div></div>`;
  }
  const split = isWide();
  const showDetail = split || state.view === 'detail';
  const showTopo = split || state.view === 'topo';
  const tabs = split ? '' : `<div class="tabs" role="tablist">
      <button role="tab" aria-selected="${state.view === 'detail'}" data-view="detail">${esc(selTitle())}</button>
      <button role="tab" aria-selected="${state.view === 'topo'}" data-view="topo">⌖ 拓扑 · ${esc(topoGateway()?.name ?? '')}</button>
      <span class="hint">窗口 ≥1920 宽时两者并排</span></div>`;
  $('#center').innerHTML = `${tabs}<div class="panes">
      ${showDetail ? `<div class="pane">${split ? `<div class="pane-h">${esc(selTitle())}</div>` : ''}<div class="body">${h}</div></div>` : ''}
      ${showTopo ? `<div class="pane">${split ? `<div class="pane-h">⌖ 拓扑 · ${esc(topoGateway()?.name ?? '')}</div>` : ''}<div class="topo" id="topo" role="group" aria-label="拓扑"></div></div>` : ''}
    </div>`;
  for (const t of document.querySelectorAll('#center [data-view]')) {
    t.onclick = () => { state.view = t.dataset.view; renderEditor(); };
  }
  if (showTopo) renderTopology();
  for (const input of document.querySelectorAll('#center input[data-path]')) {
    input.oninput = () => setEdit(JSON.parse(input.dataset.path), input.value);
  }
  for (const b of document.querySelectorAll('#center [data-table]')) {
    b.onclick = () => { state.reg.table = b.dataset.table; state.reg.prev.clear(); renderEditor(); };
  }
  for (const b of document.querySelectorAll('#center [data-step]')) {
    b.onclick = () => moveRegisters(state.reg.start + Number(b.dataset.step) * state.reg.page);
  }
  const jump = $('#jump');
  if (jump) jump.onkeydown = (e) => { if (e.key === 'Enter') moveRegisters(parseInt(jump.value, 16) || 0); };
  showFieldProblems();
  if (s?.kind === 'sim') { showSimStatus(); refreshRegisters(); }
}

const isWide = () => innerWidth >= 1920;

function selTitle() {
  const s = state.sel;
  if (!s) return '';
  if (s.kind === 'sim') return cfg().simulations[s.sim].name;
  const gw = cfg().gateways[s.gw];
  return s.kind === 'ds' ? gw.downstreams[s.ds].name : gw.name;
}

// topoGatewayIndex picks the gateway the topology shows: the selection's
// own, or for a simulation the first gateway with a downstream bound to it.
function topoGatewayIndex() {
  const s = state.sel, c = cfg(), gws = c.gateways || [];
  if (!s) return 0;
  if (s.kind !== 'sim') return s.gw;
  const name = c.simulations[s.sim].name;
  return Math.max(0, gws.findIndex((g) => (g.downstreams || []).some((d) => d.simulation?.ref === name)));
}
const topoGateway = () => cfg().gateways?.[topoGatewayIndex()];

function renderTopology() {
  const el = $('#topo');
  const gi = topoGatewayIndex();
  const gw = cfg().gateways?.[gi];
  if (!el || !gw) return;
  const W = el.clientWidth, H = el.clientHeight;
  const sims = [...new Set((gw.downstreams || []).map((d) => d.simulation?.ref).filter(Boolean))];
  const nw = Math.max(132, Math.min(200, W / (sims.length ? 5.2 : 4.2)));
  const cx = (sims.length ? [0.13, 0.37, 0.63, 0.87] : [0.17, 0.5, 0.83]).map((c) => W * c);
  const ys = (n, i) => H / 2 + 8 + (i - (n - 1) / 2) * Math.min((H - 40) / (n + 1), 130);
  const simIndex = (name) => cfg().simulations.findIndex((x) => x.name === name);
  const nodes = [];
  (gw.upstreams || []).forEach((u, i) => nodes.push({ id: `up${i}`, sel: { kind: 'gw', gw: gi }, x: cx[0], y: ys(gw.upstreams.length, i), k: `上游 · ${u.type}`, n: u.tcp?.address ?? u.serial?.device ?? '' }));
  nodes.push({ id: 'gw', dark: true, live: gw.name, sel: { kind: 'gw', gw: gi }, x: cx[1], y: ys(1, 0), k: '网关', n: gw.name });
  (gw.downstreams || []).forEach((d, i) => nodes.push({
    id: `ds${i}`, live: `${gw.name}/${d.name}`, sel: { kind: 'ds', gw: gi, ds: i }, x: cx[2], y: ys(gw.downstreams.length, i),
    k: `${d.type} · ID ${d.slave_ids}`, n: d.name, bad: problemsUnder(['gateways', gi, 'downstreams', i]).length > 0,
  }));
  sims.forEach((name, i) => nodes.push({ id: `sim:${name}`, sel: { kind: 'sim', sim: simIndex(name) }, x: cx[3], y: ys(sims.length, i), k: '模拟从站', n: name }));
  const at = Object.fromEntries(nodes.map((n) => [n.id, n]));
  const edges = [];
  (gw.upstreams || []).forEach((_, i) => edges.push([`up${i}`, 'gw', gw.name]));
  (gw.downstreams || []).forEach((d, i) => {
    edges.push(['gw', `ds${i}`, `${gw.name}/${d.name}`]);
    if (d.simulation?.ref) edges.push([`ds${i}`, `sim:${d.simulation.ref}`, `${gw.name}/${d.name}`, d.type === 'injector']);
  });
  const on = nodes.find((n) => sameSel(n.sel, state.sel) && !n.id.startsWith('up'))?.id;
  const half = nw / 2;
  const curve = (a, b) => { const mx = (a.x + b.x) / 2; return `M${a.x + half},${a.y} C${mx},${a.y} ${mx},${b.y} ${b.x - half},${b.y}`; };
  el.style.setProperty('--nw', `${nw}px`);
  el.innerHTML = `<svg aria-hidden="true">${edges.map(([a, b, key, inj]) => {
      const hi = on && (a === on || b === on);
      return `<path d="${curve(at[a], at[b])}" fill="none" stroke="${hi ? '#a1a1aa' : '#d4d4d8'}" stroke-width="${hi ? 3 : 2}"/>
        <path d="${curve(at[a], at[b])}" fill="none" stroke="${inj ? '#3b82f6' : '#111'}" stroke-width="1.5" stroke-dasharray="4 6" class="flow" data-edge="${esc(key)}" style="opacity:${on && !hi ? 0.3 : 0.85}"/>
        <text x="${(at[a].x + at[b].x) / 2}" y="${(at[a].y + at[b].y) / 2 - 6}" text-anchor="middle" class="edge-label" ${inj ? '' : `data-edge-label="${esc(key)}"`}>${inj ? '写入映射' : ''}</text>`;
    }).join('')}</svg>
    ${(sims.length ? ['上游', '网关', '下游', '模拟从站'] : ['上游', '网关', '下游']).map((l, i) => `<div class="tcol" style="left:${cx[i]}px">${l}</div>`).join('')}
    ${nodes.map((n) => `<button class="tn${n.dark ? ' gw' : ''}" aria-pressed="${n.id === on}" data-sel='${JSON.stringify(n.sel)}' style="left:${n.x}px;top:${n.y}px">
      <span class="k"><span>${esc(n.k)}</span>${n.bad ? '<span class="err-txt">● 冲突</span>' : ''}</span>
      <span class="n">${esc(n.n)}</span><span class="s"${n.live ? ` data-node-live="${esc(n.live)}"` : ''}></span></button>`).join('')}`;
  for (const b of el.querySelectorAll('[data-sel]')) b.onclick = () => select(JSON.parse(b.dataset.sel));
  showTopologyTraffic();
}

// showTopologyTraffic refreshes edge rates in place on every metrics poll.
function showTopologyTraffic() {
  for (const t of document.querySelectorAll('[data-edge-label]')) t.textContent = `${Math.round(state.rates.get(t.dataset.edgeLabel) ?? 0)} /s`;
  for (const p of document.querySelectorAll('[data-edge]')) {
    const r = state.rates.get(p.dataset.edge) ?? 0;
    p.style.animationDuration = r ? `${Math.max(0.25, 3 / r)}s` : '0s';
  }
  for (const n of document.querySelectorAll('[data-node-live]')) {
    const c = state.metrics.get(n.dataset.nodeLive);
    n.textContent = `${Math.round(state.rates.get(n.dataset.nodeLive) ?? 0)} req/s · p99 ${fmtMs(c?.p99_ms ?? 0)}`;
  }
}

function moveRegisters(start) {
  state.reg.start = Math.min(65535, Math.max(0, start));
  state.reg.prev.clear();
  $('#jump').value = hex(state.reg.start, 4);
  refreshRegisters();
}

function showSimStatus() {
  const name = state.sel?.kind === 'sim' && cfg().simulations[state.sel.sim].name;
  const st = state.status?.simulations?.find((x) => x.name === name);
  const badge = $('#sim-status');
  if (!badge || !st) return;
  badge.textContent = st.status;
  badge.className = `badge ${st.status === 'ready' ? 'ok' : 'warn'}`;
  $('#sim-version').textContent = `v${st.version}`;
}

// gridShape picks columns from the box width and rows from its height, so
// one screen of addresses fits without scrolling; paging is by address.
function gridShape(el, table) {
  const addrW = 60 * K, cellW = (isBits(table) ? 32 : 60) * K;
  const fitCols = Math.floor((el.clientWidth - addrW) / cellW);
  const cols = [32, 16, 10, 8].find((c) => c <= fitCols && (isBits(table) || c <= 16)) || Math.max(4, fitCols);
  return { cols, rows: fit(el, Math.round(22 * K)), addrW };
}

async function refreshRegisters() {
  const el = $('#grid');
  if (!el || state.sel?.kind !== 'sim') return;
  const r = state.reg, name = cfg().simulations[state.sel.sim].name;
  const { cols, rows, addrW } = gridShape(el, r.table);
  const count = Math.max(1, Math.min(2048, rows * cols, 65536 - r.start));
  let body;
  try {
    ({ body } = await api(`/api/v1/simulations/${encodeURIComponent(name)}/registers?table=${r.table}&start=${r.start}&count=${count}`));
  } catch {
    return;
  }
  if (state.sel?.kind !== 'sim' || body.start !== r.start || body.table !== r.table) return; // moved on meanwhile
  r.page = rows * cols;
  const bits = isBits(r.table);
  let h = `<div class="reg-row" role="row" style="grid-template-columns:${addrW}px repeat(${cols},1fr)"><div class="reg addr" role="columnheader"></div>${
    Array.from({ length: cols }, (_, i) => `<div class="reg addr head" role="columnheader">+${i}</div>`).join('')}</div>`;
  for (let row = 0; row * cols < body.values.length; row++) {
    const a0 = r.start + row * cols;
    h += `<div class="reg-row" role="row" style="grid-template-columns:${addrW}px repeat(${cols},1fr)"><div class="reg addr" role="rowheader">${hex(a0, 4)}</div>`;
    for (let c = 0; c < cols && row * cols + c < body.values.length; c++) {
      const addr = a0 + c, v = body.values[row * cols + c];
      const changed = r.prev.has(addr) && r.prev.get(addr) !== v;
      r.prev.set(addr, v);
      h += `<div class="reg${changed ? ' hl' : ''}${v ? '' : ' zero'}" role="gridcell" aria-label="地址 ${addr}">${bits ? (v ? '●' : '○') : v}</div>`;
    }
    h += '</div>';
  }
  el.innerHTML = h;
  $('#reg-range').textContent = `${hex(r.start, 4)}–${hex(r.start + body.values.length - 1, 4)}`;
}

// showFieldProblems marks the editor's fields in place, so typing keeps focus.
function showFieldProblems() {
  for (const input of document.querySelectorAll('#center input[data-path]')) {
    const msgs = state.problems.filter((p) => pathKey(p.path) === input.dataset.path).map((p) => p.message);
    if (msgs.length) input.setAttribute('aria-invalid', 'true');
    else input.removeAttribute('aria-invalid');
    const err = document.querySelector(`[data-err-for='${input.dataset.path}']`);
    if (err) err.textContent = msgs.map((m) => `✕ ${m}`).join(' ');
  }
}

// totals sums every gateway when nothing with its own series is selected.
function seriesFor(key) {
  if (key) return { counts: state.metrics.get(key), rate: state.rates.get(key) ?? 0, hist: state.history.get(key) ?? [] };
  let requests = 0, errors = 0, rate = 0;
  for (const [k, c] of state.metrics) {
    if (k.includes('/')) continue;
    requests += c.requests; errors += c.errors; rate += state.rates.get(k) ?? 0;
  }
  return { counts: { requests, errors, p50_ms: 0, p99_ms: 0 }, rate, hist: [] };
}

function spark(hist, h) {
  const n = 40, w = 240, data = [...Array(Math.max(0, n - hist.length)).fill(0), ...hist];
  const max = Math.max(1, ...data);
  const pts = data.map((v, i) => `${(i / (n - 1) * w).toFixed(1)},${(h - v / max * (h - 2) - 1).toFixed(1)}`).join(' ');
  return `<svg viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" style="display:block;width:100%;height:${h}px" aria-hidden="true">
    <polyline points="0,${h} ${pts} ${w},${h}" fill="#111" fill-opacity=".07"/><polyline points="${pts}" fill="none" stroke="#111" stroke-width="1.5" vector-effect="non-scaling-stroke"/></svg>`;
}

function renderInspector() {
  const key = selKey();
  const { counts, rate, hist } = seriesFor(key);
  const c = counts ?? { requests: 0, errors: 0, p50_ms: 0, p99_ms: 0 };
  const errRate = c.requests ? (c.errors / c.requests * 100).toFixed(2) : '0.00';
  const kpi = (label, value) => `<div><b aria-label="${label}">${value}</b><span>${label}</span></div>`;
  $('#insp').innerHTML = `<div class="insp-h">运行指标 <span class="muted">· ${esc(key ?? '全部')}</span></div>
    <div class="kpi">${kpi('请求/秒', Math.round(rate))}${kpi('错误率', `${errRate}%`)}${kpi('p50', fmtMs(c.p50_ms))}${kpi('p99', fmtMs(c.p99_ms))}</div>
    ${key ? `<div><div class="muted small">请求/秒 · 近 40 秒</div>${spark(hist, 52)}</div>` : ''}
    <dl class="totals"><dt>累计请求</dt><dd class="mono" aria-label="累计请求">${c.requests}</dd><dt>累计错误</dt><dd class="mono" aria-label="累计错误">${c.errors}</dd></dl>
    ${innerHeight >= 1000 ? '<div class="small">最近错误</div><ul class="recent" id="recent" aria-label="最近错误"></ul>' : ''}`;
  const recent = $('#recent');
  if (recent) {
    const errors = state.log.filter((e) => e.error && logMatches(e, key)).slice(0, fit(recent, Math.round(38 * K)) + 1);
    recent.innerHTML = errors.map((e) => `<li><div class="e">${esc(e.error)}</div><div class="muted mono small">${e.time.slice(11, 23)} · ${esc(e.downstream || '—')} · slave ${e.slave_id}</div></li>`).join('')
      || '<li class="muted">无</li>';
  }
}

// logMatches reports whether a request belongs to the series key (all
// requests when key is null).
const logMatches = (e, key) => !key || (key.includes('/') ? `${e.gateway}/${e.downstream}` === key : e.gateway === key);

// Log columns drop in priority order (4 first) when the dock is narrow.
const LOG_COLS = [
  ['时间', 108, 1, (e) => `<span class="mono muted">${e.time.slice(11, 23)}</span>`],
  ['网关', 110, 3, (e) => esc(e.gateway)],
  ['来源', 130, 4, (e) => `<span class="mono">${esc(e.source)}</span>`],
  ['从站', 46, 1, (e) => `<span class="mono">${e.slave_id}</span>`],
  ['功能码', 104, 1, (e) => `<span class="mono">${hex(e.function_code, 2)}</span> ${FC[e.function_code] ?? ''}`],
  ['地址', 80, 2, (e) => `<span class="mono">${e.address}${e.quantity > 1 ? '+' + e.quantity : ''}</span>`],
  ['下游', 110, 2, (e) => esc(e.downstream || '—')],
  ['耗时', 72, 1, (e) => `<span class="mono">${fmtMs(e.duration_ms)}</span>`],
  ['结果', 0, 1, (e) => (e.error ? `<span class="err-txt">${esc(e.error)}</span>` : '<span class="ok-txt">OK</span>')],
];

function renderLog() {
  const el = $('#log');
  if (!el) return;
  const key = selKey();
  let cols = LOG_COLS;
  for (const lvl of [4, 3, 2]) {
    if (cols.reduce((w, c) => w + c[1] * K, 150) > el.clientWidth) cols = cols.filter((c) => c[2] < lvl);
  }
  const rows = state.log.filter((e) => logMatches(e, key)).slice(0, fit(el, ROW_H));
  el.innerHTML = `<table class="t" aria-label="请求日志"><colgroup>${cols.map((c) => `<col style="${c[1] ? `width:${Math.round(c[1] * K)}px` : ''}">`).join('')}</colgroup>
    <thead><tr>${cols.map((c) => `<th>${c[0]}</th>`).join('')}</tr></thead>
    <tbody>${rows.map((e) => `<tr${e.error ? ' class="bad"' : ''}>${cols.map((c) => `<td>${c[3](e)}</td>`).join('')}</tr>`).join('')}</tbody></table>`;
}

let logFrame = 0;
function scheduleLog() {
  if (!logFrame) logFrame = requestAnimationFrame(() => { logFrame = 0; renderLog(); });
}

function subscribeEvents() {
  const es = new EventSource('/api/v1/events');
  es.onmessage = (msg) => {
    const batch = JSON.parse(msg.data);
    state.log = [...batch.reverse(), ...state.log].slice(0, LOG_CAP);
    scheduleLog();
  };
}

// pollMetrics derives req/s from the change in cumulative counts between
// two polls; the server keeps no rate windows.
let lastPoll = 0;
async function pollMetrics() {
  const now = performance.now();
  let body;
  try {
    ({ body } = await api('/api/v1/metrics'));
  } catch {
    // The gateway is stopped or restarting; keep the last view and retry.
    setOnline(false);
    lastPoll = 0;
    return;
  }
  setOnline(true);
  const dt = lastPoll ? (now - lastPoll) / 1000 : 0;
  lastPoll = now;
  const next = new Map();
  for (const g of body.gateways ?? []) {
    next.set(g.name, g);
    for (const d of g.downstreams ?? []) next.set(`${g.name}/${d.name}`, d);
  }
  for (const [key, c] of next) {
    const prev = state.metrics.get(key);
    const rate = prev && dt ? (c.requests - prev.requests) / dt : 0;
    state.rates.set(key, rate);
    state.history.set(key, [...(state.history.get(key) ?? []), rate].slice(-40));
  }
  state.metrics = next;
  renderInspector();
  showTopologyTraffic();
  try { state.status = (await api('/api/v1/status')).body; } catch { return; }
  showSimStatus();
  refreshRegisters();
}

function renderDock() {
  const key = selKey();
  const n = state.problems.length;
  const tab = (id, label) => `<button role="tab" aria-selected="${state.dockTab === id}" data-dock="${id}">${label}</button>`;
  $('#dock').innerHTML = `<div class="dock-tabs" role="tablist">
      ${tab('log', '请求日志')}${tab('problems', `问题${n ? ` <span class="badge err">${n}</span>` : ''}`)}${desktop ? tab('output', '网关输出') : ''}
      <span class="sp"></span><span class="faint">${state.dockTab === 'log' ? (key ? `筛选：${esc(key)}` : '全部网关') : ''}</span>
    </div><div class="fill" id="dock-body"></div>`;
  for (const t of document.querySelectorAll('[data-dock]')) {
    t.onclick = () => { state.dockTab = t.dataset.dock; renderDock(); };
  }
  renderDockBody();
}

function renderDockBody() {
  const body = $('#dock-body');
  if (!body) return;
  if (state.dockTab === 'log') {
    body.innerHTML = '<div class="fill" id="log"></div>';
    renderLog();
  } else if (state.dockTab === 'problems') {
    body.innerHTML = state.problems.length
      ? `<table class="t" aria-label="问题"><tbody>${state.problems.map((p) => `<tr><td class="err-txt" style="width:24px">✕</td><td>${esc(p.message)}</td></tr>`).join('')}</tbody></table>`
      : '<div class="muted pad">没有发现问题</div>';
  } else {
    renderOutput();
  }
}

let outputFrame = 0;
function scheduleOutput() {
  if (!outputFrame) outputFrame = requestAnimationFrame(() => { outputFrame = 0; renderOutput(); });
}

// renderOutput shows the newest gateway output lines that fit.
function renderOutput() {
  const body = $('#dock-body');
  if (!body || state.dockTab !== 'output') return;
  const lines = state.output.slice(-Math.max(1, Math.floor(body.clientHeight / Math.round(18 * K))));
  body.innerHTML = `<div class="output mono" role="log" aria-label="网关输出">${lines.map((l) => `<div>${esc(l)}</div>`).join('')}</div>`;
}

function renderToolbar() {
  const n = state.edits.size;
  const canSave = n > 0 && state.problems.length === 0 && !state.validating && !state.stale;
  const file = state.status?.config_path?.split(/[\\/]/).pop() || '配置文件';
  $('#toolbar').innerHTML = `<span class="file" title="${esc(state.status?.config_path)}">${esc(file)}${n ? ' •' : ''}</span>
    <button class="btn ${canSave ? 'pri' : ''}" id="save" ${canSave ? '' : 'disabled'}>保存 <span class="kbd">Ctrl+S</span></button>
    ${desktop ? '<button class="btn" id="restart">重启网关</button>' : ''}
    <span class="sp"></span>
    ${state.stale ? '<span class="stale" role="alert">配置文件已在别处被修改，当前修改无法保存。<button class="btn" id="reload">重新加载</button></span>' : ''}
    ${n ? `<span class="badge warn">${n} 处未保存修改 · 保存后需重启生效</span>`
      : state.config.running_matches ? '<span class="badge">配置与运行中一致</span>'
      : '<span class="badge warn">已保存，重启网关后生效</span>'}`;
  $('#save').onclick = save;
  if ($('#restart')) $('#restart').onclick = () => desktop.restartGateway();
  if ($('#reload')) $('#reload').onclick = reload;
}

// reload drops unsaved edits and shows the file as it now is on disk.
async function reload() {
  clearTimeout(validateTimer);
  validateSeq++;
  state.edits.clear();
  state.problems = [];
  state.stale = false;
  state.validating = false;
  await loadConfig();
  render();
}

async function loadConfig() {
  state.config = (await api('/api/v1/config')).body;
}

async function save() {
  clearTimeout(validateTimer);
  const { status, body } = await api('/api/v1/config', {
    method: 'PUT',
    body: JSON.stringify({ base_revision: state.config.revision, edits: [...state.edits.values()] }),
  });
  if (status === 409) {
    state.stale = true;
    renderToolbar();
    return;
  }
  if (status === 422) {
    state.problems = body.problems ?? [];
    render();
    return;
  }
  state.edits.clear();
  state.problems = [];
  await loadConfig();
  render();
}

// setEdit records (or, when the value is back to the saved one, drops) an
// edit, then revalidates the whole draft shortly after typing stops.
let validateTimer;
function setEdit(path, value) {
  if (getIn(state.config.config, path) === value) state.edits.delete(pathKey(path));
  else state.edits.set(pathKey(path), { op: 'set', path, value });
  state.validating = true;
  renderToolbar();
  clearTimeout(validateTimer);
  validateTimer = setTimeout(validate, 300);
}

let validateSeq = 0;
async function validate() {
  const seq = ++validateSeq;
  const { status, body } = await api('/api/v1/config/validate', {
    method: 'POST',
    body: JSON.stringify({ base_revision: state.config.revision, edits: [...state.edits.values()] }),
  });
  if (seq !== validateSeq) return; // superseded by newer typing
  state.stale = status === 409;
  state.problems = body.problems ?? [];
  state.validating = false;
  renderTree();
  renderToolbar();
  renderDock();
  showFieldProblems();
  renderTopology(); // node labels and conflict marks follow the draft
}

function setOnline(online) {
  if (state.online === online) return;
  state.online = online;
  $('#status').innerHTML = online
    ? '<span><span class="dot"></span> 已连接网关</span><span class="sp"></span>'
    : '<span role="alert"><span class="dot warn"></span> 与网关的连接已断开，正在重试…</span><span class="sp"></span>';
}

function render() {
  renderToolbar();
  renderTree();
  renderEditor();
  renderInspector();
  renderDock();
}

function select(sel) {
  state.sel = sel;
  render();
}

async function start() {
  await loadConfig();
  state.status = (await api('/api/v1/status')).body;
  addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
      e.preventDefault(); // not the browser's "save page"
      if (!$('#save')?.disabled) save();
    }
  });
  if (cfg().gateways?.[0]?.downstreams?.length) state.sel = { kind: 'ds', gw: 0, ds: 0 };
  render();
  subscribeEvents();
  if (desktop) {
    desktop.onView((view) => { state.view = view; renderEditor(); });
    state.output = await desktop.getOutput();
    desktop.onOutput((lines) => {
      state.output = state.output.concat(lines).slice(-500);
      scheduleOutput();
    });
  }
  pollMetrics();
  setInterval(pollMetrics, 1000);
  let wide = isWide();
  addEventListener('resize', () => {
    scheduleLog();
    renderInspector();
    if (isWide() !== wide) { wide = isWide(); renderEditor(); } else renderTopology();
  });
}

start();
