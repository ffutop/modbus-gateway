const { test, expect } = require('../fixtures');
const { readHolding } = require('../modbus');

test('renaming a draft preserves running metrics and logs even after saving', async ({ page, gateway, modbusPort }) => {
  await page.goto(gateway.url);
  await page.getByRole('treeitem', { name: /plc-sim/ }).click();
  await readHolding(modbusPort, 100, 0, 1);
  await expect(page.getByLabel('累计请求')).toHaveText('1');
  await page.getByLabel('名称', { exact: true }).fill('renamed');
  await expect(page.getByRole('button', { name: /保存/ })).toBeEnabled();
  await page.waitForTimeout(1100);
  await expect(page.getByLabel('累计请求')).toHaveText('1');
  await page.getByRole('button', { name: /^保存/ }).click();
  await page.reload();
  await page.getByRole('treeitem', { name: /renamed/ }).click();
  await expect(page.getByLabel('累计请求')).toHaveText('1');
});

test('pausing the log keeps a readable snapshot while Modbus continues', async ({ page, gateway, modbusPort }) => {
  await page.goto(gateway.url);
  await readHolding(modbusPort, 100, 0, 1);
  await expect(page.getByRole('row').filter({ hasText: '读保持' })).toHaveCount(1);
  await page.getByRole('button', { name: '暂停显示' }).click();
  await readHolding(modbusPort, 100, 0, 1);
  await page.waitForTimeout(500);
  await expect(page.getByRole('row').filter({ hasText: '读保持' })).toHaveCount(1);
  await page.getByRole('button', { name: '恢复实时' }).click();
  await expect(page.getByRole('row').filter({ hasText: '读保持' })).toHaveCount(2);
});

test('gateway overview shows listener and routes; register addresses reject invalid input', async ({ page, gateway }) => {
  await page.goto(gateway.url);
  await expect(page.getByRole('treeitem', {name:/business/})).toHaveAttribute('aria-selected','true');
  await expect(page.getByRole('region', {name:'编辑区'})).toContainText('Slave ID 路由表');
  await page.screenshot({path:'/private/tmp/modmux-overview.png'});
  await page.getByRole('treeitem', {name:/line-a/}).click();
  await page.getByLabel('起始地址').fill('ZZZZ');
  await page.getByLabel('起始地址').press('Enter');
  await expect(page.getByLabel('起始地址')).toHaveAttribute('aria-invalid','true');
  await expect(page.getByLabel('地址 0', {exact:true})).toBeVisible();
  await page.getByRole('button', {name:'切换十进制'}).click();
  await page.getByLabel('起始地址').fill('65535');
  await page.getByLabel('起始地址').press('Enter');
  await expect(page.getByLabel('地址 65535', {exact:true})).toBeVisible();
  await expect(page.getByRole('button', {name:'下一屏'})).toBeDisabled();
  await page.getByLabel('地址 65535', {exact:true}).click();
  await expect(page.getByRole('dialog')).toContainText('65535 / 0xFFFF');
});

test('a failed save retains edits and offers an actionable error', async ({page,gateway}) => {
  await page.goto(gateway.url);
  await page.getByRole('treeitem', {name:/plc-sim/}).click();
  await page.getByLabel('Slave IDs').fill('110');
  await expect(page.getByRole('button',{name:/^保存/})).toBeEnabled();
  await page.route('**/api/v1/config', route => route.request().method() === 'PUT' ? route.fulfill({status:500,contentType:'application/json',body:'{"error":"disk full"}'}) : route.continue());
  await page.getByRole('button',{name:/^保存/}).click();
  await expect(page.getByRole('alert')).toContainText('保存失败');
  await expect(page.getByLabel('Slave IDs')).toHaveValue('110');
  await expect(page.getByRole('banner')).toContainText('1 处未保存修改');
  expect(gateway.readConfig()).toContain('slave_ids: "100"');
});

test('a small browser can enter a compact workspace and still inspect routes', async ({page,gateway}) => {
  await page.setViewportSize({width:1100,height:650});
  await page.goto(gateway.url);
  await page.getByRole('button',{name:'进入紧凑布局'}).click();
  await expect(page.getByRole('alert').filter({hasText:'窗口尺寸低于'})).toBeHidden();
  await expect(page.getByRole('region',{name:'编辑区'})).toContainText('Slave ID 路由表');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth && document.documentElement.scrollHeight <= innerHeight)).toBe(true);
  await page.screenshot({path:'/private/tmp/modmux-compact.png'});
});

test('log filters and details expose full requests without changing the resource selection', async ({page,gateway,modbusPort}) => {
  await page.goto(gateway.url);
  await page.getByRole('treeitem',{name:/plc-sim/}).click();
  await readHolding(modbusPort,100,0,1);
  await readHolding(modbusPort,101,0,1);
  await page.getByLabel('日志范围').selectOption('gateway');
  await expect(page.getByRole('row').filter({hasText:'读保持'})).toHaveCount(2);
  await page.getByLabel('筛选 Slave ID').fill('101');
  await expect(page.getByRole('row').filter({hasText:'读保持'})).toHaveCount(1);
  await page.getByRole('row').filter({hasText:'读保持'}).click();
  await expect(page.getByRole('dialog')).toContainText('"slave_id": 101');
  await expect(page.getByRole('treeitem',{name:/plc-sim/})).toHaveAttribute('aria-selected','true');
});

test('problem rows navigate to the offending field', async ({page,gateway}) => {
  await page.goto(gateway.url);
  await page.getByRole('treeitem',{name:/boiler-sim/}).click();
  await page.getByLabel('Slave IDs').fill('100');
  await expect(page.getByLabel('Slave IDs')).toHaveAttribute('aria-invalid','true');
  await page.getByRole('treeitem',{name:/remote-plc/}).click();
  await page.getByRole('tab',{name:/问题/}).click();
  await page.getByRole('table',{name:'问题'}).getByRole('row').first().click();
  await expect(page.getByLabel('Slave IDs')).toBeFocused();
  await expect(page.getByRole('treeitem',{name:/boiler-sim/})).toHaveAttribute('aria-selected','true');
});

test('external reordering does not bind a downstream to another runtime series', async ({page,gateway,modbusPort}) => {
  await readHolding(modbusPort,100,0,1);
  const fs = require('node:fs');
  const text = gateway.readConfig();
  const start = text.indexOf('      - name: plc-sim');
  const middle = text.indexOf('      - name: boiler-sim');
  const end = text.indexOf('      - name: remote-plc');
  fs.writeFileSync(gateway.configPath, text.slice(0,start) + text.slice(middle,end) + text.slice(start,middle) + text.slice(end));
  await page.goto(gateway.url);
  await page.getByRole('treeitem',{name:/boiler-sim/}).click();
  await expect(page.getByLabel('累计请求')).toHaveText('—'); // no samples exist for this downstream
  await page.getByRole('treeitem',{name:/plc-sim/}).click();
  await expect(page.getByLabel('累计请求')).toHaveText('1');
});

test.describe('legacy configurations', () => {
  test.use({configText: async ({modbusPort},use) => use(`gateways:\n  - name: legacy\n    upstreams:\n      - type: tcp\n        tcp: {address: "127.0.0.1:${modbusPort}"}\n    downstreams:\n      - name: old-local\n        type: local\n        slave_ids: "100"\n        local: {device: demo, persistence: {type: memory}}\n`)});
  test('legacy fields are visibly read-only and synthesized simulations are listed',async ({page,gateway}) => {
    await page.goto(gateway.url);
    await page.getByRole('treeitem',{name:/old-local/}).first().click();
    await expect(page.getByLabel('Slave IDs')).toHaveAttribute('readonly','');
    await expect(page.getByRole('banner')).toContainText('旧版配置仅支持查看');
    await expect(page.getByRole('button',{name:/^保存/})).toBeDisabled();
  });
});

test('a disconnected metrics feed marks displayed values as stale rather than healthy zeroes',async ({page,gateway,modbusPort}) => {
  await page.goto(gateway.url);
  await readHolding(modbusPort,100,0,1);
  await expect(page.getByLabel('累计请求')).toHaveText('1');
  await page.route('**/api/v1/metrics', route => route.abort());
  await expect(page.getByRole('alert').filter({hasText:'数据已过期'})).toBeVisible();
  await expect(page.getByLabel('累计请求')).toHaveText('—');
  await expect(page.getByRole('contentinfo')).toContainText('最后更新');
});

test.describe('large resource lists', () => {
  test.use({configText: async ({modbusPort},use) => {
    const header = `version: 1\nsimulations:\n  - name: model\n    persistence: {type: memory}\ngateways:\n  - name: many\n    upstreams:\n      - type: tcp\n        tcp: {address: "127.0.0.1:${modbusPort}"}\n    downstreams:\n`;
    const rows = Array.from({length:50},(_,i) => `      - name: node-${i}\n        type: local\n        slave_ids: "${i+20}"\n        simulation: {ref: model}\n`).join('');
    await use(header+rows);
  }});
  test('the last resource remains reachable, and search filters the tree',async ({page,gateway}) => {
    await page.goto(gateway.url);
    await page.getByRole('treeitem',{name:/node-49/}).click();
    await expect(page.getByLabel('Slave IDs')).toHaveValue('69');
    await page.getByLabel('搜索资源').fill('node-49');
    await expect(page.getByRole('treeitem',{name:/node-0 /})).toHaveCount(0);
    await expect(page.getByRole('treeitem',{name:/node-49/})).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(true);
  });
});

test.describe('shared simulation references', () => {
  test.use({configText: async ({modbusPort},use) => use(`version: 1\nsimulations:\n  - name: shared\n    persistence: {type: memory}\ngateways:\n  - name: first\n    upstreams:\n      - type: tcp\n        tcp: {address: "127.0.0.1:${modbusPort}"}\n    downstreams:\n      - name: one\n        type: local\n        slave_ids: "100"\n        simulation: {ref: shared}\n  - name: second\n    upstreams:\n      - type: tcp\n        tcp: {address: "127.0.0.1:0"}\n    downstreams:\n      - name: two\n        type: local\n        slave_ids: "100"\n        simulation: {ref: shared}\n`)});
  test('all referencing gateways can be selected in the running topology',async ({page,gateway}) => {
    await page.goto(gateway.url);
    await page.getByRole('treeitem',{name:/shared/}).click();
    await expect(page.getByRole('complementary',{name:'检查器'})).toContainText('first/one');
    await expect(page.getByRole('complementary',{name:'检查器'})).toContainText('second/two');
    await page.getByRole('tab',{name:/拓扑/}).click();
    await page.getByLabel('引用网关').selectOption('1');
    await expect(page.getByRole('group',{name:'拓扑'}).getByRole('button',{name:/two/})).toBeVisible();
    await expect(page.getByRole('tab',{name:/拓扑/})).toContainText('second');
  });
});

test('selecting a simulation filters requests to its runtime references',async ({page,gateway,modbusPort}) => {
  await page.goto(gateway.url);
  await readHolding(modbusPort,100,0,1);
  await readHolding(modbusPort,101,0,1);
  await expect(readHolding(modbusPort,1,0,1)).rejects.toThrow();
  await page.getByRole('treeitem',{name:/line-a/}).click();
  await expect(page.getByRole('row').filter({hasText:'读保持'})).toHaveCount(2);
  await expect(page.getByRole('region',{name:'底部面板'})).toContainText('关联模拟从站：line-a');
  await page.getByLabel('日志范围').selectOption('gateway');
  await expect(page.getByRole('row').filter({hasText:'读保持'})).toHaveCount(3);
});
