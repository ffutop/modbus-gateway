// Which config file the app runs, and the recently opened ones. Choice order:
// MODMUX_CONFIG (development and tests), the last file opened, then a sample
// config created in the user data directory on first run.
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const RECENT_MAX = 8;

const SAMPLE_CONFIG = `# ModMux sample configuration, created on first run.
# Edit it in the console or any text editor; the gateway reads it on start.
version: 1

simulations:
  - name: demo
    persistence: { type: memory }

gateways:
  - name: demo
    upstreams:
      - type: tcp
        tcp: { address: "127.0.0.1:5020" }
    downstreams:
      - name: demo-slave
        type: local
        slave_ids: "1"
        simulation: { ref: demo }
`;

class ConfigFiles {
  constructor(userDataDir) {
    this.dir = userDataDir;
    this.settingsPath = path.join(userDataDir, 'settings.json');
    this.settings = this.load();
  }

  load() {
    try {
      const s = JSON.parse(fs.readFileSync(this.settingsPath, 'utf8'));
      return { recent: Array.isArray(s.recent) ? s.recent : [] };
    } catch {
      return { recent: [] };
    }
  }

  get recent() {
    return this.settings.recent;
  }

  // initial picks the config to run at startup.
  initial() {
    if (process.env.MODMUX_CONFIG) return process.env.MODMUX_CONFIG;
    const last = this.recent.find((p) => fs.existsSync(p));
    if (last) return last;
    const sample = path.join(this.dir, 'config.yaml');
    if (!fs.existsSync(sample)) {
      fs.mkdirSync(this.dir, { recursive: true });
      fs.writeFileSync(sample, SAMPLE_CONFIG);
    }
    return sample;
  }

  // remember puts file first in the recent list and saves it.
  remember(file) {
    this.settings.recent = [file, ...this.recent.filter((p) => p !== file)].slice(0, RECENT_MAX);
    fs.mkdirSync(this.dir, { recursive: true });
    fs.writeFileSync(this.settingsPath, JSON.stringify(this.settings, null, 2));
  }
}

module.exports = { ConfigFiles };
