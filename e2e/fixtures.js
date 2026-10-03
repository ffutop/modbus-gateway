// Starts the real gateway binary the way the desktop shell does
// (-ui-listen 127.0.0.1:0, ui_ready line on stdout, stdin EOF to stop) with
// a fresh config file per test.
const base = require('@playwright/test');
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const readline = require('node:readline');

const BINARY = path.resolve(__dirname, '..', 'modbus-gateway');

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

// A v1 config with one gateway: plc-sim (local, slave 100) and boiler-sim
// (local, slave 101) share simulation line-a; remote-plc forwards 1-10 to a
// port nobody listens on.
function sampleConfig(modbusPort) {
  return `# Line A field gateway
version: 1

simulations:
  - name: line-a
    persistence: { type: memory }

gateways:
  - name: business
    upstreams:
      - type: tcp
        tcp: { address: "127.0.0.1:${modbusPort}" }
    downstreams:
      - name: plc-sim # PLC under test
        type: local
        slave_ids: "100"
        simulation: { ref: line-a }
      - name: boiler-sim
        type: local
        slave_ids: "101"
        simulation: { ref: line-a }
      - name: remote-plc
        type: tcp
        slave_ids: "1-10"
        tcp: { address: "127.0.0.1:9" }
`;
}

async function startGateway(configText) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'modmux-e2e-'));
  const configPath = path.join(dir, 'line-a-gateway.yaml');
  fs.writeFileSync(configPath, configText);
  const child = spawn(BINARY, ['-config', configPath, '-ui-listen', '127.0.0.1:0', '-exit-on-stdin-eof'], {
    stdio: ['pipe', 'pipe', 'inherit'],
  });
  const output = [];
  const addr = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`no ui_ready line:\n${output.join('\n')}`)), 5000);
    child.on('exit', (code) => reject(new Error(`gateway exited (${code}):\n${output.join('\n')}`)));
    readline.createInterface({ input: child.stdout }).on('line', (line) => {
      output.push(line);
      try {
        const msg = JSON.parse(line);
        if (msg.event === 'ui_ready') {
          clearTimeout(timer);
          resolve(msg.addr);
        }
      } catch {}
    });
  });
  return {
    url: `http://${addr}`,
    configPath,
    readConfig: () => fs.readFileSync(configPath, 'utf8'),
    async stop() {
      const exited = new Promise((resolve) => child.once('exit', resolve));
      child.stdin.end();
      await exited;
      fs.rmSync(dir, { recursive: true, force: true });
    },
  };
}

exports.test = base.test.extend({
  // Any uncaught error in the page fails the test with the error itself.
  page: async ({ page }, use) => {
    const errors = [];
    page.on('pageerror', (err) => errors.push(err));
    // A Content-Security-Policy block only logs to the console.
    page.on('console', (msg) => { if (/Content Security Policy/i.test(msg.text())) errors.push(new Error(msg.text())); });
    await use(page);
    base.expect(errors, `uncaught page errors:\n${errors.map((e) => e.stack).join('\n')}`).toEqual([]);
  },
  modbusPort: async ({}, use) => use(await freePort()),
  configText: async ({ modbusPort }, use) => use(sampleConfig(modbusPort)),
  gateway: async ({ configText }, use) => {
    const gw = await startGateway(configText);
    await use(gw);
    await gw.stop();
  },
});
exports.expect = base.expect;
