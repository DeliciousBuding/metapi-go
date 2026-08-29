// URL-state contract for the site-announcements page (W19-T1 P2-l residual):
// filters and the page cursor are normalized from the URL search and
// serialized back with default values omitted. Every param is resilient — a
// malformed or JSON-parsed (number/boolean) value degrades to the page
// default instead of throwing.

import { describe, expect, it } from 'vitest'

import {
  buildSiteAnnouncementsHref,
  parseSiteAnnouncementsSearch,
} from '../url-state'
import { DEFAULT_SITE_ANNOUNCEMENTS_FILTERS } from '../types'

describe('parseSiteAnnouncementsSearch', () => {
  it('returns the default filters and page 0 for an empty search', () => {
    expect(parseSiteAnnouncementsSearch({})).toEqual({
      filters: DEFAULT_SITE_ANNOUNCEMENTS_FILTERS,
      page: 0,
    })
  })

  it('parses a valid siteId from a string or a router-parsed number', () => {
    expect(parseSiteAnnouncementsSearch({ siteId: '7' }).filters.siteId).toBe(7)
    expect(parseSiteAnnouncementsSearch({ siteId: 7 }).filters.siteId).toBe(7)
  })

  it('rejects non-positive, fractional, boolean and garbage siteIds', () => {
    for (const siteId of ['0', '-3', '1.5', 'abc', true, false]) {
      expect(
        parseSiteAnnouncementsSearch({ siteId }).filters.siteId,
        `expected ${JSON.stringify(siteId)} to degrade to null`
      ).toBeNull()
    }
  })

  it('keeps the platform string verbatim', () => {
    expect(parseSiteAnnouncementsSearch({ platform: 'newapi' }).filters.platform).toBe(
      'newapi'
    )
  })

  it('accepts only true/false for the read filter', () => {
    expect(parseSiteAnnouncementsSearch({ read: 'true' }).filters.read).toBe(
      'true'
    )
    expect(parseSiteAnnouncementsSearch({ read: 'false' }).filters.read).toBe(
      'false'
    )
    expect(parseSiteAnnouncementsSearch({ read: 'bogus' }).filters.read).toBe(
      'all'
    )
  })

  it('accepts only the known statuses', () => {
    for (const status of ['active', 'expired', 'dismissed'] as const) {
      expect(parseSiteAnnouncementsSearch({ status }).filters.status).toBe(
        status
      )
    }
    expect(parseSiteAnnouncementsSearch({ status: 'bogus' }).filters.status).toBe(
      'all'
    )
  })

  it('parses the page cursor and clamps invalid values to 0', () => {
    expect(parseSiteAnnouncementsSearch({ page: '2' }).page).toBe(2)
    expect(parseSiteAnnouncementsSearch({ page: 3 }).page).toBe(3)
    for (const page of ['0', '-1', '1.5', 'abc', true]) {
      expect(
        parseSiteAnnouncementsSearch({ page }).page,
        `expected ${JSON.stringify(page)} to degrade to 0`
      ).toBe(0)
    }
  })
})

describe('buildSiteAnnouncementsHref', () => {
  it('returns the bare path when everything is default', () => {
    expect(
      buildSiteAnnouncementsHref(DEFAULT_SITE_ANNOUNCEMENTS_FILTERS, 0)
    ).toBe('/site-announcements')
  })

  it('serializes every non-default filter and the page cursor', () => {
    const href = buildSiteAnnouncementsHref(
      { siteId: 7, platform: 'newapi', read: 'false', status: 'active' },
      2
    )
    const query = new URLSearchParams(href.split('?')[1])
    expect(query.get('siteId')).toBe('7')
    expect(query.get('platform')).toBe('newapi')
    expect(query.get('read')).toBe('false')
    expect(query.get('status')).toBe('active')
    expect(query.get('page')).toBe('2')
  })

  it('URL-encodes platform values with reserved characters', () => {
    const href = buildSiteAnnouncementsHref(
      { ...DEFAULT_SITE_ANNOUNCEMENTS_FILTERS, platform: 'a b&c' },
      0
    )
    expect(new URLSearchParams(href.split('?')[1]).get('platform')).toBe(
      'a b&c'
    )
  })

  it('round-trips through the parser', () => {
    const filters = {
      siteId: 9,
      platform: 'sub2api',
      read: 'true' as const,
      status: 'expired' as const,
    }
    const parsed = parseSiteAnnouncementsSearch(
      Object.fromEntries(
        new URLSearchParams(buildSiteAnnouncementsHref(filters, 4).split('?')[1])
      )
    )
    expect(parsed).toEqual({ filters, page: 4 })
  })
})
