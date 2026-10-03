// Builds one desktop installer: compiles the modbus-gateway binary for the
// target into resources/bin/<os>-<arch>/ (bundled via extraResources), then
// runs electron-builder. Installer names are ModMux-<version>-<os>-<arch>
// with our own os/arch words on every platform (electron-builder's ${arch}
// would say x86_64 or amd64 depending on the package format).
// Usage: node scripts/dist.js <mac|win|linux> <x64|arm64>
'use strict';

const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const [os, arch] = process.argv.slice(2);
const GOOS = { mac: 'darwin', win: 'windows', linux: 'linux' }[os];
const GOARCH = { x64: 'amd64', arm64: 'arm64' }[arch];
if (!GOOS || !GOARCH) {
  console.error('usage: dist.js <mac|win|linux> <x64|arm64>');
  process.exit(2);
}

const appDir = path.resolve(__dirname, '..');
const root = path.resolve(appDir, '..');
const { version } = require('../package.json');

const binDir = path.join(appDir, 'resources', 'bin', `${os}-${arch}`);
fs.rmSync(binDir, { recursive: true, force: true });
fs.mkdirSync(binDir, { recursive: true });
const bin = path.join(binDir, GOOS === 'windows' ? 'modbus-gateway.exe' : 'modbus-gateway');
execFileSync('go', ['build', '-trimpath', '-ldflags', `-s -w -X main.version=${version}`, '-o', bin, '.'], {
  cwd: root,
  env: { ...process.env, GOOS, GOARCH, CGO_ENABLED: '0' },
  stdio: 'inherit',
});
console.log(`staged ${path.relative(appDir, bin)} (${GOOS}/${GOARCH}, v${version})`);

const name = `ModMux-\${version}-${os}-${arch}`;
const builder = path.join(appDir, 'node_modules', '.bin', process.platform === 'win32' ? 'electron-builder.cmd' : 'electron-builder');
execFileSync(builder, [
  `--${os}`, `--${arch}`, '--publish', 'never',
  `-c.artifactName=${name}.\${ext}`,
  `-c.portable.artifactName=${name}-portable.\${ext}`,
], { cwd: appDir, stdio: 'inherit', shell: process.platform === 'win32' });
