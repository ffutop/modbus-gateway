// Browser tests for the management console. They start the prebuilt
// ../modbus-gateway binary per test (see fixtures.js); build it first:
//   go build -o modbus-gateway . && (cd e2e && npm test)
const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './tests',
  timeout: 20_000,
  fullyParallel: true,
  reporter: 'list',
  use: {
    viewport: { width: 1280, height: 720 },
    trace: 'retain-on-failure',
  },
});
