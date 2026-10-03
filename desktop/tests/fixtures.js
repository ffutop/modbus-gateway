// Launches the desktop app with an isolated user-data directory, a config
// file and the gateway binary all under a per-test temp directory, so tests
// never touch the developer's real ModMux settings.
const base = require('@playwright/test');
const { _electron: electron } = require('playwright');
const fs = require('node:fs');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');

const APP_DIR = path.resolve(__dirname, '..');
const GATEWAY_BIN = path.resolve(APP_DIR, '..', 'modbus-gateway');

async function freePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.listen(0, '127.0.0.1', () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
    srv.on('error', reject);
  });
}

function sampleConfig(modbusPort, persistence = '{ type: memory }') {
  return `# Desktop test config
version: 1

simulations:
  - name: line-a
    persistence: ${persistence}

gateways:
  - name: business
    upstreams:
      - type: tcp
        tcp: { address: "127.0.0.1:${modbusPort}" }
    downstreams:
      - name: plc-sim
        type: local
        slave_ids: "100"
        simulation: { ref: line-a }
`;
}

// launchApp starts the app; extraEnv overrides the defaults.
async function launchApp(dir, extraEnv = {}) {
  const app = await electron.launch({
    args: [APP_DIR],
    env: {
      ...process.env,
      MODMUX_GATEWAY_BIN: GATEWAY_BIN,
      MODMUX_USER_DATA: path.join(dir, 'user-data'),
      ...extraEnv,
    },
  });
  return app;
}

// closeApp quits the app, confirming the "stop the gateway?" dialog.
async function closeApp(app) {
  await app.evaluate(({ dialog }) => {
    dialog.showMessageBox = async (_win, opts) => ({ response: opts.buttons.indexOf('停止并退出') });
  }).catch(() => {}); // already gone (e.g. killed by the test)
  await app.close().catch(() => {});
}

exports.test = base.test.extend({
  dir: async ({}, use) => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'modmux-desktop-'));
    await use(dir);
    fs.rmSync(dir, { recursive: true, force: true });
  },
  modbusPort: async ({}, use) => use(await freePort()),
  configPath: async ({ dir, modbusPort }, use) => {
    const p = path.join(dir, 'line-a.yaml');
    fs.writeFileSync(p, sampleConfig(modbusPort));
    await use(p);
  },
  app: async ({ dir, configPath }, use) => {
    const app = await launchApp(dir, { MODMUX_CONFIG: configPath });
    await use(app);
    await closeApp(app);
  },
  window: async ({ app }, use) => {
    const win = await app.firstWindow();
    const errors = [];
    win.on('pageerror', (e) => errors.push(e));
    // A Content-Security-Policy block only logs to the console.
    win.on('console', (msg) => { if (/Content Security Policy/i.test(msg.text())) errors.push(new Error(msg.text())); });
    await use(win);
    base.expect(errors, `uncaught page errors:\n${errors.map((e) => e.stack).join('\n')}`).toEqual([]);
  },
});
exports.expect = base.expect;
exports.launchApp = launchApp;
exports.closeApp = closeApp;
exports.sampleConfig = sampleConfig;
