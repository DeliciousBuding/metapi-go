// Vitest for the primary-site URL analysis (port of the TS original's
// shared/sitePrimaryUrl tests plus the go-side sanity cases).
import { describe, expect, it } from 'vitest'

import { analyzePrimarySiteUrl } from './site-primary-url'

describe('sitePrimaryUrl', () => {
  it('returns unchanged for empty, nullish, and invalid inputs', () => {
    expect(analyzePrimarySiteUrl('')).toMatchObject({
      canonicalUrl: '',
      persistedUrl: '',
      action: 'unchanged',
      matchedPath: '',
    })
    expect(analyzePrimarySiteUrl(null as unknown as string)).toMatchObject({
      canonicalUrl: '',
      persistedUrl: '',
      action: 'unchanged',
      matchedPath: '',
    })
    expect(analyzePrimarySiteUrl(undefined as unknown as string)).toMatchObject(
      {
        canonicalUrl: '',
        persistedUrl: '',
        action: 'unchanged',
        matchedPath: '',
      }
    )
    expect(analyzePrimarySiteUrl(' not a valid url/// ')).toMatchObject({
      canonicalUrl: 'not a valid url',
      persistedUrl: 'not a valid url',
      action: 'unchanged',
      matchedPath: '',
    })
  })

  it('returns unchanged for root-only urls', () => {
    expect(analyzePrimarySiteUrl('https://example.com/')).toMatchObject({
      canonicalUrl: 'https://example.com',
      persistedUrl: 'https://example.com',
      action: 'unchanged',
      matchedPath: '/',
    })
  })

  it('auto-strips known non-api request suffixes to the host root', () => {
    expect(
      analyzePrimarySiteUrl('https://api.openai.com/v1/messages?trace=1#frag')
    ).toMatchObject({
      canonicalUrl: 'https://api.openai.com/v1/messages',
      persistedUrl: 'https://api.openai.com',
      action: 'auto_strip_known_api_suffix',
      matchedPath: '/v1/messages',
    })
  })

  it('auto-strips a bare /v1 suffix', () => {
    expect(
      analyzePrimarySiteUrl('https://openai.example.com/v1')
    ).toMatchObject({
      canonicalUrl: 'https://openai.example.com/v1',
      persistedUrl: 'https://openai.example.com',
      action: 'auto_strip_known_api_suffix',
      matchedPath: '/v1',
    })
  })

  it('preserves api-prefixed paths and marks them as warnings', () => {
    expect(
      analyzePrimarySiteUrl('api.example.com/api/v1/models')
    ).toMatchObject({
      canonicalUrl: 'https://api.example.com/api/v1/models',
      persistedUrl: 'https://api.example.com/api/v1/models',
      action: 'preserve_api_path',
      matchedPath: '/api/v1/models',
    })
  })

  it('preserves known semantic paths without warnings', () => {
    expect(
      analyzePrimarySiteUrl('https://example.com/anthropic')
    ).toMatchObject({
      canonicalUrl: 'https://example.com/anthropic',
      persistedUrl: 'https://example.com/anthropic',
      action: 'preserve_semantic_path',
      matchedPath: '/anthropic',
    })
    expect(
      analyzePrimarySiteUrl('https://example.com/backend-api/codex')
    ).toMatchObject({
      action: 'preserve_semantic_path',
      persistedUrl: 'https://example.com/backend-api/codex',
    })
  })

  it('preserves unknown extra paths and marks them as warnings', () => {
    expect(analyzePrimarySiteUrl('https://example.com/login')).toMatchObject({
      canonicalUrl: 'https://example.com/login',
      persistedUrl: 'https://example.com/login',
      action: 'preserve_unknown_path',
      matchedPath: '/login',
    })
  })

  it('handles missing protocol by assuming https', () => {
    expect(analyzePrimarySiteUrl('example.com')).toMatchObject({
      canonicalUrl: 'https://example.com',
      persistedUrl: 'https://example.com',
      action: 'unchanged',
    })
  })
})
