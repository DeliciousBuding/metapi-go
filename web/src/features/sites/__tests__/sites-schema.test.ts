import { describe, expect, it } from 'vitest'

import {
  SITE_FORM_DEFAULT_VALUES,
  siteFormSchema,
  sitesSearchSchema,
  type SiteFormValues,
} from '../lib/sites-schema'

function validOverrides(): Partial<SiteFormValues> {
  return {
    name: 'My Site',
    url: 'https://example.com',
  }
}

function validSiteForm(): SiteFormValues {
  return { ...SITE_FORM_DEFAULT_VALUES, ...validOverrides() }
}

// ---------------------------------------------------------------------------
// happy path
// ---------------------------------------------------------------------------

describe('siteFormSchema — happy path', () => {
  it('parses a minimal valid form', () => {
    const result = siteFormSchema.safeParse(validSiteForm())
    expect(result.success).toBe(true)
  })

  it('trims name and url before validating', () => {
    const result = siteFormSchema.parse({
      ...validSiteForm(),
      name: '  My Site  ',
      url: '  https://example.com  ',
    })
    expect(result.name).toBe('My Site')
    expect(result.url).toBe('https://example.com')
  })
})

// ---------------------------------------------------------------------------
// name + url required / bounds
// ---------------------------------------------------------------------------

describe('siteFormSchema — name', () => {
  const nameCases: Array<[string, string, string | undefined]> = [
    ['empty', '', 'sites.form.errors.nameRequired'],
    ['whitespace-only after trimming', '   ', undefined],
    ['over 120 chars', 'x'.repeat(121), 'sites.form.errors.nameTooLong'],
  ]

  it.each(nameCases)('rejects a %s name', (_label, name, message) => {
    const result = siteFormSchema.safeParse({ ...validSiteForm(), name })
    expect(result.success).toBe(false)
    if (result.success) return
    if (message) expect(result.error.issues[0]?.message).toBe(message)
  })
})

describe('siteFormSchema — url', () => {
  const badUrlCases: Array<[string, string, string]> = [
    ['empty', '', 'sites.form.errors.urlRequired'],
    ['ftp scheme', 'ftp://example.com', 'sites.form.errors.invalidUrl'],
    ['plain string', 'not a url', 'sites.form.errors.invalidUrl'],
    [
      'javascript scheme',
      'javascript:alert(1)',
      'sites.form.errors.invalidUrl',
    ],
  ]

  it.each(badUrlCases)(
    'rejects an invalid url (%s)',
    (_label, url, message) => {
      const result = siteFormSchema.safeParse({ ...validSiteForm(), url })
      expect(result.success).toBe(false)
      if (result.success) return
      expect(result.error.issues[0]?.message).toBe(message)
    }
  )

  it('accepts http and https', () => {
    expect(
      siteFormSchema.safeParse({ ...validSiteForm(), url: 'http://x' }).success
    ).toBe(true)
    expect(
      siteFormSchema.safeParse({ ...validSiteForm(), url: 'https://x' }).success
    ).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// optional URL fields (empty ok, non-http rejected)
// ---------------------------------------------------------------------------

describe('siteFormSchema — optional URLs', () => {
  it('accepts an empty externalCheckinUrl / proxyUrl', () => {
    expect(
      siteFormSchema.safeParse({ ...validSiteForm(), externalCheckinUrl: '' })
        .success
    ).toBe(true)
    expect(
      siteFormSchema.safeParse({ ...validSiteForm(), proxyUrl: '' }).success
    ).toBe(true)
  })

  it('rejects a non-http externalCheckinUrl with invalidUrlOrEmpty', () => {
    const result = siteFormSchema.safeParse({
      ...validSiteForm(),
      externalCheckinUrl: 'ftp://x',
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'sites.form.errors.invalidUrlOrEmpty'
    )
  })
})

// ---------------------------------------------------------------------------
// customHeaders JSON refine
// ---------------------------------------------------------------------------

describe('siteFormSchema — customHeaders', () => {
  it.each([
    ['an empty string', ''],
    ['a plain JSON object', '{"a":1}'],
  ])('accepts %s', (_label, customHeaders) => {
    expect(
      siteFormSchema.safeParse({ ...validSiteForm(), customHeaders }).success
    ).toBe(true)
  })

  it.each([
    ['array', '[1,2]'],
    ['null', 'null'],
    ['number', '123'],
    ['unparseable', '{"a":1,}'],
    ['plain text', 'abc'],
  ])('rejects %s with invalidJson', (_label, customHeaders) => {
    const result = siteFormSchema.safeParse({
      ...validSiteForm(),
      customHeaders,
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'sites.form.errors.invalidJson'
    )
  })
})

// ---------------------------------------------------------------------------
// numeric bounds + enum
// ---------------------------------------------------------------------------

describe('siteFormSchema — numerics + enum', () => {
  const numericBoundCases: Array<[string, Partial<SiteFormValues>, string]> = [
    [
      'a negative globalWeight',
      { globalWeight: -1 },
      'sites.form.errors.globalWeightMin',
    ],
    [
      'a non-integer maxConcurrency',
      { maxConcurrency: 1.5 },
      'sites.form.errors.maxConcurrencyInteger',
    ],
    [
      'a non-integer latency threshold',
      { postRefreshProbeLatencyThresholdMs: 1.5 },
      'sites.form.errors.latencyInteger',
    ],
  ]

  it.each(numericBoundCases)('rejects %s', (_label, overrides, message) => {
    const result = siteFormSchema.safeParse({
      ...validSiteForm(),
      ...overrides,
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(message)
  })

  it('accepts postRefreshProbeScope "all" and rejects unknown values', () => {
    expect(
      siteFormSchema.safeParse({
        ...validSiteForm(),
        postRefreshProbeScope: 'all',
      }).success
    ).toBe(true)
    const result = siteFormSchema.safeParse({
      ...validSiteForm(),
      postRefreshProbeScope:
        'random' as SiteFormValues['postRefreshProbeScope'],
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toContain('Invalid option')
  })
})

// ---------------------------------------------------------------------------
// search schema
// ---------------------------------------------------------------------------

describe('sitesSearchSchema', () => {
  it('accepts page 0 and any status string (no enum)', () => {
    expect(
      sitesSearchSchema.parse({ page: '0', status: 'whatever' })
    ).toMatchObject({
      page: 0,
      status: 'whatever',
    })
  })

  it('falls back instead of throwing for out-of-range / non-string values', () => {
    const result = sitesSearchSchema.parse({
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

// ---------------------------------------------------------------------------
// create deep-link param (one-shot transient — not a persisted filter)
// ---------------------------------------------------------------------------

describe('sitesSearchSchema — create deep-link', () => {
  it.each([
    ['?create=1 (number) to true', { create: 1 }, true],
    ['a literal boolean true to true', { create: true }, true],
    ['an absent param to undefined', {}, undefined],
    ['?create=0 (number) to false', { create: 0 }, false],
  ])('coerces %s', (_label, input, expected) => {
    expect(sitesSearchSchema.parse(input).create).toBe(expected)
  })

  it('does not throw on a malformed create value (graceful fallback)', () => {
    expect(
      sitesSearchSchema.safeParse({ create: { weird: true } }).success
    ).toBe(true)
  })
})
