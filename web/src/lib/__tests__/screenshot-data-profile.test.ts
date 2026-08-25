import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { resolve } from 'node:path'

import { afterEach, describe, expect, it } from 'vitest'

const servers: Array<ReturnType<typeof createServer>> = []

afterEach(async () => {
  await Promise.all(
    servers
      .splice(0)
      .map((server) => new Promise<void>((done) => server.close(() => done())))
  )
})

async function startProfileServer(profile: 'empty' | 'seeded') {
  const server = createServer((request, response) => {
    response.setHeader('content-type', 'application/json')
    if (request.url === '/api/sites') {
      response.end(JSON.stringify(profile === 'empty' ? [] : [{ id: 1 }]))
      return
    }
    if (request.url?.startsWith('/api/accounts')) {
      response.end(
        JSON.stringify({
          items: profile === 'empty' ? [] : [{ id: 1 }],
          total: profile === 'empty' ? 0 : 1,
        })
      )
      return
    }
    response.statusCode = 404
    response.end('{}')
  })
  servers.push(server)
  await new Promise<void>((done) => server.listen(0, '127.0.0.1', done))
  const address = server.address()
  if (!address || typeof address === 'string')
    throw new Error('missing test port')
  return `http://127.0.0.1:${address.port}`
}

async function runProfileCheck(baseURL: string, expected: 'empty' | 'seeded') {
  return new Promise<{ code: number | null; output: string }>((done) => {
    const child = spawn(
      process.execPath,
      [resolve('scripts/screenshot-scan.mjs')],
      {
        cwd: process.cwd(),
        env: {
          ...process.env,
          BASE_URL: baseURL,
          AUTH_TOKEN: 'test-token',
          EXPECTED_DATA_PROFILE: expected,
          PROFILE_CHECK_ONLY: '1',
        },
        stdio: ['ignore', 'pipe', 'pipe'],
      }
    )
    let output = ''
    child.stdout.on('data', (chunk) => (output += String(chunk)))
    child.stderr.on('data', (chunk) => (output += String(chunk)))
    child.on('close', (code) => done({ code, output }))
  })
}

describe('screenshot data profile preflight', () => {
  it.each(['empty', 'seeded'] as const)(
    'accepts a matching %s runtime before capture',
    async (profile) => {
      const baseURL = await startProfileServer(profile)
      const result = await runProfileCheck(baseURL, profile)
      expect(result.code).toBe(0)
      expect(result.output).toContain(`data profile ${profile} verified`)
      expect(result.output).toContain('capture skipped')
    }
  )

  it('rejects a mislabeled empty capture before Chromium starts', async () => {
    const baseURL = await startProfileServer('seeded')
    const result = await runProfileCheck(baseURL, 'empty')
    expect(result.code).not.toBe(0)
    expect(result.output).toContain(
      'data profile mismatch: expected empty, observed seeded'
    )
    expect(result.output).not.toContain('capture skipped')
  })
})
