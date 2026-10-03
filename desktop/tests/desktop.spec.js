const { test, expect } = require('./fixtures');

test('starting the app runs the gateway and shows its console in the window', async ({ window }) => {
  const tree = window.getByRole('tree', { name: '资源' });
  await expect(tree.getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });
  await expect(tree.getByRole('treeitem', { name: /line-a/ })).toBeVisible();
  expect(await window.title()).toContain('ModMux');
});

test('the gateway port only serves the window: other local clients get 401', async ({ window }) => {
  await expect(window.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });
  const origin = new URL(window.url()).origin;
  expect(origin).toMatch(/^http:\/\/127\.0\.0\.1:\d+$/);

  for (const path of ['/api/v1/status', '/api/v1/config', '/']) {
    const res = await fetch(origin + path);
    expect(res.status, `GET ${path} from outside the window`).toBe(401);
  }
  // Inside the window the same API works (the console loaded its metrics).
  await expect(window.getByRole('complementary', { name: '检查器' }).getByLabel('累计请求')).toHaveText('0');
});

test('the window cannot shrink below 1280x720 and shows the gateway output', async ({ app, window }) => {
  await expect(window.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });

  await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(1000, 600));
  // The minimum applies to the page area, not the frame: the console's own
  // "window too small" notice must never show in the desktop app.
  await expect.poll(() => window.evaluate(() => [innerWidth, innerHeight])).toEqual([1280, 720]);
  await expect(window.getByRole('alert').filter({ hasText: '窗口尺寸低于最小要求' })).toBeHidden();

  const dock = window.getByRole('region', { name: '底部面板' });
  await dock.getByRole('tab', { name: '网关输出' }).click();
  await expect(dock.getByRole('log', { name: '网关输出' })).toContainText('Modbus TCP server listening');
});

test('restarting the gateway from the window applies the saved config', async ({ window, modbusPort }) => {
  const { readHolding } = require('../../e2e/modbus');
  const tree = window.getByRole('tree', { name: '资源' });
  await tree.getByRole('treeitem', { name: /plc-sim/ }).click({ timeout: 10_000 });
  await window.getByRole('region', { name: '编辑区' }).getByLabel('Slave IDs').fill('110');
  await window.getByRole('button', { name: /保存/ }).click();
  await expect(window.getByRole('banner')).toContainText('已保存，重启网关后生效');
  await expect(readHolding(modbusPort, 110, 0, 1)).rejects.toThrow(); // still running the old routes

  await window.getByRole('button', { name: '重启网关' }).click();

  await expect(window.getByRole('banner')).toContainText('配置与运行中一致', { timeout: 10_000 });
  await expect(readHolding(modbusPort, 110, 0, 1)).resolves.toEqual([0]);
  await expect(readHolding(modbusPort, 100, 0, 1)).rejects.toThrow();
});

const net = require('node:net');
const fs = require('node:fs');
const path = require('node:path');
const { launchApp, closeApp, sampleConfig } = require('./fixtures');

function portOpen(port) {
  return new Promise((resolve) => {
    const s = net.connect(port, '127.0.0.1');
    s.once('connect', () => { s.destroy(); resolve(true); });
    s.once('error', () => resolve(false));
  });
}

// answerCloseDialog makes the next confirmation dialog pick the button
// whose label matches.
function answerCloseDialog(app, label) {
  return app.evaluate(({ dialog }, label) => {
    dialog.showMessageBox = async (_win, opts) => ({ response: opts.buttons.indexOf(label) });
  }, label);
}

test('closing the window asks first, then stops the gateway gracefully so its data survives', async ({ dir, modbusPort }) => {
  const { writeRegister, readHolding } = require('../../e2e/modbus');
  const configPath = path.join(dir, 'mmap.yaml');
  fs.writeFileSync(configPath, sampleConfig(modbusPort, `{ type: mmap, path: "${path.join(dir, 'line-a.bin')}" }`));

  let app = await launchApp(dir, { MODMUX_CONFIG: configPath });
  let win = await app.firstWindow();
  await expect(win.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });
  await writeRegister(modbusPort, 100, 9, 2468);

  await answerCloseDialog(app, '取消');
  await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].close());
  await expect.poll(() => portOpen(modbusPort)).toBe(true);
  expect(await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows().length)).toBe(1);

  await answerCloseDialog(app, '停止并退出');
  const closed = app.waitForEvent('close');
  await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].close());
  await closed;
  expect(await portOpen(modbusPort)).toBe(false);

  app = await launchApp(dir, { MODMUX_CONFIG: configPath });
  win = await app.firstWindow();
  await expect(win.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });
  await expect(readHolding(modbusPort, 100, 9, 1)).resolves.toEqual([2468]);
  await closeApp(app);
});

test('if the app itself is killed, the gateway exits within 5s', async ({ app, window, modbusPort }) => {
  await expect(window.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });
  expect(await portOpen(modbusPort)).toBe(true);

  app.process().kill('SIGKILL');

  await expect.poll(() => portOpen(modbusPort), { timeout: 5_000 }).toBe(false);
});

test('if the gateway dies, the window says why and can start it again', async ({ window, configPath, modbusPort }) => {
  const { execSync } = require('node:child_process');
  await expect(window.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });

  execSync(`pkill -KILL -f -- ${JSON.stringify(configPath)}`); // simulate a crash

  await expect(window.getByRole('heading', { name: '网关已停止' })).toBeVisible({ timeout: 5_000 });
  await expect(window.getByText('SIGKILL')).toBeVisible();
  await expect(window.getByRole('log', { name: '网关输出' })).toContainText('Modbus TCP server listening');

  await window.getByRole('button', { name: '重新启动' }).click();
  await expect(window.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });
  await expect.poll(() => portOpen(modbusPort)).toBe(true);
});

test('a gateway that cannot start shows the reason instead of a blank window', async ({ dir, modbusPort }) => {
  const badConfig = path.join(dir, 'bad.yaml');
  fs.writeFileSync(badConfig, sampleConfig(modbusPort).replace('version: 1', 'version: 1\nnot_a_field: true'));
  let app = await launchApp(dir, { MODMUX_CONFIG: badConfig });
  let win = await app.firstWindow();
  await expect(win.getByRole('heading', { name: '网关启动失败' })).toBeVisible({ timeout: 10_000 });
  await expect(win.getByRole('log', { name: '网关输出' })).toContainText('not_a_field');
  await closeApp(app);

  app = await launchApp(dir, { MODMUX_CONFIG: badConfig, MODMUX_GATEWAY_BIN: path.join(dir, 'no-such-binary') });
  win = await app.firstWindow();
  await expect(win.getByRole('heading', { name: '网关启动失败' })).toBeVisible({ timeout: 10_000 });
  await expect(win.getByText(/ENOENT/)).toBeVisible();

  // Nothing is running, so closing does not ask (a "取消" answer would keep it open).
  await answerCloseDialog(app, '取消');
  const closed = app.waitForEvent('close');
  await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].close());
  await closed;
});

test('launching the app again focuses the running one instead of starting a second gateway', async ({ app, window, dir, configPath }) => {
  const { spawn, execSync } = require('node:child_process');
  await expect(window.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });

  const second = spawn(require('electron'), [path.resolve(__dirname, '..')], {
    env: { ...process.env, MODMUX_USER_DATA: path.join(dir, 'user-data'), MODMUX_CONFIG: configPath, MODMUX_GATEWAY_BIN: path.resolve(__dirname, '../../modbus-gateway') },
    stdio: 'ignore',
  });
  const code = await new Promise((resolve, reject) => {
    const t = setTimeout(() => { second.kill(); reject(new Error('second instance kept running')); }, 10_000);
    second.once('exit', (c) => { clearTimeout(t); resolve(c); });
  });
  expect(code).toBe(0);

  const gateways = execSync(`pgrep -f -- ${JSON.stringify(configPath)} || true`).toString().trim().split('\n').filter(Boolean);
  expect(gateways).toHaveLength(1);
  await expect(window.getByRole('tree', { name: '资源' })).toBeVisible();
});

test('first run creates a sample config; File > Open switches config and is remembered', async ({ dir, modbusPort }) => {
  const userData = path.join(dir, 'user-data');
  const tree = (win) => win.getByRole('tree', { name: '资源' });

  // First run: no config anywhere yet.
  let app = await launchApp(dir);
  let win = await app.firstWindow();
  await expect(tree(win).getByRole('treeitem', { name: /demo/ }).first()).toBeVisible({ timeout: 10_000 });
  expect(fs.readFileSync(path.join(userData, 'config.yaml'), 'utf8')).toContain('version: 1');

  // File > Open… (the native dialog is replaced by a chosen path).
  const other = path.join(dir, 'line-a.yaml');
  fs.writeFileSync(other, sampleConfig(modbusPort));
  await app.evaluate(({ dialog, Menu }, file) => {
    dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [file] });
    Menu.getApplicationMenu().getMenuItemById('open-config').click();
  }, other);
  await expect(tree(win).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });
  await closeApp(app);

  // Next launch opens the last config and lists it under Open Recent.
  app = await launchApp(dir);
  win = await app.firstWindow();
  await expect(tree(win).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 10_000 });
  const recent = await app.evaluate(({ Menu }) => Menu.getApplicationMenu().getMenuItemById('recent-configs').submenu.items.map((i) => i.label));
  expect(recent).toContain(other);
  await closeApp(app);
});

test('View > Details / Topology (Cmd/Ctrl+1/2) switch the editor', async ({ app, window }) => {
  const editor = window.getByRole('region', { name: '编辑区' });
  await expect(editor.getByLabel('Slave IDs')).toBeVisible({ timeout: 10_000 });
  const clickView = (label) => app.evaluate(({ Menu }, label) => {
    const view = Menu.getApplicationMenu().items.find((i) => i.label === '视图');
    view.submenu.items.find((i) => i.label === label).click();
  }, label);

  await clickView('拓扑');
  await expect(editor.getByRole('group', { name: '拓扑' })).toBeVisible();
  await clickView('详情');
  await expect(editor.getByLabel('Slave IDs')).toBeVisible();
});

test('the page has no Node access, cannot navigate away, and opens external links in the browser', async ({ app, window }) => {
  await expect(window.getByRole('tree', { name: '资源' })).toBeVisible({ timeout: 10_000 });
  const consoleUrl = window.url();

  expect(await window.evaluate(() => [typeof require, typeof process, Object.keys(window.modmuxDesktop).sort().join(',')]))
    .toEqual(['undefined', 'undefined', 'getOutput,onOutput,onView,restartGateway']);

  // Stub the system browser so the test never opens a real one.
  await app.evaluate(({ shell }) => { globalThis.opened = []; shell.openExternal = async (u) => { globalThis.opened.push(u); }; });

  await window.evaluate(() => { location.href = 'file:///etc/passwd'; });
  await window.waitForTimeout(300);
  expect(window.url()).toBe(consoleUrl);
  await window.evaluate(() => { location.href = 'https://example.com/'; });
  await window.evaluate(() => window.open('https://example.com/docs'));
  await window.evaluate(() => window.open('file:///etc/passwd'));
  await expect.poll(() => app.evaluate(() => globalThis.opened)).toEqual(['https://example.com/', 'https://example.com/docs']);
  expect(window.url()).toBe(consoleUrl);
  expect(await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows().length)).toBe(1);
});

test('the local error page cannot navigate to other local files', async ({ window, configPath }) => {
  const { execSync } = require('node:child_process');
  await expect(window.getByRole('tree', { name: '资源' })).toBeVisible({ timeout: 10_000 });
  execSync(`pkill -KILL -f -- ${JSON.stringify(configPath)}`);
  await expect(window.getByRole('heading', { name: '网关已停止' })).toBeVisible({ timeout: 5_000 });
  const stoppedUrl = window.url();

  // file:// to file:// is allowed by Chromium itself, so only the app's own
  // navigation rules stop this.
  await window.evaluate(() => { location.href = 'file:///etc/passwd'; });
  await window.waitForTimeout(300);
  expect(window.url()).toBe(stoppedUrl);
});
