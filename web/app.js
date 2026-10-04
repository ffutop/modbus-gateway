// ModMux management console. Plain JS, no build step: everything it shows
// comes from the /api/v1 endpoints served by the same process.
'use strict';

const $ = (s) => document.querySelector(s);
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

const state = {
  running: null,
  saving: false, notice: "", paused: false, frozenLog: [], errorsOnly: false, logScope: "selected", slaveFilter: "", fcFilter: "", addressBase: 16, search: "", collapsed: new Set(), topoGW: null,
  config: null, // GET /api/v1/config response; config is the file as saved
  sel: null, // {kind: 'gw'|'ds'|'sim', gw, ds, sim} indexes into the config tree
  edits: new Map(), // pathKey -> {op:'set', path, value}: unsaved changes
  problems: [], // from POST /api/v1/config/validate for the current edits
  validating: false,
  stale: false, // the file changed on disk since it was loaded (409)
  dockTab: 'log', // 'log' | 'problems'
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
// K is the size scale from tokens.css (--k); pixel geometry computed here must
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
// unsaved edits applied. It is rebuilt only after the file or the edits
// change (see changed); callers must treat it as read-only.
let draft = null;
function cfg() {
  if (!draft) {
    draft = structuredClone(state.config.config);
    for (const { path, value } of state.edits.values()) {
      const parent = getIn(draft, path.slice(0, -1));
      if (parent) parent[path[path.length - 1]] = value;
    }
  }
  return draft;
}

// changed must follow any change to state.config or state.edits.
function changed() {
  draft = null;
}

const problemsUnder = (prefix) =>
  state.problems.filter((p) => prefix.every((k, i) => p.path[i] === k));
const sameSel = (a, b) => JSON.stringify(a) === JSON.stringify(b);

// selKey names the telemetry series of the selection: "gateway" or
// "gateway/downstream"; null when the selection has no series.
function runningSelection() {
  const s = state.sel;
  if (!s || s.kind === 'sim') return null;
  if (state.config.running_index_matches) return s;
  const gw = cfg().gateways[s.gw];
  const gi = state.running.gateways.findIndex(g => g.name === gw?.name);
  if (gi < 0) return null;
  if (s.kind === 'gw') return {kind:'gw',gw:gi};
  const di = state.running.gateways[gi].downstreams.findIndex(d => d.name === gw.downstreams[s.ds]?.name);
  return di < 0 ? null : {kind:'ds',gw:gi,ds:di};
}

function selKey() {
  if (!state.sel || state.sel.kind === 'sim') return null;
  const s = runningSelection();
  if (!s) return '__unmatched__';
  const gw = state.running.gateways[s.gw];
  return s.kind === 'ds' ? `${gw.name}/${gw.downstreams[s.ds].name}` : gw.name;
}

function configSelection(runtimeSel) {
  if (state.config.running_index_matches) return runtimeSel;
  if (runtimeSel.kind === 'sim') return cfg().simulations?.[runtimeSel.sim] ? runtimeSel : null;
  const gw = state.running.gateways[runtimeSel.gw];
  const gi = cfg().gateways.findIndex(g => g.name === gw.name);
  if (gi < 0) return null;
  if (runtimeSel.kind === 'gw') return {kind:'gw',gw:gi};
  const di = cfg().gateways[gi].downstreams.findIndex(d => d.name === gw.downstreams[runtimeSel.ds].name);
  return di < 0 ? null : {kind:'ds',gw:gi,ds:di};
}

function renderTree() {
  const item = (label, depth, sel, right = '', invalid = false) =>
    `<div class="node" role="treeitem" tabindex="0" aria-level="${depth + 1}" aria-selected="${sameSel(sel, state.sel)}"${invalid ? ' aria-invalid="true"' : ''} style="--d:${depth}" data-sel='${JSON.stringify(sel)}'>${label}<span class="r">${right}</span></div>`;
  const scrollTop = $('#side .tree-content')?.scrollTop || 0;
  let h = '<label class="search-label">搜索资源<input id="resource-search" placeholder="名称 / Slave ID" value="' + esc(state.search) + '"></label><div class="tree-content" role="tree" aria-label="资源"><div class="sec">网关</div>';
  (cfg().gateways || []).forEach((gw, gi) => {
    if (state.search && !JSON.stringify(gw).toLowerCase().includes(state.search.toLowerCase())) return;
    h += item(`<button class="collapse" data-collapse="${gi}" aria-expanded="${!state.collapsed.has(gi)}" aria-label="折叠或展开 ${esc(gw.name)}">${state.collapsed.has(gi) ? '▸' : '▾'}</button><b>${esc(gw.name)}</b>`, 0, { kind: 'gw', gw: gi });
    if (!state.collapsed.has(gi)) (gw.downstreams || []).forEach((ds, di) => {
      if (state.search && !gw.name.toLowerCase().includes(state.search.toLowerCase()) && !JSON.stringify(ds).toLowerCase().includes(state.search.toLowerCase())) return;
      const bad = problemsUnder(['gateways', gi, 'downstreams', di]).length > 0;
      h += item(`<span class="muted">→</span> ${esc(ds.name || `${ds.type}#${di}`)}`, 1, { kind: 'ds', gw: gi, ds: di },
        bad ? '<span class="err-txt" title="有校验错误">●</span>' : `<span class="faint mono">${esc(ds.slave_ids)}</span>`, bad);
    });
  });
  h += '<div class="sec">模拟从站</div>';
  (cfg().simulations || []).forEach((sim, si) => {
    if (state.search && !sim.name.toLowerCase().includes(state.search.toLowerCase())) return;
    h += item(esc(sim.name), 0, { kind: 'sim', sim: si }, `<span class="faint">${esc(sim.persistence?.type)}</span>`);
  });
  $('#side').innerHTML = h + '</div>';
  $('#side .tree-content').scrollTop = scrollTop;
  $('#resource-search').oninput = (e) => { state.search = e.target.value; const cursor = e.target.selectionStart; renderTree(); $('#resource-search').focus(); $('#resource-search').setSelectionRange(cursor,cursor); };
  for (const b of document.querySelectorAll('[data-collapse]')) b.onclick = (e) => { e.stopPropagation(); const i = Number(b.dataset.collapse); state.collapsed.has(i) ? state.collapsed.delete(i) : state.collapsed.add(i); renderTree(); };
  for (const n of document.querySelectorAll('#side [data-sel]')) {
    n.onclick = () => select(JSON.parse(n.dataset.sel));
    n.onkeydown = (e) => { if (e.target !== n) return; if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); n.click(); } };
  }
}

let fieldSeq = 0;
// field renders one input; with a path it is editable and records a "set"
// edit at that path in the config file.
function field(label, value, path) {
  const id = `f${++fieldSeq}`;
  path = state.config.schema_version === 1 && path && getIn(state.config.config, path) !== undefined ? path : null;
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
      ? simulationField(ds.simulation.ref, p('simulation', 'ref'))
      : ds.tcp ? field('目标地址', ds.tcp.address, p('tcp', 'address'))
      : ds.serial ? field('串口', ds.serial.device, p('serial', 'device')) : '';
    h = `<div class="h"><h2>${esc(ds.name || `${ds.type}#${s.ds}`)}</h2><span class="badge">${esc(ds.type)}</span><span class="muted">属于网关 ${esc(gw.name)}</span></div>
      <div class="cols"><div class="box"><h4>属性</h4>
        ${field('名称', ds.name, p('name'))}${field('类型', ds.type)}${field('Slave IDs', ds.slave_ids, p('slave_ids'))}${target}${ds.serial ? Object.entries(ds.serial).filter(([k]) => k !== 'device').map(([k,v]) => field(({baud_rate:'波特率',data_bits:'数据位',parity:'校验位',stop_bits:'停止位',timeout:'超时',rqst_pause:'请求间隔'})[k] || k, v, p('serial', k))).join('') : ''}
      <div class="small">支持修改已有字段；新增字段、增删对象请编辑 YAML。</div></div></div>${ds.simulation?.mappings ? `<div class="box"><h4>注入映射</h4>${ds.simulation.mappings.map((m) => `<div class="mono">${esc(m.source.table)} @${m.source.start_address} ×${m.source.count} → ${esc(m.target.table)} @${m.target.start_address}</div>`).join('')}</div>` : ''}`;
  } else if (s?.kind === 'gw') {
    const gw = cfg().gateways[s.gw];
    h = `<div class="h"><h2>${esc(gw.name)}</h2><span class="badge">配置详情</span></div><div class="box"><h4>上游监听配置（非监听健康状态）</h4>${(gw.upstreams || []).map((u) => field('协议', u.type) + field('监听地址 / 串口', u.tcp?.address || u.serial?.device)).join('')}</div><div class="box"><h4>Slave ID 路由表</h4><table class="t"><thead><tr><th>Slave IDs</th><th>下游</th><th>类型</th><th>目标</th></tr></thead><tbody>${(gw.downstreams || []).map((d, i) => `<tr><td>${esc(d.slave_ids)}</td><td><button data-route="${i}">${esc(d.name || d.type)}</button></td><td>${esc(d.type)}</td><td>${esc(d.simulation?.ref || d.tcp?.address || d.serial?.device)}</td></tr>`).join('')}</tbody></table></div>`;
  } else if (s?.kind === 'sim') {
    const sim = cfg().simulations[s.sim];
    const r = state.reg;
    h = `<div class="h"><h2>${esc(sim.name)}</h2><span class="badge" id="sim-status"></span>
        <span class="muted">${esc(sim.persistence?.type)}${sim.persistence?.path ? ' · ' + esc(sim.persistence.path) : ''}</span><span class="muted mono" id="sim-version"></span></div>
      <div class="regbar">
        <div class="seg">${TABLES.map(([t, l]) => `<button aria-pressed="${t === r.table}" data-table="${t}">${l}</button>`).join('')}</div>
        <span class="sp"></span>
        <button class="btn" data-step="-1" aria-label="上一屏">‹</button>
        <label class="muted" for="jump">起始地址 ${state.addressBase === 16 ? '0x' : '十进制'}</label><input id="jump" class="mono jump" value="${state.addressBase === 16 ? hex(r.start, 4) : r.start}"><button class="btn" id="address-base">${state.addressBase === 16 ? '切换十进制' : '切换十六进制'}</button>
        <button class="btn" data-step="1" aria-label="下一屏">›</button>
        <span class="muted mono" id="reg-range"></span>
      </div>
      <div class="small">协议地址从 0 开始；4x / 3x 是表类型，0 对应常用引用编号 40001 / 30001。<span id="reg-error" role="alert"></span></div><div class="box grid-box"><div id="grid" role="grid" aria-label="寄存器"></div></div>`;
  }
  h += `<div class="small">${state.edits.size || !state.config.running_matches ? '配置草稿 / 磁盘配置 · 运行中仍使用启动配置' : ''}</div>`;
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
  for (const b of document.querySelectorAll('[data-route]')) b.onclick = () => select({kind:'ds',gw:s.gw,ds:Number(b.dataset.route)});
  if ($('#address-base')) $('#address-base').onclick = () => { state.addressBase = state.addressBase === 16 ? 10 : 16; renderEditor(); };
  for (const input of document.querySelectorAll('#center [data-path]')) {
    input.oninput = () => setEdit(JSON.parse(input.dataset.path), input.value);
  }
  for (const b of document.querySelectorAll('#center [data-table]')) {
    b.onclick = () => { state.reg.table = b.dataset.table; state.reg.prev.clear(); renderEditor(); };
  }
  for (const b of document.querySelectorAll('#center [data-step]')) {
    b.onclick = () => moveRegisters(state.reg.start + Number(b.dataset.step) * state.reg.page);
  }
  const jump = $('#jump');
  if (jump) jump.onkeydown = (e) => { if (e.key === 'Enter') { const text = jump.value.trim(); const valid = state.addressBase === 16 ? /^[0-9a-f]{1,4}$/i.test(text) : /^\d+$/.test(text); const n = parseInt(text, state.addressBase); if (!valid || n > 65535) { $('#reg-error').textContent = '请输入 0–65535 范围内的有效地址'; jump.setAttribute('aria-invalid','true'); } else { $('#reg-error').textContent = ''; jump.removeAttribute('aria-invalid'); moveRegisters(n); } } };
  showFieldProblems();
  if (s?.kind === 'sim') { showSimStatus(); refreshRegisters(); }
}

const isWide = () => innerWidth >= 1920;

function selTitle() {
  const s = state.sel;
  if (!s) return '';
  if (s.kind === 'sim') return cfg().simulations[s.sim].name;
  const gw = cfg().gateways[s.gw];
  return s.kind === 'ds' ? gw.downstreams[s.ds].name || `${gw.downstreams[s.ds].type}#${s.ds}` : gw.name;
}

// topoGatewayIndex picks the gateway the topology shows: the selection's
// own, or for a simulation the first gateway with a downstream bound to it.
function topoGatewayIndex() {
  const s = state.sel, c = state.running, gws = c.gateways || [];
  if (!s) return 0;
  if (s.kind !== 'sim') return runningSelection()?.gw ?? -1;
  const name = cfg().simulations[s.sim]?.name;
  const refs = gws.map((g,i) => (g.downstreams || []).some((d) => d.simulation?.ref === name) ? i : -1).filter(i => i >= 0);
  return refs.includes(state.topoGW) ? state.topoGW : (refs[0] ?? -1);
}
const topoGateway = () => state.running.gateways?.[topoGatewayIndex()];

function renderTopology() {
  const el = $('#topo');
  const gi = topoGatewayIndex();
  const gw = state.running.gateways?.[gi];
  if (!el) return;
  if (!gw) { el.innerHTML = '<div class="pad muted">选中对象未匹配到运行中的网关或引用</div>'; return; }
  const sims = [...new Set((gw.downstreams || []).map((d) => d.simulation?.ref).filter(Boolean))];
  const W = Math.max(el.clientWidth, sims.length ? 600 : 460);
  const H = Math.max(el.clientHeight, 100 + Math.max(gw.upstreams.length,gw.downstreams.length,sims.length) * 90);
  const nw = Math.max(132, Math.min(200, W / (sims.length ? 5.2 : 4.2)));
  const cx = (sims.length ? [0.13, 0.37, 0.63, 0.87] : [0.17, 0.5, 0.83]).map((c) => W * c);
  const ys = (n, i) => H / 2 + 8 + (i - (n - 1) / 2) * Math.min((H - 40) / (n + 1), 130);
  const simIndex = (name) => cfg().simulations.findIndex((x) => x.name === name);
  const nodes = [];
  (gw.upstreams || []).forEach((u, i) => nodes.push({ id: `up${i}`, sel: { kind: 'gw', gw: gi }, x: cx[0], y: ys(gw.upstreams.length, i), k: `上游 · ${u.type}`, n: u.type === 'rtu' ? u.serial?.device ?? '' : u.tcp?.address ?? '' }));
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
  const on = nodes.find((n) => sameSel(configSelection(n.sel), state.sel) && !n.id.startsWith('up'))?.id;
  const half = nw / 2;
  const curve = (a, b) => { const mx = (a.x + b.x) / 2; return `M${a.x + half},${a.y} C${mx},${a.y} ${mx},${b.y} ${b.x - half},${b.y}`; };
  el.style.setProperty('--nw', `${nw}px`);
  el.innerHTML = `<div class="topology-note">运行拓扑 · 蓝色虚线：注入写入 ${state.sel?.kind === 'sim' ? `<select id="topology-gateway" aria-label="引用网关">${state.running.gateways.map((g,i) => (g.downstreams || []).some(d => d.simulation?.ref === cfg().simulations[state.sel.sim].name) ? `<option value="${i}" ${gi === i ? 'selected' : ''}>${esc(g.name)}</option>` : '').join('')}</select>` : ''}</div><svg aria-hidden="true" style="width:${W}px;height:${H}px">${edges.map(([a, b, key, inj]) => {
      const hi = on && (a === on || b === on);
      return `<path d="${curve(at[a], at[b])}" class="edge${hi ? ' hi' : ''}"/>
        <path d="${curve(at[a], at[b])}" class="flow${inj ? ' inj' : ''}" data-edge="${esc(key)}" style="opacity:${on && !hi ? 0.3 : 0.85}"/>
        <text x="${(at[a].x + at[b].x) / 2}" y="${(at[a].y + at[b].y) / 2 - 6}" text-anchor="middle" class="edge-label" ${inj ? '' : `data-edge-label="${esc(key)}"`}>${inj ? '注入写入' : ''}</text>`;
    }).join('')}</svg>
    ${(sims.length ? ['上游', '网关', '下游', '模拟从站'] : ['上游', '网关', '下游']).map((l, i) => `<div class="tcol" style="left:${cx[i]}px">${l}</div>`).join('')}
    ${nodes.map((n) => `<button class="tn${n.dark ? ' gw' : ''}" aria-pressed="${n.id === on}" data-sel='${JSON.stringify(configSelection(n.sel))}' ${configSelection(n.sel) ? '' : 'disabled'} style="left:${n.x}px;top:${n.y}px">
      <span class="k"><span>${esc(n.k)}</span>${n.bad ? '<span class="err-txt" title="配置草稿有冲突，运行配置未改变">● 草稿冲突</span>' : ''}</span>
      <span class="n">${esc(n.n)}</span><span class="s"${n.live ? ` data-node-live="${esc(n.live)}"` : ''}></span></button>`).join('')}`;
  for (const b of el.querySelectorAll('[data-sel]')) b.onclick = () => select(JSON.parse(b.dataset.sel));
  if ($('#topology-gateway')) $('#topology-gateway').onchange = (e) => { state.topoGW = Number(e.target.value); renderEditor(); };
  showTopologyTraffic();
}

// showTopologyTraffic refreshes edge rates in place on every metrics poll.
function showTopologyTraffic() {
  for (const t of document.querySelectorAll('[data-edge-label]')) t.textContent = state.online && state.metrics.has(t.dataset.edgeLabel) ? `${Math.round(state.rates.get(t.dataset.edgeLabel) ?? 0)} /s` : '—';
  for (const p of document.querySelectorAll('[data-edge]')) {
    const r = state.rates.get(p.dataset.edge) ?? 0;
    p.style.animationDuration = `${Math.max(0.25, 3 / Math.max(r, 1))}s`;
    p.style.animationPlayState = r > 0 && state.online && !state.paused ? 'running' : 'paused';
  }
  for (const n of document.querySelectorAll('[data-node-live]')) {
    const c = state.metrics.get(n.dataset.nodeLive);
    n.textContent = !c || state.online === false ? '运行数据未知' : `${Math.round(state.rates.get(n.dataset.nodeLive) ?? 0)} req/s · p99 ${c.requests ? fmtMs(c.p99_ms) : '—'}`;
  }
}

function moveRegisters(start) {
  state.reg.start = Math.min(65535, Math.max(0, start));
  state.reg.prev.clear();
  $('#jump').value = state.addressBase === 16 ? hex(state.reg.start, 4) : state.reg.start;
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
  } catch (err) {
    if (state.sel?.kind === 'sim' && cfg().simulations[state.sel.sim]?.name === name && $('#reg-error')) $('#reg-error').textContent = '寄存器读取失败，正在重试';
    return;
  }
  if (state.sel?.kind !== 'sim' || body.start !== r.start || body.table !== r.table || cfg().simulations[state.sel.sim]?.name !== name) return; // moved on meanwhile
  r.page = Math.max(1, Math.min(2048, rows * cols));
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
      h += `<div class="reg${changed ? ' hl' : ''}${v ? '' : ' zero'}" role="gridcell" tabindex="0" data-address="${addr}" data-value="${v}" aria-label="地址 ${addr}">${bits ? (v ? '●' : '○') : v}</div>`;
    }
    h += '</div>';
  }
  el.innerHTML = h;
  for (const cell of el.querySelectorAll('[data-address]')) { cell.onclick = () => showText('寄存器详情', `协议地址：${cell.dataset.address} / 0x${hex(Number(cell.dataset.address),4)}\n表：${r.table}\n值：${cell.dataset.value}`); cell.onkeydown = (e) => { if (e.key === 'Enter') cell.click(); }; }
  for (const b of document.querySelectorAll('[data-step]')) b.disabled = b.dataset.step === '-1' ? r.start === 0 : r.start + count >= 65536;
  $('#reg-range').textContent = `${hex(r.start, 4)}–${hex(r.start + body.values.length - 1, 4)}`;
}

// showFieldProblems marks the editor's fields in place, so typing keeps focus.
function showFieldProblems() {
  for (const input of document.querySelectorAll('#center [data-path]')) {
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
    <polyline points="0,${h} ${pts} ${w},${h}" class="spark-area"/><polyline points="${pts}" class="spark-line" vector-effect="non-scaling-stroke"/></svg>`;
}

function renderInspector() {
  if (state.sel?.kind === 'sim') { const name = cfg().simulations[state.sel.sim].name; const st = state.status?.simulations?.find(x => x.name === name); $('#insp').innerHTML = `<div class="insp-h">模拟从站 · ${esc(name)}</div><div>状态：${esc(st?.status || '未知')}</div><div>数据版本：${st?.version ?? '—'}</div><div>引用下游</div>${state.running.gateways.flatMap(g => (g.downstreams || []).filter(d => d.simulation?.ref === name).map(d => `<div>${esc(g.name)}/${esc(d.name)}</div>`)).join('') || '<div class="muted">无运行中引用</div>'}`; return; }
  const key = selKey();
  if (key === '__unmatched__') { $('#insp').innerHTML = '<div class="pad muted">此配置对象未匹配到运行实例；重启后才能查看对应指标。</div>'; return; }
  const { counts, rate, hist } = seriesFor(key);
  const c = counts ?? { requests: 0, errors: 0, p50_ms: 0, p99_ms: 0 };
  const errRate = c.requests ? (c.errors / c.requests * 100).toFixed(2) : '0.00';
  const kpi = (label, value) => `<div><b aria-label="${label}">${!counts || state.online === false ? '—' : value}</b><span>${label}</span></div>`;
  $('#insp').innerHTML = `<div class="insp-h">运行指标 <span class="muted">· ${esc(key ?? '全部')}</span></div>
    <div class="kpi">${kpi('请求/秒', Math.round(rate))}${kpi('错误率', `${errRate}%`)}${kpi('p50', c.requests ? fmtMs(c.p50_ms) : '—')}${kpi('p99', c.requests ? fmtMs(c.p99_ms) : '—')}</div>
    ${key ? `<div><div class="muted small">请求/秒 · 近 40 秒</div>${spark(hist, 52)}</div>` : ''}
    <dl class="totals"><dt>累计请求</dt><dd class="mono" aria-label="累计请求">${counts && state.online !== false ? c.requests : '—'}</dd><dt>累计错误</dt><dd class="mono" aria-label="累计错误">${counts && state.online !== false ? c.errors : '—'}</dd></dl>
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
function logMatches(e, key) {
  if (!key && state.sel?.kind === 'sim' && state.logScope === 'selected') {
    const name = cfg().simulations[state.sel.sim]?.name;
    const gateway = state.running.gateways.find(g => g.name === e.gateway);
    return gateway?.downstreams.some(d => d.name === e.downstream && d.simulation?.ref === name) || false;
  }
  return !key || (key.includes('/') ? `${e.gateway}/${e.downstream}` === key : e.gateway === key);
}

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
  const key = state.logScope === 'gateway' && state.sel?.gw != null ? state.running.gateways[runningSelection()?.gw]?.name ?? '__unmatched__' : selKey();
  let cols = LOG_COLS;
  for (const lvl of [4, 3, 2]) {
    if (cols.reduce((w, c) => w + c[1] * K, 150) > el.clientWidth) cols = cols.filter((c) => c[2] < lvl);
  }
  const rows = (state.paused ? state.frozenLog : state.log).filter(e => logMatches(e,key) && (!state.errorsOnly || e.error) && (!state.slaveFilter || String(e.slave_id) === state.slaveFilter) && (!state.fcFilter || String(e.function_code) === state.fcFilter)).slice(0, state.paused ? LOG_CAP : fit(el, ROW_H));
  el.innerHTML = `<table class="t" aria-label="请求日志"><colgroup>${cols.map((c) => `<col style="${c[1] ? `width:${Math.round(c[1] * K)}px` : ''}">`).join('')}</colgroup>
    <thead><tr>${cols.map((c) => `<th>${c[0]}</th>`).join('')}</tr></thead>
    <tbody>${rows.map((e,i) => `<tr tabindex="0" data-log-row="${i}"${e.error ? ' class="bad"' : ''}>${cols.map((c) => `<td>${c[3](e)}</td>`).join('')}</tr>`).join('')}</tbody></table>`;
  for (const tr of el.querySelectorAll('[data-log-row]')) { tr.onclick = () => showText('请求详情', JSON.stringify(rows[Number(tr.dataset.logRow)], null, 2)); tr.onkeydown = (e) => { if (e.key === 'Enter') tr.click(); }; }
}

let logFrame = 0;
function scheduleLog() {
  if (!logFrame) logFrame = requestAnimationFrame(() => { logFrame = 0; renderLog(); });
}

function subscribeEvents() {
  const es = new EventSource('/api/v1/events');
  es.onerror = () => { state.eventsOnline = false; setOnline(state.online); };
  es.onopen = () => { state.eventsOnline = true; setOnline(state.online); };
  es.onmessage = (msg) => {
    const batch = JSON.parse(msg.data);
    state.log = [...batch.reverse(), ...state.log].slice(0, LOG_CAP);
    if (!state.paused) scheduleLog();
  };
}

// pollMetrics derives req/s from the change in cumulative counts between
// two polls; the server keeps no rate windows.
let lastPoll = 0;
async function pollMetrics() {
  if (state.polling) return;
  state.polling = true;
  try {
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
  state.lastUpdate = new Date();
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
    const rate = prev && dt ? Math.max(0, c.requests - prev.requests) / dt : 0;
    state.rates.set(key, rate);
    state.history.set(key, [...(state.history.get(key) ?? []), rate].slice(-40));
  }
  state.metrics = next;
  if (!state.paused) { renderInspector(); showTopologyTraffic(); }
  try { state.status = (await api('/api/v1/status')).body; } catch { return; }
  if (!state.paused) { showSimStatus(); refreshRegisters(); }
  if (state.status.startup_revision !== state.startupRevision) { state.running = (await api('/api/v1/running-config')).body; state.startupRevision = state.status.startup_revision; state.metrics.clear(); state.history.clear(); renderEditor(); }
  if (state.saving) return;
  const revision = state.config.revision;
  const current = (await api('/api/v1/config')).body;
  if (state.saving || state.config.revision !== revision) return;
  if (current.revision !== state.config.revision) { state.stale = true; renderToolbar(); }
  else if (current.running_matches !== state.config.running_matches) { state.config.running_matches = current.running_matches; renderToolbar(); }
  } catch (err) { state.notice = err.message; renderToolbar(); } finally { state.polling = false; }
}

function renderDock() {
  const key = state.logScope === 'gateway' && runningSelection() ? state.running.gateways[runningSelection().gw].name : selKey();
  const n = state.problems.length;
  const tab = (id, label) => `<button role="tab" aria-selected="${state.dockTab === id}" data-dock="${id}">${label}</button>`;
  $('#dock').innerHTML = `<div class="dock-tabs" role="tablist">
      ${tab('log', '请求日志')}${tab('problems', `问题${n ? ` <span class="badge err">${n}</span>` : ''}`)}
      <span class="sp"></span><span class="faint">${state.dockTab === 'log' ? (key ? `筛选：${esc(key)}` : state.sel?.kind === 'sim' && state.logScope === 'selected' ? `关联模拟从站：${esc(cfg().simulations[state.sel.sim].name)}` : '全部网关') : ''}</span>
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
    body.innerHTML = `<div class="log-controls"><button class="btn" id="pause-log">${state.paused ? '恢复实时' : '暂停显示'}</button><label><input id="errors-only" type="checkbox" ${state.errorsOnly ? 'checked' : ''}>仅看错误</label><select id="log-scope" aria-label="日志范围"><option value="selected">选中对象</option><option value="gateway" ${state.logScope === 'gateway' ? 'selected' : ''}>${state.sel?.kind === 'sim' ? '全部网关' : '整个网关'}</option></select><input id="slave-filter" aria-label="筛选 Slave ID" placeholder="Slave ID" value="${esc(state.slaveFilter)}"><input id="fc-filter" aria-label="筛选功能码" placeholder="功能码（十进制）" value="${esc(state.fcFilter)}"><span class="small">${state.paused ? '显示已暂停，转发仍在继续' : '最近请求样本'}</span></div><div class="fill ${state.paused ? 'scroll' : ''}" id="log"></div>`;
    $('#pause-log').onclick = togglePause;
    $('#errors-only').onchange = (e) => { state.errorsOnly = e.target.checked; renderLog(); };
    $('#log-scope').onchange = (e) => { state.logScope = e.target.value; renderDock(); };
    for (const [id,key] of [['slave-filter','slaveFilter'],['fc-filter','fcFilter']]) $('#'+id).oninput = (e) => { state[key] = e.target.value; renderLog(); };
    renderLog();
  } else {
    body.innerHTML = state.problems.length
      ? `<table class="t" aria-label="问题"><tbody>${state.problems.map((p,i) => `<tr data-problem="${i}" tabindex="0"><td class="err-txt" style="width:24px">✕</td><td>${esc(p.path.join(' → '))}：${esc(p.message)}</td></tr>`).join('')}</tbody></table>`
      : '<div class="muted pad">没有发现问题</div>';
    body.classList.add('scroll');
    for (const row of body.querySelectorAll('[data-problem]')) { row.onclick = () => locateProblem(state.problems[Number(row.dataset.problem)]); row.onkeydown = (e) => { if (e.key === 'Enter') row.click(); }; }
  }
}

function renderToolbar() {
  const n = state.edits.size;
  const reason = state.config.schema_version !== 1 ? '旧版配置仅支持查看，需手动迁移至 version: 1' : state.saving ? '正在保存…' : state.validating ? '正在校验…' : state.stale ? '文件已被外部修改' : state.validationFailed ? '校验失败，请重试' : state.problems.length ? `存在 ${state.problems.length} 个问题` : !n ? '没有未保存修改' : '';
  const canSave = n > 0 && !reason;
  const file = state.status?.config_path?.split(/[\\/]/).pop() || '配置文件';
  $('#toolbar').innerHTML = `<span class="file" title="${esc(state.status?.config_path)}">${esc(file)}${n ? ' •' : ''}</span>
    <button class="btn ${canSave ? 'pri' : ''}" id="save" title="${esc(reason)}" ${canSave ? '' : 'disabled'}>保存 <span class="kbd">${navigator.platform.includes('Mac') ? '⌘' : 'Ctrl'}+S</span></button>
    ${n ? '<button class="btn" id="changes">查看修改</button><button class="btn" id="discard">放弃修改</button>' : ''}
    ${!state.config.running_matches ? '<button class="btn" id="apply-help">如何生效</button>' : ''}
    <span class="sp"></span>
    ${state.stale ? '<span class="stale" role="alert">配置文件已在别处被修改，当前修改无法保存。<button class="btn" id="copy-edits">复制修改</button><button class="btn" id="reload">重新加载</button></span>' : ''}
    ${n ? `<span class="badge warn">${n} 处未保存修改 · 保存后需重启生效</span>` : state.config.running_matches ? '<span class="badge">配置与运行中一致</span>' : '<span class="badge warn">已保存，重启网关后生效 · 运行中仍使用旧配置</span>'}
    ${reason && n || state.config.schema_version !== 1 ? `<span class="small">${esc(reason)}</span>` : ''}
    ${state.validationFailed ? '<button class="btn" id="retry-validation">重新校验</button>' : ''}
    ${state.notice ? `<span class="notice" role="alert">${esc(state.notice)}</span>` : ''}`;
  $('#save').onclick = save;
  if ($('#retry-validation')) $('#retry-validation').onclick = () => { state.validating = true; state.validationFailed = false; renderToolbar(); validate(); };
  if ($('#reload')) $('#reload').onclick = reload;
  if ($('#changes')) $('#changes').onclick = () => showText('未保存修改', editsText());
  if ($('#copy-edits')) $('#copy-edits').onclick = () => copyText(editsText());
  if ($('#discard')) $('#discard').onclick = async () => { if (await ask('放弃修改', '将丢弃所有未保存修改。', ['放弃修改','取消']) === '放弃修改') { dropEdits(); render(); } };
  if ($('#apply-help')) $('#apply-help').onclick = () => showText('使配置生效', '文件已保存，运行中仍使用旧配置。请通过实际部署方式重启服务：systemd、Docker 或启动网关的终端。连接恢复后将自动核对配置状态。');
  for (const input of document.querySelectorAll('[data-path]')) input.disabled = state.saving;
}

function dropEdits() {
  clearTimeout(validateTimer);
  validateSeq++;
  state.edits.clear(); changed(); state.problems = []; state.validating = false; state.validationFailed = false;
}

async function reload() {
  if (state.edits.size && await ask('重新加载配置', '将丢弃未保存修改。可先取消并复制修改内容。', ['丢弃并重新加载','取消']) !== '丢弃并重新加载') return false;
  try {
    const next = (await api('/api/v1/config')).body;
    dropEdits(); if (next.schema_version !== 1) next.config = structuredClone(state.running); state.config = next; state.stale = false; state.notice = ''; ensureSelection(); render(); return true;
  } catch (err) { state.notice = `重新加载失败：${err.message}`; renderToolbar(); return false; }
}

function ensureSelection() {
  const c = cfg(), s = state.sel;
  if (!s || (s.kind === 'sim' ? !c.simulations?.[s.sim] : !c.gateways?.[s.gw] || s.kind === 'ds' && !c.gateways[s.gw].downstreams?.[s.ds])) state.sel = c.gateways?.length ? {kind:'gw',gw:0} : c.simulations?.length ? {kind:'sim',sim:0} : null;
}

async function loadConfig() {
  state.config = (await api('/api/v1/config')).body;
  changed();
}

async function save() {
  if (state.saving || !state.edits.size || state.stale) return false;
  clearTimeout(validateTimer); validateSeq++;
  state.saving = true; state.notice = ''; renderToolbar();
  const edits = [...state.edits.values()];
  try {
    const { status, body } = await api('/api/v1/config', {method:'PUT', body:JSON.stringify({base_revision:state.config.revision, edits})});
    if (status === 409) { state.stale = true; return false; }
    if (status === 422) { state.problems = body.problems ?? []; state.notice = body.error || '保存失败，请修复配置问题'; state.dockTab = 'problems'; return false; }
    // Inputs are locked during saving; do not clear edits until the write is confirmed.
    dropEdits(); state.config.revision = body.revision;
    for (const edit of edits) getIn(state.config.config, edit.path.slice(0,-1))[edit.path[edit.path.length-1]] = edit.value;
    state.config.running_matches = false; changed(); state.notice = '文件已保存'; return true;
  } catch (err) { state.notice = `保存失败：${err.message}`; return false; }
  finally { state.saving = false; render(); }
}

// setEdit records (or, when the value is back to the saved one, drops) an
// edit, then revalidates the whole draft shortly after typing stops.
let validateTimer;
function setEdit(path, value) {
  if (typeof getIn(state.config.config,path) === 'boolean' && /^(true|false)$/.test(value)) value = value === 'true';
  if (typeof getIn(state.config.config,path) === 'number' && value.trim() && Number.isFinite(Number(value))) value = Number(value);
  if (getIn(state.config.config, path) === value) state.edits.delete(pathKey(path));
  else state.edits.set(pathKey(path), { op: 'set', path, value });
  changed();
  validateSeq++; state.validationFailed = false; state.notice = "";
  state.validating = true;
  renderToolbar();
  clearTimeout(validateTimer);
  validateTimer = setTimeout(validate, 300);
}

let validateSeq = 0;
async function validate() {
  const seq = ++validateSeq;
  try {
    const {status, body} = await api('/api/v1/config/validate', {method:'POST', body:JSON.stringify({base_revision:state.config.revision, edits:[...state.edits.values()]})});
    if (seq !== validateSeq) return;
    state.stale = status === 409; state.problems = body.problems ?? []; state.notice = body.error && status !== 409 ? body.error : '';
    state.validationFailed = status === 422 && !body.problems;
  } catch (err) { if (seq !== validateSeq) return; state.validationFailed = true; state.notice = `校验失败：${err.message}`; }
  finally {
    if (seq === validateSeq) { state.validating = false; renderTree(); renderToolbar(); renderDock(); showFieldProblems(); renderTopology(); }
  }
}

function setOnline(online) {
  state.online = online;
  $('#status').innerHTML = online
    ? '<span><span class="dot"></span> 管理连接正常（不代表 Modbus 监听健康）</span><span class="sp"></span>'
    : '<span role="alert"><span class="dot warn"></span> 管理连接已断开，正在重试… · 数据已过期</span><span class="sp"></span>';
  $('#status').innerHTML += `<span>${state.lastUpdate ? '最后更新 ' + state.lastUpdate.toLocaleTimeString() : '等待数据'}</span>${state.eventsOnline === false ? '<span>请求日志连接已断开，正在重试</span>' : ''}`;
  if (!online) renderInspector();
}

function render() {
  renderToolbar();
  renderTree();
  renderEditor();
  renderInspector();
  renderDock();
  for (const input of document.querySelectorAll('[data-path]')) input.disabled = state.saving;
}

function select(sel) {
  if (!sel) return;
  if (sel.gw != null) state.collapsed.delete(sel.gw);
  state.sel = sel; state.topoGW = null; state.reg.prev.clear();
  render();
}

async function start() {
  await loadConfig();
  state.running = (await api('/api/v1/running-config')).body;
  if (state.config.schema_version !== 1) { state.config.config = structuredClone(state.running); changed(); }
  state.status = (await api('/api/v1/status')).body;
  state.startupRevision = state.status.startup_revision;
  addEventListener('keydown', (e) => {
    if (e.code === 'Space' && !e.defaultPrevented && !e.target.closest('input,textarea,select,button,[contenteditable],dialog')) { e.preventDefault(); togglePause(); }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
      e.preventDefault();
      if (e.target.closest('dialog')) return; // not the browser's "save page"
      if (!$('#save')?.disabled) save();
    }
  });
  ensureSelection();
  render();
  subscribeEvents();
  addEventListener('beforeunload', (e) => { if (state.edits.size) { e.preventDefault(); e.returnValue = ''; } });
  pollMetrics();
  setInterval(pollMetrics, 1000);
  let wide = isWide();
  addEventListener('resize', () => {
    scheduleLog();
    renderInspector();
    if (isWide() !== wide) { wide = isWide(); renderEditor(); } else renderTopology();
  });
}

start().catch(err => { $('#center').innerHTML = `<div class="pad" role="alert">控制台加载失败：${esc(err.message)}<button class="btn" id="retry-start">重新加载</button></div>`; $('#retry-start').onclick = () => location.reload(); });

function togglePause() {
  state.paused = !state.paused;
  if (state.paused) state.frozenLog = state.log.slice();
  renderDock(); showTopologyTraffic();
}

function ask(title, message, choices) {
  return new Promise(resolve => {
    const dialog = document.createElement('dialog');
    dialog.innerHTML = `<h2>${esc(title)}</h2><p>${esc(message)}</p><div class="dialog-actions">${choices.map(c => `<button class="btn" data-choice="${esc(c)}">${esc(c)}</button>`).join('')}</div>`;
    document.body.append(dialog); dialog.showModal();
    const finish = choice => { dialog.close(); dialog.remove(); resolve(choice); };
    dialog.oncancel = e => { e.preventDefault(); finish('取消'); };
    for (const b of dialog.querySelectorAll('[data-choice]')) b.onclick = () => finish(b.dataset.choice);
    dialog.querySelector('button:last-child').focus();
  });
}

function editsText() {
  return [...state.edits.values()].map(e => `${e.path.join(' → ')}: ${JSON.stringify(getIn(state.config.config,e.path))} → ${JSON.stringify(e.value)}`).join('\n');
}

async function copyText(text) {
  try { await navigator.clipboard.writeText(text); }
  catch { const area = document.createElement('textarea'); area.value = text; document.body.append(area); area.select(); const copied = document.execCommand('copy'); area.remove(); if (!copied) showText('请手动复制', text); }
}

function showText(title, text) {
  const dialog = document.createElement('dialog');
  dialog.innerHTML = `<h2>${esc(title)}</h2><pre>${esc(text)}</pre><div class="dialog-actions"><button class="btn" data-copy>复制</button><button class="btn" data-close>关闭</button></div>`;
  document.body.append(dialog); dialog.showModal();
  dialog.querySelector('[data-copy]').onclick = () => copyText(text);
  dialog.querySelector('[data-close]').onclick = () => dialog.close();
  dialog.onclose = () => dialog.remove();
}

function locateProblem(problem) {
  const p = problem.path;
  if (p[0] === 'gateways') state.sel = typeof p[3] === 'number' && p[2] === 'downstreams' ? {kind:'ds',gw:p[1],ds:p[3]} : {kind:'gw',gw:p[1]};
  else if (p[0] === 'simulations') state.sel = {kind:'sim',sim:p[1]};
  state.view = 'detail'; ensureSelection(); render();
  const input = [...document.querySelectorAll('[data-path]')].find(el => el.dataset.path === pathKey(p));
  if (input) { input.focus(); input.scrollIntoView({block:'nearest'}); }
}

$('#compact-mode').onclick = () => { document.body.classList.add('compact-mode'); $('#app').classList.add('compact'); renderEditor(); };
$('#toggle-inspector').onclick = () => { $('#app').classList.toggle('hide-inspector'); renderEditor(); };
$('#maximize-pane').onclick = () => { $('#app').classList.toggle('maximize'); renderEditor(); };
$('#reset-layout').onclick = () => { $('#app').classList.remove('hide-inspector','maximize','compact'); document.body.classList.remove('compact-mode'); document.documentElement.style.removeProperty('--side-w'); document.documentElement.style.removeProperty('--dock-h'); renderEditor(); };
for (const [cls, prop, axis] of [['resize-side','--side-w','x'],['resize-dock','--dock-h','y']]) {
  const handle = document.createElement('div'); handle.className = cls; handle.title = '拖动调整，双击恢复默认'; $('.main').append(handle);
  handle.ondblclick = () => { document.documentElement.style.removeProperty(prop); renderEditor(); };
  handle.onpointerdown = e => {
    handle.setPointerCapture(e.pointerId);
    handle.onpointermove = event => { const rect = $('.main').getBoundingClientRect(); const value = axis === 'x' ? Math.min(400,Math.max(180,event.clientX)) : Math.min(rect.height - 150,Math.max(140,rect.bottom-event.clientY)); document.documentElement.style.setProperty(prop, value+'px'); scheduleLog(); renderTopology(); };
    handle.onpointerup = () => { handle.onpointermove = null; renderEditor(); };
  };
}

function simulationField(value, path) {
  if (state.config.schema_version !== 1) return field('模拟从站', value);
  const names = (cfg().simulations || []).map(sim => sim.name);
  if (!names.includes(value)) names.unshift(value);
  return `<div class="field"><label for="simulation-ref">模拟从站</label><select id="simulation-ref" data-path='${JSON.stringify(path)}'>${names.map(name => `<option ${name === value ? 'selected' : ''}>${esc(name)}</option>`).join('')}</select></div><div class="field-err" data-err-for='${JSON.stringify(path)}'></div>`;
}
