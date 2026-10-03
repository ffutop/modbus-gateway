// Shown when the gateway stopped unexpectedly or failed to start; the data
// comes from the main process in the "data" query parameter.
'use strict';

const data = JSON.parse(new URLSearchParams(location.search).get('data') || '{}');
document.getElementById('title').textContent = data.title || '网关已停止';
document.getElementById('reason').textContent = (data.reason || '') + (data.hasDraft ? ' · 未保存草稿已保留，重新启动后可继续编辑' : '');
document.getElementById('output').textContent = (data.output || []).join('\n') || '（没有输出）';
document.getElementById('restart').onclick = (e) => {
  e.target.disabled = true;
  window.modmuxDesktop.restartGateway();
};

document.getElementById('config-path').textContent = '配置文件：' + (data.configPath || '未知');
document.getElementById('open-config').onclick = () => window.modmuxDesktop.openConfig();
document.getElementById('copy').onclick = async () => {
  await navigator.clipboard.writeText([data.title, data.reason, data.configPath, ...(data.output || []), data.draft ? JSON.stringify(data.draft,null,2) : ''].join('\n'));
  document.getElementById('copy').textContent = '已复制';
};
window.modmuxDesktop.onPhase(phase => { document.getElementById('reason').textContent = phase; });
