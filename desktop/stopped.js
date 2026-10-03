// Shown when the gateway stopped unexpectedly or failed to start; the data
// comes from the main process in the "data" query parameter.
'use strict';

const data = JSON.parse(new URLSearchParams(location.search).get('data') || '{}');
document.getElementById('title').textContent = data.title || '网关已停止';
document.getElementById('reason').textContent = data.reason || '';
document.getElementById('output').textContent = (data.output || []).join('\n') || '（没有输出）';
document.getElementById('restart').onclick = (e) => {
  e.target.disabled = true;
  window.modmuxDesktop.restartGateway();
};
