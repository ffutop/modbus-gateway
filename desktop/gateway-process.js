// GatewayProcess runs the bundled modbus-gateway binary as a child process
// using the server's desktop protocol (design 6.1): it listens on a free
// loopback port reported in a {"event":"ui_ready"} stdout line, requires a
// per-launch token, and shuts down gracefully when its stdin is closed.
'use strict';

const { spawn } = require('node:child_process');
const crypto = require('node:crypto');
const { EventEmitter } = require('node:events');
const readline = require('node:readline');

const READY_TIMEOUT_MS = 10_000;
const OUTPUT_LINES = 500;
const STOP_TIMEOUT_MS = 10_000;

class GatewayProcess extends EventEmitter {
  constructor(binary) {
    super();
    this.binary = binary;
    this.child = null;
    this.token = null;
    this.url = null;
    this.output = []; // last OUTPUT_LINES lines of stdout+stderr
  }

  // running reports whether a gateway process is alive.
  get running() {
    const c = this.child;
    return Boolean(c && c.exitCode === null && c.signalCode === null);
  }

  // record keeps a line of output and announces it as an 'output' event.
  record(line) {
    this.output.push(line);
    if (this.output.length > OUTPUT_LINES) this.output.shift();
    this.emit('output', line);
  }

  // start launches the gateway on configPath and resolves with its base URL
  // once the management API is listening.
  start(configPath) {
    this.token = crypto.randomBytes(24).toString('hex');
    const child = spawn(this.binary, ['-config', configPath, '-ui-listen', '127.0.0.1:0', '-exit-on-stdin-eof'], {
      env: { ...process.env, MODMUX_UI_TOKEN: this.token },
      stdio: ['pipe', 'pipe', 'pipe'],
      windowsHide: true,
    });
    this.child = child;
    this.stopping = false;
    let ready = false;
    // 'close' (not 'exit') fires after stdout/stderr are drained, so the
    // output that explains an exit has been recorded by then.
    child.once('close', (code, signal) => {
      // After startup, any exit we did not ask for is a crash.
      if (ready && !this.stopping) this.emit('crashed', { code, signal });
    });

    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('the gateway did not start within 10s')), READY_TIMEOUT_MS);
      child.once('error', (err) => { clearTimeout(timer); reject(err); });
      child.once('close', (code, signal) => {
        clearTimeout(timer);
        reject(new Error(signal ? `网关被信号 ${signal} 终止` : `网关以退出码 ${code} 退出`));
      });
      readline.createInterface({ input: child.stderr }).on('line', (line) => this.record(line));
      readline.createInterface({ input: child.stdout }).on('line', (line) => {
        this.record(line);
        try {
          const msg = JSON.parse(line);
          if (msg.event === 'ui_ready') {
            clearTimeout(timer);
            ready = true;
            this.url = `http://${msg.addr}`;
            resolve(this.url);
          }
        } catch {}
      });
    });
  }

  // stop closes stdin (the graceful path on every OS) and waits for exit.
  async stop() {
    const child = this.child;
    if (!child || child.exitCode !== null || child.signalCode !== null) return;
    this.stopping = true;
    const exited = new Promise((resolve) => child.once('exit', resolve));
    child.stdin.end();
    const timer = setTimeout(() => child.kill(), STOP_TIMEOUT_MS);
    await exited;
    clearTimeout(timer);
  }
}

module.exports = { GatewayProcess };
