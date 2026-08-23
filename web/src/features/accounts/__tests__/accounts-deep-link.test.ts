import { describe, expect, it } from 'vitest'

import {
  resolveDeepLinkCredentialMode,
  resolveDeepLinkPreselect,
} from '../lib/accounts-deep-link'
import type { Site } from '../types'

function site(id: number): Site {
  return {
    id,
    name: `site-${id}`,
    url: `https://site-${id}.example`,
    platform: 'openai',
    status: 'active',
  }
}

describe('resolveDeepLinkPreselect', () => {
  it('returns the site id when the create flag and a matching site are present', () => {
    expect(resolveDeepLinkPreselect(true, 3, [site(1), site(3)])).toBe(3)
  })

  it('returns null when the create flag is absent', () => {
    expect(resolveDeepLinkPreselect(undefined, 3, [site(3)])).toBeNull()
  })

  it('returns null when the create flag is false', () => {
    expect(resolveDeepLinkPreselect(false, 3, [site(3)])).toBeNull()
  })

  it('returns null for a missing or non-positive siteId', () => {
    expect(resolveDeepLinkPreselect(true, undefined, [site(3)])).toBeNull()
    expect(resolveDeepLinkPreselect(true, 0, [site(3)])).toBeNull()
  })

  it('returns null when the referenced site is not in the snapshot', () => {
    expect(resolveDeepLinkPreselect(true, 99, [site(1), site(3)])).toBeNull()
  })

  it('returns null for an empty snapshot', () => {
    expect(resolveDeepLinkPreselect(true, 3, [])).toBeNull()
  })
})

describe('resolveDeepLinkCredentialMode', () => {
  it('resolves the apikey segment from the add-API-Key CTA', () => {
    expect(resolveDeepLinkCredentialMode('apikey')).toBe('apikey')
  })

  it('returns null for unknown or absent segments (session default rules)', () => {
    expect(resolveDeepLinkCredentialMode(undefined)).toBeNull()
    expect(resolveDeepLinkCredentialMode('')).toBeNull()
    expect(resolveDeepLinkCredentialMode('session')).toBeNull()
    expect(resolveDeepLinkCredentialMode('password')).toBeNull()
    expect(resolveDeepLinkCredentialMode('apikey2')).toBeNull()
  })
})
