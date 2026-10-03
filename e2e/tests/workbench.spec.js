const { test, expect } = require('../fixtures');

test('resource tree lists the gateways, downstreams and simulations from the config', async ({ page, gateway }) => {
  await page.goto(gateway.url);

  const tree = page.getByRole('tree', { name: '资源' });
  await expect(tree.getByRole('treeitem', { name: /business/ })).toBeVisible();
  for (const name of ['plc-sim', 'boiler-sim', 'remote-plc', 'line-a']) {
    await expect(tree.getByRole('treeitem', { name: new RegExp(name) })).toBeVisible();
  }
});

test('selecting a downstream drives the editor, inspector and log filter together', async ({ page, gateway }) => {
  await page.goto(gateway.url);
  const tree = page.getByRole('tree', { name: '资源' });

  await tree.getByRole('treeitem', { name: /boiler-sim/ }).click();

  await expect(tree.getByRole('treeitem', { name: /boiler-sim/ })).toHaveAttribute('aria-selected', 'true');
  const editor = page.getByRole('region', { name: '编辑区' });
  await expect(editor.getByLabel('名称')).toHaveValue('boiler-sim');
  await expect(editor.getByLabel('Slave IDs')).toHaveValue('101');
  await expect(editor.getByLabel('模拟从站')).toHaveValue('line-a');
  await expect(page.getByRole('complementary', { name: '检查器' })).toContainText('business/boiler-sim');
  await expect(page.getByRole('region', { name: '底部面板' })).toContainText('筛选：business/boiler-sim');

  // A tcp downstream shows its target address instead of a simulation.
  await tree.getByRole('treeitem', { name: /remote-plc/ }).click();
  await expect(editor.getByLabel('目标地址')).toHaveValue('127.0.0.1:9');
  await expect(page.getByRole('complementary', { name: '检查器' })).toContainText('business/remote-plc');
});

test('a conflicting slave ID is flagged in the form, tree and problems tab, and blocks saving', async ({ page, gateway }) => {
  await page.goto(gateway.url);
  const tree = page.getByRole('tree', { name: '资源' });
  const editor = page.getByRole('region', { name: '编辑区' });
  const save = page.getByRole('button', { name: /保存/ });
  await tree.getByRole('treeitem', { name: /boiler-sim/ }).click();
  await expect(save).toBeDisabled(); // nothing to save yet

  await editor.getByLabel('Slave IDs').fill('100');

  await expect(editor.getByLabel('Slave IDs')).toHaveAttribute('aria-invalid', 'true');
  await expect(editor.getByText(/already routed to downstream "plc-sim"/)).toBeVisible();
  await expect(tree.getByRole('treeitem', { name: /boiler-sim/ })).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByRole('tab', { name: /问题/ })).toContainText('1');
  await expect(save).toBeDisabled();

  await editor.getByLabel('Slave IDs').fill('102');

  await expect(editor.getByLabel('Slave IDs')).not.toHaveAttribute('aria-invalid', 'true');
  await expect(tree.getByRole('treeitem', { name: /boiler-sim/ })).not.toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByRole('tab', { name: /问题/ })).not.toContainText('1');
  await expect(save).toBeEnabled();
  expect(gateway.readConfig()).toContain('slave_ids: "101"'); // validating never writes
});

test('saving writes only the edited value and says a restart is needed', async ({ page, gateway }) => {
  const before = gateway.readConfig();
  await page.goto(gateway.url);
  const editor = page.getByRole('region', { name: '编辑区' });
  await page.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /boiler-sim/ }).click();

  await editor.getByLabel('Slave IDs').fill('102');
  await page.getByRole('button', { name: /保存/ }).click();

  await expect(page.getByRole('banner')).toContainText('已保存，重启网关后生效');
  await expect(page.getByRole('banner')).not.toContainText('未保存修改');
  await expect(page.getByRole('button', { name: /保存/ })).toBeDisabled();
  expect(gateway.readConfig()).toBe(before.replace('slave_ids: "101"', 'slave_ids: "102"'));

  // The notice survives a reload: it comes from the file differing from what is running.
  await page.reload();
  await expect(page.getByRole('banner')).toContainText('已保存，重启网关后生效');
  await expect(editor.getByLabel('Slave IDs')).toHaveValue('100'); // plc-sim is selected first
});

test('live requests appear in the log for the selection, and the inspector shows their rate', async ({ page, gateway, modbusPort }) => {
  const { readHolding } = require('../modbus');
  await page.goto(gateway.url); // plc-sim (slave 100) is selected first
  const dock = page.getByRole('region', { name: '底部面板' });
  const inspector = page.getByRole('complementary', { name: '检查器' });

  for (let i = 0; i < 5; i++) await readHolding(modbusPort, 100, 0, 1);
  await readHolding(modbusPort, 101, 0, 1); // boiler-sim: filtered out of plc-sim's log

  const rows = dock.getByRole('row').filter({ hasText: '读保持' });
  await expect(rows).toHaveCount(5);
  await expect(rows.first()).toContainText('plc-sim');
  await expect(dock.getByRole('row').filter({ hasText: 'boiler-sim' })).toHaveCount(0);
  await expect(inspector.getByLabel('累计请求')).toHaveText('5');

  // Keep traffic flowing so the rate between two metrics polls is non-zero.
  const pump = setInterval(() => readHolding(modbusPort, 100, 0, 1).catch(() => {}), 50);
  try {
    await expect(inspector.getByLabel('请求/秒')).not.toHaveText('0', { timeout: 5000 });
  } finally {
    clearInterval(pump);
  }
});

test('the register viewer shows what Modbus clients wrote, and keeps up with new writes', async ({ page, gateway, modbusPort }) => {
  const { writeRegister } = require('../modbus');
  await writeRegister(modbusPort, 100, 3, 4321);
  await page.goto(gateway.url);
  const editor = page.getByRole('region', { name: '编辑区' });

  await page.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /line-a/ }).click();

  await expect(editor.getByRole('button', { name: '保持寄存器 4x' })).toHaveAttribute('aria-pressed', 'true');
  await expect(editor.getByLabel('地址 3', { exact: true })).toHaveText('4321');
  await expect(editor.getByText('ready')).toBeVisible();

  await writeRegister(modbusPort, 101, 3, 1111); // boiler-sim shares line-a
  await expect(editor.getByLabel('地址 3', { exact: true })).toHaveText('1111');

  await editor.getByLabel('起始地址').fill('0010');
  await editor.getByLabel('起始地址').press('Enter');
  await expect(editor.getByLabel('地址 16', { exact: true })).toBeVisible();
  await expect(editor.getByLabel('地址 3', { exact: true })).toHaveCount(0);
});

test('the topology shows the selected gateway and selecting a node selects it in the tree', async ({ page, gateway, modbusPort }) => {
  await page.goto(gateway.url);
  const editor = page.getByRole('region', { name: '编辑区' });

  await editor.getByRole('tab', { name: /拓扑/ }).click();
  const topo = editor.getByRole('group', { name: '拓扑' });
  for (const name of [`127.0.0.1:${modbusPort}`, 'business', 'plc-sim', 'boiler-sim', 'remote-plc', 'line-a']) {
    await expect(topo.getByRole('button', { name: new RegExp(name.replace(/\./g, '\\.')) })).toBeVisible();
  }
  await expect(topo.getByRole('button', { name: /plc-sim/ }).first()).toHaveAttribute('aria-pressed', 'true');

  await topo.getByRole('button', { name: /remote-plc/ }).click();
  await expect(page.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /remote-plc/ })).toHaveAttribute('aria-selected', 'true');
  await expect(topo.getByRole('button', { name: /remote-plc/ })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('complementary', { name: '检查器' })).toContainText('business/remote-plc');

  // Wide windows show details and topology side by side, without tabs.
  await page.setViewportSize({ width: 1920, height: 1080 });
  await expect(editor.getByRole('tab')).toHaveCount(0);
  await expect(editor.getByLabel('目标地址')).toBeVisible();
  await expect(editor.getByRole('group', { name: '拓扑' })).toBeVisible();
});

test('an edit made in a text editor meanwhile is never overwritten; the console asks to reload', async ({ page, gateway }) => {
  const fs = require('node:fs');
  await page.goto(gateway.url);
  const editor = page.getByRole('region', { name: '编辑区' });
  await page.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /boiler-sim/ }).click();

  const external = gateway.readConfig().replace('slave_ids: "101"', 'slave_ids: "105"');
  fs.writeFileSync(gateway.configPath, external);
  await editor.getByLabel('Slave IDs').fill('102');

  const alert = page.getByRole('alert').filter({ hasText: '配置文件已在别处被修改' });
  await expect(alert).toBeVisible();
  await expect(page.getByRole('button', { name: /保存/ })).toBeDisabled();
  expect(gateway.readConfig()).toBe(external);

  await alert.getByRole('button', { name: '重新加载' }).click();
  await expect(alert).toHaveCount(0);
  await expect(editor.getByLabel('Slave IDs')).toHaveValue('105');
  await expect(page.getByRole('banner')).not.toContainText('未保存修改');
});

test('a topology shown beside the form follows the draft and flags its conflicts', async ({ page, gateway }) => {
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto(gateway.url);
  const editor = page.getByRole('region', { name: '编辑区' });
  await page.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /boiler-sim/ }).click();

  await editor.getByLabel('Slave IDs').fill('100');

  const node = editor.getByRole('group', { name: '拓扑' }).getByRole('button', { name: /boiler-sim/ });
  await expect(node).toContainText('冲突');
  await expect(node).toContainText('ID 100');
  await expect(editor.getByLabel('Slave IDs')).toBeFocused(); // updating the topology must not steal focus
});

test('Ctrl+S saves, and the toolbar names the actual config file', async ({ page, gateway }) => {
  const path = require('node:path');
  await page.goto(gateway.url);
  await expect(page.getByRole('banner')).toContainText(path.basename(gateway.configPath));
  await page.getByRole('tree', { name: '资源' }).getByRole('treeitem', { name: /boiler-sim/ }).click();
  await page.getByRole('region', { name: '编辑区' }).getByLabel('Slave IDs').fill('103');
  await expect(page.getByRole('button', { name: /保存/ })).toBeEnabled();

  await page.keyboard.press('ControlOrMeta+s');

  await expect(page.getByRole('banner')).toContainText('已保存，重启网关后生效');
  expect(gateway.readConfig()).toContain('slave_ids: "103"');
});

test('the browser console has no desktop-only controls', async ({ page, gateway }) => {
  await page.goto(gateway.url);
  await expect(page.getByRole('tab', { name: '请求日志' })).toBeVisible();
  await expect(page.getByRole('tab', { name: '网关输出' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: /重启网关/ })).toHaveCount(0);
});
