import { describe, expect, it } from 'vitest'

import {
  OAUTH_START_DEFAULT_VALUES,
  oauthSearchSchema,
  oauthStartSchema,
  type OAuthStartValues,
} from '../lib/oauth-schema'

function validOAuthStart(): OAuthStartValues {
  return { ...OAUTH_START_DEFAULT_VALUES, provider: 'github' }
}

// ---------------------------------------------------------------------------
// happy path
// ---------------------------------------------------------------------------

describe('oauthStartSchema — happy path', () => {
  it('parses a minimal valid form (provider only)', () => {
    expect(oauthStartSchema.safeParse(validOAuthStart()).success).toBe(true)
  })

  it('allows a blank projectId', () => {
    expect(
      oauthStartSchema.safeParse({ ...validOAuthStart(), projectId: '' })
        .success
    ).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// provider required
// ---------------------------------------------------------------------------

describe('oauthStartSchema — provider', () => {
  it('rejects an empty provider with providerRequired', () => {
    const result = oauthStartSchema.safeParse({
      ...validOAuthStart(),
      provider: '',
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'oauth.form.errors.providerRequired'
    )
  })

  it('rejects a whitespace-only provider after trimming', () => {
    expect(
      oauthStartSchema.safeParse({ ...validOAuthStart(), provider: '   ' })
        .success
    ).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// proxyUrl refine
// ---------------------------------------------------------------------------

describe('oauthStartSchema — proxyUrl', () => {
  it('accepts an empty / http / https proxyUrl', () => {
    expect(
      oauthStartSchema.safeParse({ ...validOAuthStart(), proxyUrl: '' }).success
    ).toBe(true)
    expect(
      oauthStartSchema.safeParse({
        ...validOAuthStart(),
        proxyUrl: 'https://proxy.example',
      }).success
    ).toBe(true)
  })

  it.each([
    ['ftp scheme', 'ftp://x'],
    ['plain string', 'not-a-url'],
  ])('rejects %s with invalidProxyUrl', (_label, proxyUrl) => {
    const result = oauthStartSchema.safeParse({
      ...validOAuthStart(),
      proxyUrl,
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'oauth.form.errors.invalidProxyUrl'
    )
  })
})

// ---------------------------------------------------------------------------
// useSystemProxy boolean (no coerce)
// ---------------------------------------------------------------------------

describe('oauthStartSchema — useSystemProxy', () => {
  it('rejects a string flag (no coerce)', () => {
    const result = oauthStartSchema.safeParse({
      ...validOAuthStart(),
      useSystemProxy: 'true' as unknown as boolean,
    })
    expect(result.success).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// defaults
// ---------------------------------------------------------------------------

describe('OAUTH_START_DEFAULT_VALUES', () => {
  it('exposes the canonical default shape', () => {
    expect(OAUTH_START_DEFAULT_VALUES).toEqual({
      provider: '',
      projectId: '',
      proxyUrl: '',
      useSystemProxy: false,
    })
  })

  it('fails schema validation (provider empty)', () => {
    expect(oauthStartSchema.safeParse(OAUTH_START_DEFAULT_VALUES).success).toBe(
      false
    )
  })
})

// ---------------------------------------------------------------------------
// search schema
// ---------------------------------------------------------------------------

describe('oauthSearchSchema', () => {
  it('accepts page 0 and any status string (no enum)', () => {
    expect(
      oauthSearchSchema.parse({ page: '0', status: 'whatever' })
    ).toMatchObject({ page: 0, status: 'whatever' })
  })

  it('normalizes a comma-separated sort descriptor to a canonical string', () => {
    expect(
      oauthSearchSchema.parse({ sort: 'created:desc,model:asc' }).sort
    ).toBe('created:desc,model:asc')
  })

  it('returns undefined for empty sort (no URL noise)', () => {
    expect(oauthSearchSchema.parse({}).sort).toBeUndefined()
    expect(oauthSearchSchema.parse({ sort: '[]' }).sort).toBeUndefined()
  })

  it('falls back instead of throwing for out-of-range / non-string values', () => {
    const result = oauthSearchSchema.parse({
      page: 'abc',
      pageSize: '201',
      sort: 123,
      q: 42,
    })
    expect(result.page).toBe(0)
    expect(result.pageSize).toBe(20)
    expect(result.sort).toBeUndefined()
    expect(result.q).toBe(42)
  })
})
