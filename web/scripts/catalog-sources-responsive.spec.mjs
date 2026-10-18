import { expect, test } from 'playwright/test'

import { loginSession } from './session-auth.mjs'

const baseUrl = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const token = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const name = '中文目录镜像-abcdefghijklmnopqrstuvwxyz'
const url = `https://fixture.example/${'catalog-path-'.repeat(8)}all.json`
const error = `同步失败：${'upstream-error-'.repeat(12)}`

for (const width of [320, 375, 639]) {
  test(`catalog controls remain reachable at ${width}px`, async ({
    context,
    page,
  }) => {
    await page.setViewportSize({ width, height: 1000 })
    await loginSession(context, { baseUrl, token })
    await context.addInitScript(() => {
      localStorage.setItem('i18nextLng', 'zh-CN')
    })
    const sources = [name, '备用目录'].map((name, index) => ({
      id: index + 1,
      name,
      url,
      type: 'custom',
      enabled: false,
      sortOrder: index,
      lastCount: 0,
      lastSuccessAt: null,
      lastAttemptAt: '2026-10-18T00:00:00Z',
      lastError: error,
    }))
    await page.route('**/api/models/catalog-sync', (route) =>
      route.fulfill({
        json: {
          enabled: false,
          autoSync: false,
          intervalMin: 0,
          snapshot: { source: '', fetchedAt: null, models: 0 },
          sources,
        },
      })
    )
    await page.route('**/api/models/catalog-sources/1', (route) =>
      route.fulfill({ json: { source: sources[0] } })
    )
    await page.goto(`${baseUrl}/settings/proxy-models/catalog-sources`)
    const row = page.getByRole('row').filter({ hasText: name })
    await expect(row).toBeVisible()
    for (const control of await row.locator('button,[role=switch]').all()) {
      await expect(control).toBeVisible()
      const bounds = await control.boundingBox()
      expect(bounds.x).toBeGreaterThanOrEqual(0)
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
    }
    const layout = await row.evaluate((row) => {
      const table = row.closest('[data-slot=table-container]')
      return {
        tableWidth: table.clientWidth,
        tableScrollWidth: table.scrollWidth,
        pageWidth: innerWidth,
        pageScrollWidth: document.documentElement.scrollWidth,
      }
    })
    expect(layout.tableScrollWidth).toBeLessThanOrEqual(layout.tableWidth)
    expect(layout.pageScrollWidth).toBeLessThanOrEqual(layout.pageWidth)
    for (const text of [row.getByText(url), row.getByText(error)]) {
      const box = await text.evaluate((node) => ({
        scrollHeight: node.scrollHeight,
        clientHeight: node.clientHeight,
        whiteSpace: getComputedStyle(node).whiteSpace,
      }))
      expect(box.whiteSpace).not.toBe('nowrap')
      expect(box.scrollHeight).toBeLessThanOrEqual(box.clientHeight + 1)
    }
    const handle = row.getByRole('button', { name: `调整 ${name} 顺序` })
    await handle.focus()
    await handle.press('Enter')
    await handle.press('ArrowDown')
    const update = page.waitForRequest((request) => request.method() === 'PUT')
    await handle.press('Space')
    expect((await update).postDataJSON()).toEqual({ sortOrder: 1 })
    await row.getByRole('button', { name: '编辑', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await expect(
      page.getByRole('dialog').getByLabel('URL', { exact: true })
    ).toHaveValue(url)
  })
}
