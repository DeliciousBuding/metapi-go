import { describe, expect, it } from 'vitest'

import type { Site } from '@/features/sites/types'

import { resolveAnnouncementSourceURL } from '../announcement-source-url'
import type { SiteAnnouncement } from '../types'

const SITE: Site = {
  id: 7,
  name: 'Alpha',
  url: 'https://alpha.example/base/',
}

const ITEM: SiteAnnouncement = {
  id: 1,
  siteId: 7,
  platform: 'new-api',
  title: 'Notice',
  content: '',
  level: 'info',
  sourceKey: 'notice-1',
  sourceUrl: null,
  startsAt: null,
  endsAt: null,
  firstSeenAt: '2026-08-25T00:00:00Z',
  lastSeenAt: '2026-08-25T00:00:00Z',
  upstreamCreatedAt: null,
  upstreamUpdatedAt: null,
  readAt: null,
  dismissedAt: null,
  rawPayload: null,
}

describe('resolveAnnouncementSourceURL', () => {
  it('resolves a same-origin relative detail path', () => {
    expect(
      resolveAnnouncementSourceURL(
        { ...ITEM, sourceUrl: '/notice/detail/42' },
        SITE
      )
    ).toBe('https://alpha.example/notice/detail/42')
  })

  it('allows an absolute same-origin HTTP(S) detail URL', () => {
    expect(
      resolveAnnouncementSourceURL(
        { ...ITEM, sourceUrl: 'https://alpha.example/notice/42' },
        SITE
      )
    ).toBe('https://alpha.example/notice/42')
  })

  it.each([
    'https://evil.example/phish',
    '//evil.example/phish',
    'javascript:alert(1)',
    'data:text/html,boom',
    'file:///tmp/secret',
  ])(
    'falls back to the trusted Site home for unsafe source %s',
    (sourceUrl) => {
      expect(resolveAnnouncementSourceURL({ ...ITEM, sourceUrl }, SITE)).toBe(
        'https://alpha.example/base/'
      )
    }
  )

  it('returns the Site home when no detail URL exists', () => {
    expect(resolveAnnouncementSourceURL(ITEM, SITE)).toBe(
      'https://alpha.example/base/'
    )
  })

  it('returns null when the stored Site URL is not HTTP(S)', () => {
    expect(
      resolveAnnouncementSourceURL(ITEM, { ...SITE, url: 'javascript:x' })
    ).toBeNull()
  })
})
