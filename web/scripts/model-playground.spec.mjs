import { readFile } from 'node:fs/promises'

import { expect, test } from 'playwright/test'

import { loginSession } from './session-auth.mjs'

const baseUrl = process.env.BASE_URL ?? 'http://127.0.0.1:4099'
const token = process.env.AUTH_TOKEN ?? 'dev-admin-token-123'
const model = 'playground-fixture-model'
const channels = Array.from({ length: 12 }, (_, index) => ({
  id: index + 1,
  name: `测试渠道 ${index + 1}`,
  site: { id: 1, name: '示例站点' },
  enabled: true,
  models: model,
}))

const configurations = [
  ['zh-CN', 'light', 1440],
  ['zh-CN', 'dark', 1440],
  ['zh-CN', 'light', 375],
  ['zh-CN', 'dark', 375],
  ['en', 'light', 1440],
]

for (const [locale, theme, width] of configurations) {
  test(`chat playground ${locale} ${theme} ${width}px`, async ({
    context,
    page,
  }, testInfo) => {
    const zh = locale === 'zh-CN'
    const errors = []
    const requests = []
    page.on('pageerror', (error) => errors.push(error.message))
    await page.setViewportSize({ width, height: width === 375 ? 667 : 1000 })
    await loginSession(context, { baseUrl, token })
    await context.addInitScript(
      (locale) => localStorage.setItem('i18nextLng', locale),
      locale
    )
    await context.addCookies([
      { name: 'vite-ui-theme', value: theme, url: baseUrl },
    ])
    await page.route('**/api/models/marketplace*', (route) =>
      route.fulfill({ json: { models: [{ name: model }] } })
    )
    await page.route('**/api/channels', (route) =>
      route.fulfill({ json: { items: channels } })
    )
    await page.route('**/api/test/chat', async (route) => {
      const payload = route.request().postDataJSON()
      requests.push(payload)
      const prompt = payload.messages.at(-1).content
      const failed = prompt === '模拟失败'
      const content =
        prompt === '长回复'
          ? `## 代码示例\n\n${'这是一段用于检查阅读位置和中文排版的回复。\n\n'.repeat(30)}\n\n\`\`\`ts\nconst value = "${'long-value-'.repeat(30)}"\n\`\`\``
          : `回复：${prompt}`
      await route.fulfill({
        json: {
          success: !failed,
          statusCode: failed ? 503 : 200,
          latencyMs: 128,
          error: failed ? '模拟上游暂不可用' : null,
          truncatedBody: JSON.stringify({
            choices: [
              {
                message: {
                  content: failed ? '' : content,
                  reasoning_content: '这一轮的推理内容。',
                },
                finish_reason: 'stop',
              },
            ],
            usage: {
              prompt_tokens: 12,
              completion_tokens: 28,
              total_tokens: 40,
            },
          }),
        },
      })
    })
    await page.goto(`${baseUrl}/model-tester?model=${model}`, {
      waitUntil: 'networkidle',
    })
    const settings = page.getByRole('button', {
      name: zh ? '对话设置' : 'Chat settings',
      exact: true,
    })
    await page.screenshot({
      path: testInfo.outputPath('empty.png'),
      animations: 'disabled',
    })
    await settings.click()
    const channel = page.getByRole('combobox', {
      name: zh ? '渠道' : 'Channel',
      exact: true,
    })
    await channel.click()
    await page.getByRole('option', { name: /测试渠道 1 ·/ }).click()
    await page.screenshot({
      path: testInfo.outputPath('settings.png'),
      animations: 'disabled',
    })
    await page
      .getByRole('button', { name: zh ? '关闭' : 'Close', exact: true })
      .first()
      .click()
    await expect(page.getByRole('dialog')).not.toBeVisible()
    const prompt = page.getByRole('textbox', {
      name: zh ? '提示词' : 'Prompt',
      exact: true,
    })
    const send = page.getByRole('button', {
      name: zh ? '发送' : 'Send',
      exact: true,
    })
    const log = page.getByRole('log', { name: zh ? '对话' : 'Conversation' })
    await prompt.fill('你好，简单介绍一下你能做什么。')
    await send.click()
    await expect(
      log.getByText('回复：你好，简单介绍一下你能做什么。', { exact: true })
    ).toBeVisible()
    await page.screenshot({
      path: testInfo.outputPath('normal-conversation.png'),
      animations: 'disabled',
    })
    await prompt.fill('再举一个例子。')
    await prompt.press('Control+Enter')
    await expect(
      log.getByText('回复：再举一个例子。', { exact: true })
    ).toBeVisible()
    await page.screenshot({
      path: testInfo.outputPath('normal-multiturn.png'),
      animations: 'disabled',
    })
    await page
      .getByRole('button', {
        name: zh ? '清空会话' : 'Clear session',
        exact: true,
      })
      .click()
    await page
      .getByRole('button', { name: zh ? '清空' : 'Clear', exact: true })
      .click()
    await expect(log).not.toBeAttached()
    await prompt.fill('长回复')
    await send.click()
    await expect(log.getByRole('heading', { name: '代码示例' })).toBeAttached()
    await expect(prompt).toHaveValue('')
    await expect(prompt).toBeFocused()
    await expect(send).toBeInViewport()
    expect(
      await log.evaluate((node) => node.scrollHeight > node.clientHeight)
    ).toBe(true)
    await log.evaluate((node) => {
      node.scrollTop = 0
      node.dispatchEvent(new Event('scroll'))
    })
    await expect(
      page.getByRole('button', { name: zh ? '回到最新' : 'Jump to latest' })
    ).toBeVisible()
    await prompt.fill('继续')
    await prompt.press('Control+Enter')
    await expect(log.getByText('回复：继续', { exact: true })).toBeAttached()
    expect(await log.evaluate((node) => node.scrollTop)).toBe(0)
    expect(requests.at(-1).messages.map((message) => message.role)).toEqual([
      'user',
      'assistant',
      'user',
    ])
    await page
      .getByRole('button', { name: zh ? '回到最新' : 'Jump to latest' })
      .click()
    await expect(log.getByText('回复：继续', { exact: true })).toBeInViewport()
    await log
      .locator('summary')
      .filter({ hasText: zh ? '响应详情' : 'Response details' })
      .last()
      .click()
    await page.screenshot({
      path: testInfo.outputPath('conversation.png'),
      animations: 'disabled',
    })
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth
      )
    ).toBe(true)
    await prompt.fill('模拟失败')
    await send.click()
    await expect(log.getByRole('alert')).toHaveText('模拟上游暂不可用')
    await expect(prompt).toHaveValue('模拟失败')
    await settings.click()
    await page
      .getByRole('switch', {
        name: zh ? '对比渠道' : 'Compare channels',
        exact: true,
      })
      .click()
    await page.getByRole('checkbox', { name: /测试渠道 1 ·/ }).check()
    await page.getByRole('checkbox', { name: /测试渠道 2 ·/ }).check()
    await page
      .getByRole('button', { name: zh ? '关闭' : 'Close', exact: true })
      .first()
      .click()
    await expect(page.getByRole('dialog')).not.toBeVisible()
    await prompt.fill('对比')
    await send.click()
    await expect(
      page
        .locator('form')
        .getByText(zh ? '2 成功 / 0 失败' : '2 succeeded / 0 failed', {
          exact: true,
        })
    ).toBeVisible()
    await expect(send).toBeInViewport()
    await page.screenshot({
      path: testInfo.outputPath('comparison.png'),
      animations: 'disabled',
    })
    if (width === 375) {
      await page.setViewportSize({ width, height: 667 })
      await expect(page.locator('[data-sonner-toast]')).toHaveCount(0)
      await settings.click()
      await expect(page.getByRole('dialog')).toBeInViewport()
      await expect(page.getByRole('dialog')).toHaveCSS('width', '375px')
      await page.screenshot({
        path: testInfo.outputPath('settings-667.png'),
        animations: 'disabled',
      })
      await page
        .getByRole('button', { name: zh ? '关闭' : 'Close', exact: true })
        .first()
        .click()
      await expect(page.getByRole('dialog')).not.toBeVisible()
      await expect(send).toBeInViewport()
      await expect(prompt).toBeInViewport()
      const sendBounds = await send.boundingBox()
      expect(sendBounds.y + sendBounds.height).toBeLessThanOrEqual(667)
      expect(
        await page
          .locator('form')
          .evaluate((node) => node.scrollHeight <= node.clientHeight + 1)
      ).toBe(true)
    }
    await page.evaluate(
      await readFile(
        new URL('../node_modules/axe-core/axe.min.js', import.meta.url),
        'utf8'
      )
    )
    const violations = await page.locator('form').evaluate(async (node) =>
      (
        await axe.run(node, {
          runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa'] },
        })
      ).violations.map((violation) => ({
        id: violation.id,
        nodes: violation.nodes.map((node) => node.target),
      }))
    )
    expect(violations).toEqual([])
    expect(errors).toEqual([])
  })
}
