import { mkdirSync } from 'node:fs'
import { join } from 'node:path'

import { expect, test } from 'playwright/test'

const baseUrl = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const outDir = process.env.OUT_DIR
const model = 'public-coding-model'
const future = '2999-01-01T00:00:00Z'
const candidates = Array.from({ length: 42 }, (_, index) => ({
  channelId: index + 101,
  candidateRef: {
    kind: 'account',
    id: index + 101,
    accountId: index + 1,
    siteId: 1,
  },
  label: `New API · team-${String(index + 1).padStart(2, '0')}@example.test`,
  actualModel: `upstream-model-${index + 1}`,
  priority: index,
  weight: index === 0 ? 80 : 20,
  eligibility: ['eligible', 'blocked', 'notEvaluated'][index % 3],
  blockers:
    index % 3 === 1
      ? [
          {
            code: index === 40 ? 'source_model_mismatch' : 'cooldown',
            message: 'Synthetic filter blocker',
          },
        ]
      : [],
  cooldownUntil: index % 3 === 1 && index !== 40 ? future : null,
  reasonText: index === 41 ? 'Unknown evaluation context retained' : null,
  probabilityEvaluated: false,
}))
const decision = {
  mode: 'adminPreview',
  requestedModel: model,
  actualModel: model,
  matched: true,
  routeId: 9,
  modelPattern: model,
  checkedAt: '2026-10-21T10:00:00Z',
  requiredUpstreamProtocol: 2,
  routingStrategy: 'weighted',
  candidates,
}
const summary = {
  id: 9,
  modelPattern: model,
  displayName: null,
  displayIcon: null,
  routeMode: 'pattern',
  enabled: true,
  routingStrategy: 'weighted',
  channelCount: 42,
  enabledChannelCount: 42,
  siteNames: ['New API'],
  decisionSnapshot: null,
  decisionRefreshedAt: null,
}

async function fixture(context, language, theme) {
  const state = {
    mode: 'normal',
    recoverFailed: false,
    clears: [],
    decisionReads: 0,
  }
  await context.addCookies([
    { name: 'vite-ui-theme', value: theme, url: baseUrl },
  ])
  await context.addInitScript(
    (lang) => localStorage.setItem('i18nextLng', lang),
    language
  )
  await context.route('**/api/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const json = (body, status = 200) => route.fulfill({ json: body, status })
    if (path === '/api/auth/session')
      return json({ authenticated: true, expiresAt: future, source: 'session' })
    if (path === '/api/routes/summary') return json([summary])
    if (path === '/api/routes/decision') {
      state.decisionReads += 1
      if (state.mode === 'failure')
        return json(
          { success: false, message: 'Synthetic routing check failure' },
          500
        )
      if (state.mode === 'empty')
        return json({
          success: true,
          decision: { ...decision, candidates: [] },
        })
      if (state.mode === 'noRoute')
        return json({
          success: true,
          decision: {
            ...decision,
            matched: false,
            routeId: undefined,
            modelPattern: undefined,
            candidates: [],
          },
        })
      return json({ success: true, decision })
    }
    const clear = path.match(/^\/api\/channels\/(\d+)\/cooldown\/clear$/)
    if (clear) {
      state.clears.push(Number(clear[1]))
      return state.recoverFailed
        ? json({ success: false, message: 'Synthetic recovery failure' }, 500)
        : json({ success: true })
    }
    if (path === '/api/sites') return json([])
    if (path === '/api/accounts')
      return json({
        accounts: [],
        sites: [],
        generatedAt: '2026-10-21T10:00:00Z',
      })
    if (path === '/api/channels' || path === '/api/routes/9/channels')
      return json([])
    if (path === '/api/downstream-keys') return json({ items: [] })
    if (path === '/api/models/token-candidates')
      return json({ models: {}, modelsWithoutToken: {} })
    // Shell data is unrelated to the diagnostic task.
    return json({ success: true, items: [], unreadCount: 0 })
  })
  return state
}

async function screenshot(page, name) {
  if (!outDir) return
  mkdirSync(outDir, { recursive: true })
  await page.screenshot({ path: join(outDir, `${name}.png`), fullPage: true })
}

for (const { language, theme, width } of [
  { language: 'zh-CN', theme: 'light', width: 1440 },
  { language: 'zh-CN', theme: 'dark', width: 1440 },
  { language: 'zh-CN', theme: 'dark', width: 390 },
  { language: 'en', theme: 'light', width: 1440 },
]) {
  test(`candidate inventory ${language} ${theme} ${width}px`, async ({
    context,
    page,
  }) => {
    const zh = language === 'zh-CN'
    const state = await fixture(context, language, theme)
    await page.setViewportSize({ width, height: 1000 })
    const errors = []
    page.on('pageerror', (error) => errors.push(error.message))
    await page.goto(
      `${baseUrl}/token-routes?workspaceModel=${model}&workspaceEndpoint=%2Fv1%2Fchat%2Fcompletions`
    )
    const list = page.getByRole('list', {
      name: zh ? '路由候选' : 'Routing candidates',
    })
    await expect(list.getByRole('listitem')).toHaveCount(20)
    await expect(list.getByRole('listitem').first()).toContainText('team-01@')
    const href = page.url()
    const search = page.getByRole('textbox', {
      name: zh ? '查找候选' : 'Find candidates',
    })
    await screenshot(page, `${language}-${theme}-${width}-long-list`)
    await page
      .getByRole('button', {
        name: zh ? '受阻（14）' : 'Blocked (14)',
        exact: true,
      })
      .click()
    await expect(list.getByRole('listitem')).toHaveCount(14)
    await search.fill('team-41')
    await expect(list.getByRole('listitem')).toHaveCount(1)
    await expect(list).toContainText(
      zh ? '配置优先级：40' : 'Configured priority: 40'
    )
    await expect(list).toContainText(
      zh ? '配置权重：20' : 'Configured weight: 20'
    )
    expect(page.url()).toBe(href)
    await list.scrollIntoViewIfNeeded()
    await screenshot(page, `${language}-${theme}-${width}-located-blocker`)
    await page
      .getByRole('button', {
        name: zh ? '编辑路由配置' : 'Edit route configuration',
        exact: true,
      })
      .click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await expect(
      dialog.getByRole('textbox', {
        name: zh ? '模型匹配规则' : 'Model match rule',
        exact: true,
      })
    ).toHaveValue(model)
    await page.keyboard.press('Escape')
    await expect(dialog).not.toBeVisible()
    await search.fill('not-found')
    await expect(
      page.getByText(
        zh ? '没有符合筛选条件的候选。' : 'No candidates match these filters.',
        { exact: true }
      )
    ).toBeVisible()
    await screenshot(page, `${language}-${theme}-${width}-no-matches`)
    await page
      .getByRole('button', {
        name: zh ? '重置候选筛选' : 'Reset candidate filters',
      })
      .click()
    await expect(list.getByRole('listitem')).toHaveCount(20)
    await search.fill('team-02')
    state.recoverFailed = true
    const clear = list.getByRole('button', {
      name: zh ? '清除冷却' : 'Clear cooldown',
      exact: true,
    })
    await clear.click()
    await expect.poll(() => state.clears).toEqual([102])
    await expect(clear).toBeEnabled()
    await expect(list).toContainText(zh ? '冷却至' : 'Cooling until')
    await screenshot(page, `${language}-${theme}-${width}-recovery-failure`)
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth
      )
    ).toBe(true)
    expect(errors).toEqual([])
  })
}

test('current diagnostic failure clears earlier candidates; empty and no-route stay distinct', async ({
  context,
  page,
}) => {
  const state = await fixture(context, 'en', 'light')
  await page.goto(
    `${baseUrl}/token-routes?workspaceModel=${model}&workspaceEndpoint=%2Fv1%2Fchat%2Fcompletions`
  )
  await expect(
    page.getByText('New API · team-01@example.test', { exact: true })
  ).toBeVisible()
  state.mode = 'failure'
  await page
    .getByRole('button', { name: 'Check call path', exact: true })
    .click()
  await expect(
    page
      .getByRole('alert')
      .filter({ hasText: 'Call-path check failed. Please retry.' })
  ).toBeVisible()
  await expect(
    page.getByText('New API · team-01@example.test', { exact: true })
  ).toHaveCount(0)
  await screenshot(page, 'en-light-1440-check-failure')
  state.mode = 'empty'
  await page
    .getByRole('button', { name: 'Check call path', exact: true })
    .click()
  await expect(
    page.getByText('No candidates were returned for this route.', {
      exact: true,
    })
  ).toBeVisible()
  await screenshot(page, 'en-light-1440-empty-candidates')
  state.mode = 'noRoute'
  await page
    .getByRole('button', { name: 'Check call path', exact: true })
    .click()
  await expect(
    page.getByText('Check the client model or configure a matching route.', {
      exact: true,
    })
  ).toBeVisible()
  await screenshot(page, 'en-light-1440-no-route')
})
