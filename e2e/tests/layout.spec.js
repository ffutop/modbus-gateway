const { test, expect } = require('../fixtures');

// overflow reports anything that would make the page scroll or cut content
// off: page-level scroll, elements past the viewport, panels whose content
// is taller/wider than the panel.
async function overflow(page) {
  return page.evaluate(() => {
    const out = [];
    const root = document.documentElement;
    if (root.scrollHeight > innerHeight || root.scrollWidth > innerWidth) out.push(`page ${root.scrollWidth}x${root.scrollHeight}`);
    for (const el of document.querySelectorAll('#app *')) {
      if (el.closest('svg') || el.closest('.topo')) continue; // the topology canvas positions its own nodes
      const r = el.getBoundingClientRect();
      if (r.width && (r.bottom > innerHeight + 1 || r.right > innerWidth + 1)) out.push(`outside: ${el.className || el.tagName}`);
    }
    for (const el of document.querySelectorAll('.side,.insp,.body,.box,.tool,.dock-tabs,.regbar')) {
      if ((el.scrollHeight > el.clientHeight + 2 && !['auto','scroll'].includes(getComputedStyle(el).overflowY)) || el.scrollWidth > el.clientWidth + 2) out.push(`clipped: ${el.className}`);
    }
    return [...new Set(out)];
  });
}

for (const [width, height] of [[1280, 720], [1366, 768], [1600, 900], [1920, 1080], [2560, 1440]]) {
  test(`one screen without scrolling at ${width}x${height}`, async ({ page, gateway }) => {
    await page.setViewportSize({ width, height });
    await page.goto(gateway.url);
    const tree = page.getByRole('tree', { name: '资源' });
    await expect(tree.getByRole('treeitem', { name: /business/ })).toHaveAttribute('aria-selected','true');
    await tree.getByRole('treeitem', { name: /plc-sim/ }).click();
    await expect(tree.getByRole('treeitem', { name: /plc-sim/ })).toHaveAttribute('aria-selected', 'true');
    expect(await overflow(page), 'downstream view').toEqual([]);

    await tree.getByRole('treeitem', { name: /line-a/ }).click();
    await expect(page.getByLabel('地址 0', { exact: true })).toBeVisible();
    expect(await overflow(page), 'simulation view').toEqual([]);

    const topoTab = page.getByRole('tab', { name: /拓扑/ });
    if (await topoTab.count()) await topoTab.click();
    await expect(page.getByRole('group', { name: '拓扑' })).toBeVisible();
    expect(await overflow(page), 'topology view').toEqual([]);
  });
}

test('ultra-wide windows move the request log into a full-height right column', async ({ page, gateway }) => {
  await page.setViewportSize({ width: 2560, height: 1440 });
  await page.goto(gateway.url);
  const dock = await page.getByRole('region', { name: '底部面板' }).boundingBox();
  const insp = await page.getByRole('complementary', { name: '检查器' }).boundingBox();
  expect(dock.x).toBeGreaterThanOrEqual(insp.x + insp.width - 1);
  expect(dock.height).toBeGreaterThan(1300);
});

test('tall windows add recent errors to the inspector', async ({ page, gateway }) => {
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto(gateway.url);
  await expect(page.getByRole('complementary', { name: '检查器' }).getByRole('list', { name: '最近错误' })).toBeVisible();
  await page.setViewportSize({ width: 1600, height: 900 });
  await expect(page.getByRole('complementary', { name: '检查器' }).getByRole('list', { name: '最近错误' })).toHaveCount(0);
});

test('a window below 1280x720 shows the minimum-size notice', async ({ page, gateway }) => {
  await page.setViewportSize({ width: 1100, height: 700 });
  await page.goto(gateway.url);
  await expect(page.getByRole('alert').filter({ hasText: '窗口尺寸低于最小要求 1280 × 720' })).toBeVisible();
  await page.setViewportSize({ width: 1280, height: 720 });
  await expect(page.getByRole('alert').filter({ hasText: '窗口尺寸低于最小要求' })).toBeHidden();
});
