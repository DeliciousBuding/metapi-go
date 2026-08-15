import { describe, expect, it } from 'vitest'

import {
  COLUMN_FILTER_ITEM_SCHEMA,
  PAGINATION_SCHEMA,
  SITE_FORM_DEFAULT_VALUES,
  SORTING_ITEM_SCHEMA,
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
  it('rejects an empty name with nameRequired', () => {
    const result = siteFormSchema.safeParse({ ...validSiteForm(), name: '' })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'sites.form.errors.nameRequired'
    )
  })

  it('rejects a whitespace-only name after trimming', () => {
    expect(
      siteFormSchema.safeParse({ ...validSiteForm(), name: '   ' }).success
    ).toBe(false)
  })

  it('rejects a name over 120 chars', () => {
    const result = siteFormSchema.safeParse({
      ...validSiteForm(),
      name: 'x'.repeat(121),
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'sites.form.errors.nameTooLong'
    )
  })
})

describe('siteFormSchema — url', () => {
  it('rejects an empty url with urlRequired', () => {
    const result = siteFormSchema.safeParse({ ...validSiteForm(), url: '' })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'sites.form.errors.urlRequired'
    )
  })

  it.each([
    ['ftp scheme', 'ftp://example.com'],
    ['plain string', 'not a url'],
    ['javascript scheme', 'javascript:alert(1)'],
  ])('rejects %s with invalidUrl', (_label, url) => {
    const result = siteFormSchema.safeParse({ ...validSiteForm(), url })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe('sites.form.errors.invalidUrl')
  })

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
  it('accepts an empty string', () => {
    expect(
      siteFormSchema.safeParse({ ...validSiteForm(), customHeaders: '' })
        .success
    ).toBe(true)
  })

  it('accepts a plain JSON object', () => {
    expect(
      siteFormSchema.safeParse({
        ...validSiteForm(),
        customHeaders: '{"a":1}',
      }).success
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
  it('rejects a negative globalWeight', () => {
    const result = siteFormSchema.safeParse({
      ...validSiteForm(),
      globalWeight: -1,
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'sites.form.errors.globalWeightMin'
    )
  })

  it('rejects a non-integer maxConcurrency', () => {
    const result = siteFormSchema.safeParse({
      ...validSiteForm(),
      maxConcurrency: 1.5,
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'sites.form.errors.maxConcurrencyInteger'
    )
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

  it('rejects a non-integer latency threshold', () => {
    const result = siteFormSchema.safeParse({
      ...validSiteForm(),
      postRefreshProbeLatencyThresholdMs: 1.5,
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'sites.form.errors.latencyInteger'
    )
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

describe('PAGINATION_SCHEMA + SORTING_ITEM_SCHEMA + COLUMN_FILTER_ITEM_SCHEMA', () => {
  it('PAGINATION_SCHEMA applies defaults', () => {
    expect(PAGINATION_SCHEMA.parse({})).toEqual({ pageIndex: 0, pageSize: 20 })
  })

  it('SORTING_ITEM_SCHEMA requires both fields', () => {
    expect(SORTING_ITEM_SCHEMA.safeParse({ id: 'a' }).success).toBe(false)
    expect(SORTING_ITEM_SCHEMA.safeParse({ id: 'a', desc: true }).success).toBe(
      true
    )
  })

  it('COLUMN_FILTER_ITEM_SCHEMA accepts string / string[] / boolean values', () => {
    expect(
      COLUMN_FILTER_ITEM_SCHEMA.safeParse({ id: 'a', value: 'x' }).success
    ).toBe(true)
    expect(
      COLUMN_FILTER_ITEM_SCHEMA.safeParse({ id: 'a', value: ['x', 'y'] })
        .success
    ).toBe(true)
    expect(
      COLUMN_FILTER_ITEM_SCHEMA.safeParse({ id: 'a', value: true }).success
    ).toBe(true)
    expect(
      COLUMN_FILTER_ITEM_SCHEMA.safeParse({ id: 'a', value: 42 }).success
    ).toBe(false)
  })
})
