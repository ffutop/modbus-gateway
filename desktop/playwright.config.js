// Drives the real Electron app (tests/fixtures.js). The app runs the prebuilt
// ../modbus-gateway binary; build it first:
//   go build -o modbus-gateway . && (cd desktop && npm test)
const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './tests',
  timeout: 30_000,
  workers: 2,
  reporter: 'list',
  use: { trace: 'retain-on-failure' },
  projects: [
    { name: 'app', testIgnore: /packaged\// },
    // Needs `npm run dist:mac-x64` first.
    { name: 'packaged', testMatch: /packaged\/.*\.spec\.js/ },
  ],
});
