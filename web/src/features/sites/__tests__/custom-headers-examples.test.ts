// metapi-go/features/sites — pure unit tests for the read-only customHeaders
// example snippets (#1132). The snippets are user-facing suggestions that the
// proxy will actually forward, so the contract is pinned here rather than
// discovered at runtime: what a site may inject is decided by
// platform.IsDeniedCustomHeader (data plane) and
// service.isReservedPlatformCustomHeader (assembly side).
import { describe, expect, it } from 'vitest'

import { CUSTOM_HEADERS_EXAMPLES } from '../lib/custom-headers-examples'

// Keep in step with those two Go predicates. An example suggesting a denied
// header would be dropped silently on the way upstream, so it must never ship.
const DENIED_HEADERS = new Set([
  'authorization',
  'host',
  'content-length',
  'content-type',
  'transfer-encoding',
  'accept-encoding',
  'connection',
  'cookie',
  'keep-alive',
  'te',
  'trailer',
  'trailers',
  'upgrade',
  'new-api-user',
])

describe('CUSTOM_HEADERS_EXAMPLES', () => {
  it('ships the three CLI clients with unique names and snippets', () => {
    expect(CUSTOM_HEADERS_EXAMPLES.map((e) => e.client)).toEqual([
      'Claude Code',
      'Codex CLI',
      'Gemini CLI',
    ])
    const clients = CUSTOM_HEADERS_EXAMPLES.map((e) => e.client)
    const snippets = CUSTOM_HEADERS_EXAMPLES.map((e) => e.snippet)
    // `client` doubles as the React key, so names must stay unique.
    expect(new Set(clients).size).toBe(clients.length)
    expect(new Set(snippets).size).toBe(snippets.length)
  })

  it('is valid JSON objects the field schema would accept', () => {
    for (const { client, snippet } of CUSTOM_HEADERS_EXAMPLES) {
      // Must satisfy the field's own `isEmptyOrValidJson` rule (lib/sites-schema):
      // a JSON object — never an array, a bare string or invalid JSON. Inserting
      // an example must not be able to put the form into an error state.
      const parsed: unknown = JSON.parse(snippet)
      expect(parsed, `${client}: must parse`).not.toBeNull()
      expect(parsed, `${client}: must be an object`).toBeTypeOf('object')
      expect(Array.isArray(parsed), `${client}: must not be an array`).toBe(
        false
      )
      expect(
        Object.keys(parsed as Record<string, unknown>).length,
        `${client}: must carry at least one header`
      ).toBeGreaterThan(0)
    }
  })

  it('only suggests headers a site is actually allowed to inject', () => {
    for (const { client, snippet } of CUSTOM_HEADERS_EXAMPLES) {
      for (const name of Object.keys(JSON.parse(snippet))) {
        const lower = name.toLowerCase()
        expect(
          DENIED_HEADERS.has(lower),
          `${client}: ${name} is not site-injectable`
        ).toBe(false)
        expect(
          lower.startsWith('proxy-'),
          `${client}: ${name} is a Proxy-* header`
        ).toBe(false)
      }
    }
  })

  it('stays obviously a placeholder and never bakes in a current version', () => {
    // #1132's core constraint. These CLIs re-release constantly, so a concrete
    // version rots into a wrong-but-authoritative value; a stale template is
    // worse than none because users treat it as the real one.
    for (const { client, snippet } of CUSTOM_HEADERS_EXAMPLES) {
      expect(snippet, `${client}: must contain a <PLACEHOLDER> token`).toMatch(
        /<[A-Z][A-Z_]*>/
      )
      expect(
        snippet,
        `${client}: must not bake in a concrete version`
      ).not.toMatch(/\d+\.\d+/)
    }
  })
})
