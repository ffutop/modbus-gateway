// Exposes the few desktop-only capabilities to the console page. The page
// itself never gets Node.js or the gateway token.
'use strict';

const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('modmuxDesktop', {
  // The gateway's recent stdout/stderr lines, then each new batch of lines.
  getOutput: () => ipcRenderer.invoke('gateway:output'),
  onOutput: (callback) => ipcRenderer.on('gateway:output', (_event, lines) => callback(lines)),
  // Stops the gateway gracefully and starts it again on the same config;
  // the window then reloads from the restarted gateway.
  restartGateway: () => ipcRenderer.invoke('gateway:restart'),
  // View menu: 'detail' or 'topo'.
  onView: (callback) => ipcRenderer.on('view', (_event, view) => callback(view)),
});
