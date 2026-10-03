// Runs against the output of `npm run dist:mac-x64` (see package.json).
const { test, expect, launchApp, closeApp, sampleConfig } = require('../fixtures');
const { _electron: electron } = require('playwright');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const DIST = path.resolve(__dirname, '../../dist');
const VERSION = require('../../package.json').version;
const APP = path.join(DIST, 'mac', 'ModMux.app');

test('the macOS x64 build is named per convention and bundles the x64 gateway', async () => {
  expect(fs.existsSync(path.join(DIST, `ModMux-${VERSION}-mac-x64.dmg`))).toBe(true);
  const bin = path.join(APP, 'Contents', 'Resources', 'bin', 'modbus-gateway');
  expect(execFileSync('file', [bin]).toString()).toMatch(/Mach-O 64-bit executable x86_64/);
});

test('the packaged app starts its bundled gateway and shows the console', async ({ dir, configPath }) => {
  const app = await electron.launch({
    executablePath: path.join(APP, 'Contents', 'MacOS', 'ModMux'),
    env: { ...process.env, MODMUX_USER_DATA: path.join(dir, 'user-data'), MODMUX_CONFIG: configPath },
  });
  const win = await app.firstWindow();
  await expect(win.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /plc-sim/ })).toBeVisible({ timeout: 15_000 });
  const status = await win.evaluate(() => fetch('/api/v1/status').then((r) => r.json()));
  expect(status.version).toBe(VERSION);
  await closeApp(app);
});
