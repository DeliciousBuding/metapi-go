#!/usr/bin/env node
// Isolated production-binary + real Chromium acceptance. Never points at an
// existing deployment: every run owns a fresh localhost server and temp DB.
// Build first, then METAPI_BINARY=/absolute/server node scripts/user-portal-smoke.mjs.
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { once } from 'node:events'
import { createWriteStream } from 'node:fs'
import { mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import path from 'node:path'

import { chromium } from 'playwright'

const binary = process.env.METAPI_BINARY
if (!binary || !path.isAbsolute(binary))
  throw new Error(
    'METAPI_BINARY must name an absolute, freshly built server binary'
  )
const output = await mkdtemp(
  path.join(
    process.env.USER_PORTAL_OUTPUT_DIR ?? tmpdir(),
    'metapi-user-smoke-'
  )
)
const token = 'fixture-admin-' + randomBytes(20).toString('hex')
const password = 'fixture-user-' + randomBytes(16).toString('hex')
let upstreamCalls = 0
const upstream = createServer(async (req, res) => {
  let raw = ''
  for await (const part of req) raw += part
  const body = JSON.parse(raw || '{}')
  assert.equal(req.headers.authorization, 'Bearer synthetic-upstream-only')
  assert.ok(
    body.max_tokens > 0 || body.max_completion_tokens > 0,
    'user request gets a bounded output limit'
  )
  upstreamCalls++
  res.writeHead(200, { 'Content-Type': 'application/json' })
  res.end(
    JSON.stringify({
      id: 'fixture-chat',
      object: 'chat.completion',
      model: 'fixture-model',
      choices: [
        {
          index: 0,
          message: { role: 'assistant', content: 'personal-call-ok' },
          finish_reason: 'stop',
        },
      ],
      usage: { prompt_tokens: 10, completion_tokens: 4, total_tokens: 14 },
    })
  )
})
upstream.listen(0, '127.0.0.1')
await once(upstream, 'listening')
const upstreamURL = 'http://127.0.0.1:' + upstream.address().port
const reservation = createServer()
reservation.listen(0, '127.0.0.1')
await once(reservation, 'listening')
const port = reservation.address().port
await new Promise((resolve) => reservation.close(resolve))
const base = 'http://127.0.0.1:' + port
const log = createWriteStream(path.join(output, 'server.log'))
const server = spawn(binary, [], {
  cwd: output,
  env: {
    ...process.env,
    AUTH_TOKEN: token,
    HOST: '127.0.0.1',
    PORT: String(port),
    DATA_DIR: path.join(output, 'data'),
    DB_TYPE: 'sqlite',
    DB_URL: '',
    CHECKIN_ENABLED: 'false',
    PRICING_CATALOG_ENABLED: 'false',
    PROXY_LOG_ASYNC: 'false',
    ADMIN_SESSION_COOKIE_SECURE: 'false',
  },
})
server.stdout.pipe(log)
server.stderr.pipe(log)
let browser
const checks = []
function pass(name) {
  checks.push(name)
}
async function admin(url, method = 'GET', body) {
  const response = await fetch(base + url, {
    method,
    headers: {
      Authorization: 'Bearer ' + token,
      'X-Admin-Confirm-Token': token,
      'Content-Type': 'application/json',
      'X-External-Origin-Key': 'user-smoke',
    },
    ...(method === 'GET' || body === undefined
      ? {}
      : { body: JSON.stringify(body) }),
  })
  const text = await response.text()
  if (!response.ok)
    throw new Error(
      'Fixture administrator request ' +
        method +
        ' ' +
        url +
        ' failed: ' +
        response.status +
        ' ' +
        text
    )
  return JSON.parse(text)
}
try {
  for (let attempt = 0; attempt < 150; attempt++) {
    if (server.exitCode !== null)
      throw new Error('Fixture server exited before readiness')
    try {
      if ((await fetch(base + '/ready')).ok) break
    } catch {}
    if (attempt === 149) throw new Error('Fixture server did not become ready')
    await new Promise((resolve) => setTimeout(resolve, 200))
  }
  await admin('/api/settings/backup/import', 'POST', {
    version: '1.4',
    timestamp: '2026-10-21T00:00:00Z',
    channels: [
      {
        id: 1,
        type: 'openai',
        name: 'Isolated fixture',
        base_url: upstreamURL,
        credentials: { apiKey: 'synthetic-upstream-only' },
        supported_models: ['fixture-model'],
        endpoints: [
          {
            api_format: 'openai/chat_completions',
            path: '/v1/chat/completions',
          },
        ],
      },
    ],
    models: [
      {
        id: 1,
        model_id: 'personal-model',
        status: 'enabled',
        settings: {
          associations: [
            {
              type: 'channel_model',
              channelModel: { channelId: 1, modelId: 'fixture-model' },
            },
          ],
        },
      },
    ],
  })
  const routeRows = await admin('/api/routes/lite')
  const route = routeRows.find((row) => row.modelPattern === 'personal-model')
  assert.ok(route, 'import produced a public route')
  const group = await admin('/api/admin/user-groups', 'POST', {
    slug: 'personal-fixture',
    name: 'Personal group',
    enabled: true,
  })
  const emptyGroup = await admin('/api/admin/user-groups', 'POST', {
    slug: 'empty-fixture',
    name: 'Empty group',
    enabled: true,
  })
  await admin('/api/admin/user-groups/' + group.id + '/routes', 'POST', {
    routeId: route.id,
  })
  await admin('/api/admin/user-groups/' + group.id + '/pricing', 'PUT', {
    multiplier: 2,
  })
  const user = await admin('/api/admin/users', 'POST', {
    username: 'person-one',
    displayName: 'Person One',
    groupId: group.id,
    enabled: true,
  })
  const other = await admin('/api/admin/users', 'POST', {
    username: 'person-two',
    displayName: 'Person Two',
    groupId: group.id,
    enabled: true,
  })
  await admin('/api/admin/users/' + user.id + '/budget', 'PUT', {
    limit: 1,
    unlimited: false,
  })
  await admin('/api/admin/users/' + other.id + '/budget', 'PUT', {
    limit: 1,
    unlimited: false,
  })
  const activation = await admin(
    '/api/admin/users/' + user.id + '/activation',
    'POST',
    {}
  )
  const otherActivation = await admin(
    '/api/admin/users/' + other.id + '/activation',
    'POST',
    {}
  )
  browser = await chromium.launch({ headless: true })
  const context = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 1440, height: 1000 },
  })
  await context.addInitScript(() => localStorage.setItem('i18nextLng', 'en'))
  const page = await context.newPage()
  const crashes = []
  page.on('pageerror', (error) => crashes.push(error.message))
  page.on('console', (message) => {
    if (
      message.type() === 'error' &&
      !message.text().includes('401 (Unauthorized)')
    )
      crashes.push(message.text())
  })
  await page.goto(base + '/user')
  await page
    .getByRole('button', { name: 'Activate or reset my account' })
    .click()
  await page.getByLabel('Username', { exact: true }).fill(user.username)
  await page.getByLabel('Activation / reset code').fill(activation.code)
  await page.getByLabel('New password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Set password and sign in' }).click()
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
  pass('one-time activation through the real browser')
  assert.equal(
    (await context.cookies()).find((c) => c.name === 'metapi_user_session')
      ?.httpOnly,
    true
  )
  const adminDenied = await context.request.get(base + '/api/admin/users')
  assert.equal(adminDenied.status(), 401)
  pass('ordinary user cookie cannot read administrator API')
  await page.getByRole('link', { name: 'Models', exact: true }).click()
  await page.getByText('personal-model', { exact: true }).waitFor()
  await page.getByRole('link', { name: 'API keys', exact: true }).click()
  await page.getByRole('button', { name: 'Create key', exact: true }).click()
  await page.getByLabel('Key name', { exact: true }).fill('Browser client')
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Create key', exact: true })
    .click()
  const secret = await page
    .getByLabel('Secret API key', { exact: true })
    .inputValue()
  assert.match(secret, /^sk-/)
  assert.equal(
    await page
      .getByLabel('Secret API key', { exact: true })
      .getAttribute('type'),
    'password'
  )
  const storage = await page.evaluate(() =>
    JSON.stringify({
      local: { ...localStorage },
      session: { ...sessionStorage },
      url: location.href,
    })
  )
  assert.ok(
    !storage.includes(secret) &&
      !storage.includes(activation.code) &&
      !storage.includes(password)
  )
  await page.getByRole('button', { name: 'I have saved the key' }).click()
  await page.getByText('Browser client', { exact: true }).waitFor()
  pass('create and securely dismiss one-time personal Key')
  async function invoke(want = 200) {
    const response = await fetch(base + '/v1/chat/completions', {
      method: 'POST',
      headers: {
        Authorization: 'Bearer ' + secret,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        model: 'personal-model',
        messages: [{ role: 'user', content: 'hello' }],
      }),
    })
    const body = await response.json()
    assert.equal(response.status, want, JSON.stringify(body))
    return body
  }
  const completion = await invoke()
  assert.equal(completion.choices[0].message.content, 'personal-call-ok')
  assert.equal(upstreamCalls, 1)
  const budget = await (
    await context.request.get(base + '/api/user/budget')
  ).json()
  assert.equal(budget.used, 0.000028)
  assert.equal(budget.reserved, 0)
  pass('real HTTP relay and single user token charge at group multiplier')
  await page.getByRole('link', { name: 'Requests', exact: true }).click()
  await page.getByRole('link', { name: 'personal-model', exact: true }).click()
  await page
    .getByRole('heading', { name: 'Request details', exact: true })
    .waitFor()
  await page.screenshot({
    path: path.join(output, 'request-detail-desktop.png'),
  })
  const records = await (
    await context.request.get(base + '/api/user/requests')
  ).json()
  assert.equal(records.items.length, 1)
  const requestID = records.items[0].id
  const otherContext = await browser.newContext()
  assert.equal(
    (
      await otherContext.request.post(base + '/api/user/auth/activate', {
        data: {
          username: other.username,
          code: otherActivation.code,
          password,
        },
      })
    ).status(),
    200
  )
  const otherKeys = await (
    await otherContext.request.get(base + '/api/user/keys')
  ).json()
  const otherRecords = await (
    await otherContext.request.get(base + '/api/user/requests')
  ).json()
  assert.equal(otherKeys.items.length, 0)
  assert.equal(otherRecords.total, 0)
  assert.equal(
    (
      await otherContext.request.get(base + '/api/user/requests/' + requestID)
    ).status(),
    404
  )
  pass('other user cannot read Keys, totals or request detail')
  await admin('/api/admin/users/' + user.id, 'PUT', {
    displayName: 'Renamed',
    groupId: emptyGroup.id,
    enabled: true,
  })
  await invoke(403)
  const ownHistory = await (
    await context.request.get(base + '/api/user/requests/' + requestID)
  ).json()
  assert.equal(ownHistory.id, requestID)
  await admin('/api/admin/users/' + user.id, 'PUT', {
    displayName: 'Renamed',
    groupId: group.id,
    enabled: false,
  })
  await invoke(403)
  assert.equal(
    (await context.request.get(base + '/api/user/session')).status(),
    401
  )
  assert.equal(upstreamCalls, 1)
  pass(
    'existing Key follows regroup and disablement; history ownership persists'
  )
  await admin('/api/admin/users/' + user.id, 'PUT', {
    displayName: 'Renamed',
    groupId: group.id,
    enabled: true,
  })
  await page.goto(base + '/user?view=overview')
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
  for (const view of ['overview', 'models', 'keys', 'requests', 'account']) {
    await page.goto(base + '/user?view=' + view)
    await page.getByRole('heading', { level: 1 }).waitFor()
    await page.screenshot({
      path: path.join(output, 'user-' + view + '-desktop.png'),
    })
    await page.setViewportSize({ width: 375, height: 812 })
    await page.screenshot({
      path: path.join(output, 'user-' + view + '-mobile.png'),
    })
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth
      ),
      'mobile document overflow: ' + view
    )
    // Run the audit through browser automation, not an inline DOM script;
    // keep the production CSP enabled for the actual application journey.
    await page.evaluate(
      await readFile(
        new URL('../node_modules/axe-core/axe.min.js', import.meta.url),
        'utf8'
      )
    )
    const violations = await page.evaluate(async () => {
      const r = await window.axe.run(document, {
        runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa'] },
      })
      return r.violations
        .filter((v) => ['critical', 'serious'].includes(v.impact))
        .map((v) => v.id)
    })
    assert.deepEqual(violations, [], 'axe ' + view)
    await page.setViewportSize({ width: 1440, height: 1000 })
  }
  pass(
    'all five personal views desktop/mobile, no overflow, axe serious/critical zero'
  )
  await page.getByRole('link', { name: 'Account', exact: true }).click()
  await page.getByLabel('Current password', { exact: true }).fill(password)
  const nextPassword = 'changed-' + password
  await page.getByLabel('New password', { exact: true }).fill(nextPassword)
  await page
    .getByRole('button', { name: 'Change password', exact: true })
    .click()
  await page
    .getByRole('heading', { name: 'User sign in', exact: true })
    .waitFor()
  pass('password change ends user session in browser')
  assert.deepEqual(crashes, [], 'browser runtime errors')
  await writeFile(
    path.join(output, 'result.json'),
    JSON.stringify({ checks, upstreamCalls, passed: true }, null, 2)
  )
  console.log(JSON.stringify({ passed: true, checks, output }, null, 2))
} catch (error) {
  await writeFile(
    path.join(output, 'result.json'),
    JSON.stringify({ checks, passed: false, error: String(error) }, null, 2)
  )
  console.error('User portal smoke failed. Evidence: ' + output)
  throw error
} finally {
  await browser?.close()
  server.kill('SIGTERM')
  await new Promise((resolve) => upstream.close(resolve))
  log.end()
}
