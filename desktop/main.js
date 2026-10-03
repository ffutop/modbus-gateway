// ModMux desktop: the management console in a native window, backed by the
// bundled modbus-gateway binary running as a child process (design 6).
'use strict';

const { app, BrowserWindow, Menu, dialog, ipcMain, session, shell } = require('electron');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { ConfigFiles } = require('./config-files');
const { GatewayProcess } = require('./gateway-process');

// Tests and portable setups can isolate settings; must precede app ready.
if (process.env.MODMUX_USER_DATA) app.setPath('userData', process.env.MODMUX_USER_DATA);

// One instance per user: a second launch would fight over the same Modbus
// ports and serial devices, so it just brings the running window forward.
if (!app.requestSingleInstanceLock()) app.exit(0);
app.on('second-instance', () => {
  if (!win || win.isDestroyed()) return;
  if (win.isMinimized()) win.restore();
  win.show();
  win.focus();
});

function gatewayBinary() {
  if (process.env.MODMUX_GATEWAY_BIN) return process.env.MODMUX_GATEWAY_BIN;
  const exe = process.platform === 'win32' ? 'modbus-gateway.exe' : 'modbus-gateway';
  return path.join(process.resourcesPath, 'bin', exe);
}

const gateway = new GatewayProcess(gatewayBinary());
let win = null;
let quitting = false; // set once the user confirmed and the gateway stopped
let configFiles = null; // created once the app (and its paths) are ready
let currentConfig = null;
let dirty = false;
let recoveryDraft = null;
let transitioning = false;
let guardId = 0;
const guards = new Map();
ipcMain.on('config:dirty', (event, value, draft) => { if (event.sender === win?.webContents) { dirty = !!value; recoveryDraft = dirty ? draft : null; } });
ipcMain.handle('config:draft', () => recoveryDraft);
ipcMain.on('config:guard-result', (event, id, allowed) => { if (event.sender === win?.webContents) guards.get(id)?.(allowed); });
ipcMain.handle('config:open', () => chooseConfig());

async function guardEdits(action = 'continue') {
  if (!dirty) return true;
  if (!win || win.isDestroyed()) return false;
  if (win.webContents.getURL().startsWith('file:')) {
    const buttons = action !== 'continue' ? ['放弃修改','取消'] : ['保留草稿并继续','放弃修改','取消'];
    const {response} = await dialog.showMessageBox(win,{type:'question',buttons,defaultId:buttons.length-1,cancelId:buttons.length-1,message:'存在未保存草稿',detail: action === 'quit' ? '退出应用会丢弃草稿。可先在故障页复制诊断信息。' : '网关停止前的草稿已暂存，重新启动后可以继续编辑。'});
    if (buttons[response] === '放弃修改') { dirty = false; recoveryDraft = null; return true; }
    return buttons[response] === '保留草稿并继续';
  }
  const id = ++guardId;
  return new Promise(resolve => {
    const timer = setTimeout(() => finish(false), 120000);
    const finish = allowed => { clearTimeout(timer); guards.delete(id); resolve(allowed); };
    guards.set(id, finish); win.webContents.send('config:guard', id);
  });
}

function phase(text) { if (win && !win.isDestroyed()) win.webContents.send('gateway:phase', text); }

ipcMain.handle('gateway:output', () => gateway.output);
ipcMain.handle('gateway:restart', () => restartGateway());
gateway.on('crashed', ({ code, signal }) => {
  showStopped({ title: '网关已停止', reason: signal ? `被信号 ${signal} 终止` : `退出码 ${code}` });
});
// Output is forwarded in batches: a busy gateway can log a line per request
// (e.g. an unreachable downstream), far more often than the window repaints.
const OUTPUT_FLUSH_MS = 100;
let pendingOutput = [];
let outputTimer = null;
gateway.on('output', (line) => {
  pendingOutput.push(line);
  outputTimer ??= setTimeout(() => {
    outputTimer = null;
    const lines = pendingOutput;
    pendingOutput = [];
    if (win && !win.isDestroyed()) win.webContents.send('gateway:output', lines);
  }, OUTPUT_FLUSH_MS);
});

// withToken adds the gateway's token to every request the window makes to
// it, so the page never handles the token itself.
function withToken(url) {
  session.defaultSession.webRequest.onBeforeSendHeaders({ urls: [`${url}/*`] }, (details, callback) => {
    details.requestHeaders.Authorization = `Bearer ${gateway.token}`;
    callback({ requestHeaders: details.requestHeaders });
  });
}

async function createWindow() {
  win = new BrowserWindow({
    width: 1440,
    height: 900,
    minWidth: 1280,
    minHeight: 720,
    useContentSize: true, // sizes are the page area, excluding the title bar
    title: 'ModMux',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
    },
  });
  // The window only ever shows the gateway's console or our local pages;
  // web links go to the system browser, anything else is refused.
  const stoppedPage = pathToFileURL(path.join(__dirname, 'stopped.html')).href;
  const isOwn = (url) => (gateway.url && url.startsWith(`${gateway.url}/`)) || url.split('?')[0] === stoppedPage;
  const openOutside = (url) => { if (/^https?:\/\//i.test(url)) shell.openExternal(url); };
  win.webContents.on('will-navigate', (event, url) => {
    if (isOwn(url)) return;
    event.preventDefault();
    openOutside(url);
  });
  win.webContents.setWindowOpenHandler(({ url }) => {
    openOutside(url);
    return { action: 'deny' };
  });

  win.on('close', (event) => {
    if (quitting) return;
    if (!gateway.running && !dirty) { // nothing to stop, nothing to confirm
      quitting = true;
      app.quit();
      return;
    }
    event.preventDefault();
    confirmQuit();
  });
  await openConsole();
}

// restartGateway stops the gateway gracefully and starts it again on the
// current config, e.g. to apply a saved change.
async function restartGateway() {
  if (transitioning) return;
  transitioning = true;
  try {
    if (!await guardEdits()) return;
    if (gateway.running) {
      const {response} = await dialog.showMessageBox(win, {type:'question', buttons:['重启网关','取消'], defaultId:1, cancelId:1, message:'重启将短暂中断 Modbus 转发', detail:'将停止当前进程，并使用磁盘上已保存的配置重新启动。'});
      if (response !== 0) return;
    }
    phase('正在停止网关…'); await gateway.stop(); dirty = false;
    phase('正在启动网关…'); await openConsole();
  } finally { transitioning = false; }
}

async function openConfig(file) {
  if (transitioning) return;
  transitioning = true;
  try {
    if (!await guardEdits('switch')) return;
    if (gateway.running) {
      const {response} = await dialog.showMessageBox(win, {type:'question', buttons:['切换配置','取消'], defaultId:1, cancelId:1, message:'切换配置将中断当前 Modbus 转发', detail:file});
      if (response !== 0) return;
    }
    phase('正在停止网关…'); await gateway.stop();
    currentConfig = file; dirty = false; configFiles.remember(file); buildMenu();
    phase('正在启动网关…'); await openConsole();
  } finally { transitioning = false; }
}

async function chooseConfig() {
  const { canceled, filePaths } = await dialog.showOpenDialog(win, {
    title: '打开配置',
    properties: ['openFile'],
    filters: [{ name: 'YAML', extensions: ['yaml', 'yml'] }, { name: '所有文件', extensions: ['*'] }],
  });
  if (!canceled && filePaths.length) await openConfig(filePaths[0]);
}

function buildMenu() {
  const isMac = process.platform === 'darwin';
  const recent = configFiles.recent.map((file) => ({ label: file, click: () => openConfig(file) }));
  Menu.setApplicationMenu(Menu.buildFromTemplate([
    ...(isMac ? [{ role: 'appMenu' }] : []),
    {
      label: '文件',
      submenu: [
        { id: 'open-config', label: '打开配置…', accelerator: 'CmdOrCtrl+O', click: chooseConfig },
        { id: 'recent-configs', label: '最近打开', submenu: recent.length ? recent : [{ label: '（无）', enabled: false }] },
        { type: 'separator' },
        { label: '重启网关', click: () => restartGateway() },
        { type: 'separator' },
        isMac ? { role: 'close' } : { role: 'quit', label: '退出' },
      ],
    },
    { role: 'editMenu', label: '编辑' }, // copy/paste in form fields
    {
      label: '视图',
      submenu: [
        { label: '详情', accelerator: 'CmdOrCtrl+1', click: () => win?.webContents.send('view', 'detail') },
        { label: '拓扑', accelerator: 'CmdOrCtrl+2', click: () => win?.webContents.send('view', 'topo') },
        { type: 'separator' },
        { label: '重新载入', accelerator: 'CmdOrCtrl+R', click: async () => { if (!transitioning && await guardEdits()) { dirty = false; win.webContents.reload(); } } },
        { role: 'togglefullscreen', label: '全屏' },
      ],
    },
    { role: 'windowMenu', label: '窗口' },
  ]));
}

// showStopped replaces the console (served by the gateway, so gone with it)
// with a local page explaining why, the last output, and a restart button.
function showStopped({ title, reason }) {
  if (!win || win.isDestroyed()) return;
  dirty = !!recoveryDraft;
  const data = JSON.stringify({ title, reason, output: gateway.output.slice(-40), configPath: currentConfig, hasDraft: !!recoveryDraft, draft: recoveryDraft?.edits });
  win.loadFile(path.join(__dirname, 'stopped.html'), { query: { data } });
}

// confirmQuit asks before stopping the gateway (closing stops forwarding),
// then stops it gracefully and quits. Window close and Cmd+Q both land here.
let confirming = false;
async function confirmQuit() {
  if (confirming) return;
  confirming = true;
  if (transitioning || !await guardEdits('quit')) { confirming = false; return; }
  if (!gateway.running) { quitting = true; confirming = false; app.quit(); return; }
  const buttons = ['停止并退出', '取消'];
  const { response } = await dialog.showMessageBox(win, {
    type: 'question',
    buttons,
    defaultId: 1,
    cancelId: 1,
    message: '关闭窗口会停止网关',
    detail: '网关停止后 Modbus 转发随之中断。需要长期运行请使用服务端版本。',
  });
  confirming = false;
  if (buttons[response] !== '停止并退出') return;
  quitting = true;
  await gateway.stop();
  app.quit();
}

// openConsole starts the gateway on the current config and shows its
// console. A restarted gateway listens on a new port with a new token.
async function openConsole() {
  let url;
  try {
    url = await gateway.start(currentConfig);
  } catch (err) {
    showStopped({ title: '网关启动失败', reason: err.message });
    return;
  }
  withToken(url);
  dirty = !!recoveryDraft;
  await win.loadURL(`${url}/`);
}

app.whenReady().then(() => {
  configFiles = new ConfigFiles(app.getPath('userData'));
  currentConfig = configFiles.initial();
  if (!process.env.MODMUX_CONFIG) configFiles.remember(currentConfig);
  buildMenu();
  return createWindow();
});

app.on('before-quit', (event) => {
  if (quitting || !win || win.isDestroyed() || !gateway.running && !dirty) return;
  event.preventDefault();
  confirmQuit();
});
