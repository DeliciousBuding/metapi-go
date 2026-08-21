// metapi-go/lib/helpers — document title resolution tests.
// Covers the three `staticData.title` shapes consumed by the root
// useDocumentTitle hook (static key, key list, param resolver).

import { describe, expect, it } from 'vitest'

import { resolveDocumentTitleKeys } from '../document-title'

describe('resolveDocumentTitleKeys', () => {
  it('wraps a static key in a single-item list', () => {
    expect(resolveDocumentTitleKeys('about.title', {})).toEqual(['about.title'])
  })

  it('passes a key list through unchanged', () => {
    const keys = ['settings.subareas.general', 'settings.general.site.title']
    expect(resolveDocumentTitleKeys(keys, {})).toEqual(keys)
  })

  it('invokes a param resolver with the route params', () => {
    const resolver = ({ section }: Record<string, string>) =>
      `dashboard.sections.${section}.title`
    expect(resolveDocumentTitleKeys(resolver, { section: 'traffic' })).toEqual([
      'dashboard.sections.traffic.title',
    ])
  })

  it('supports resolvers returning a key list (subarea + section)', () => {
    const resolver = () => ['settings.subareas.general', 'section.title']
    expect(resolveDocumentTitleKeys(resolver, { subarea: 'general' })).toEqual([
      'settings.subareas.general',
      'section.title',
    ])
  })

  it('falls back to no title when the resolver returns undefined', () => {
    const resolver = () => undefined
    expect(resolveDocumentTitleKeys(resolver, {})).toEqual([])
  })

  it('falls back to no title for an absent spec', () => {
    expect(resolveDocumentTitleKeys(undefined, {})).toEqual([])
  })
})
