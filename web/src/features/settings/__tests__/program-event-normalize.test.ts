// metapi-go/features/settings — unit tests for the program-log event row
// normalizer. The events handler returns raw DB rows (snake_case `created_at`,
// integer `read`); the section must map them to the camelCase ProgramEvent
// shape or timestamps render as — and unread detection becomes integer
// truthiness.

import { describe, expect, it } from 'vitest'

import {
  eventTitleKey,
  formatTimestamp,
  normalizeEvent,
  parseEventMessage,
  parsePanelPath,
  splitEnrichmentNames,
} from '../sections/operations/lib/event-normalize'

describe('normalizeEvent', () => {
  it('maps created_at to createdAt and integer read to boolean', () => {
    const event = normalizeEvent({
      id: 68,
      type: 'status',
      title: '运行时设置已更新',
      message: 'settings changed',
      level: 'info',
      read: 0,
      created_at: '2026-08-12T04:43:45Z',
    })
    expect(event.createdAt).toBe('2026-08-12T04:43:45Z')
    expect(event.read).toBe(false)
    expect(event.id).toBe(68)
    expect(event.level).toBe('info')
  })

  it('treats read=1 and read=true as read', () => {
    expect(normalizeEvent({ read: 1 }).read).toBe(true)
    expect(normalizeEvent({ read: '1' }).read).toBe(true)
    expect(normalizeEvent({ read: true }).read).toBe(true)
    expect(normalizeEvent({ read: 0 }).read).toBe(false)
  })

  it('defaults missing fields and keeps created_at undefined as undefined', () => {
    const event = normalizeEvent({ id: 1, type: 'checkin' })
    expect(event.createdAt).toBeUndefined()
    expect(event.read).toBeUndefined()
    expect(event.message).toBeUndefined()
    expect(event.title).toBe('')
    expect(event.type).toBe('checkin')
  })
})

describe('formatTimestamp', () => {
  it('renders a localized date-time instead of the raw ISO string', () => {
    const formatted = formatTimestamp('2026-08-12T04:43:45Z')
    // The exact punctuation is locale/ICU dependent; the contract is that
    // the raw ISO payload never reaches the user.
    expect(formatted).not.toContain('2026-08-12T04:43:45Z')
    expect(formatted).toContain('2026')
  })

  it('renders an em dash for a missing timestamp', () => {
    expect(formatTimestamp(undefined)).toBe('—')
    expect(formatTimestamp('')).toBe('—')
  })

  it('renders an em dash for an unparseable timestamp', () => {
    expect(formatTimestamp('not-a-date')).toBe('—')
  })
})

describe('eventTitleKey', () => {
  it('maps known backend titles to localized i18n keys', () => {
    expect(eventTitleKey('All proxies failed')).toBe('allProxiesFailed')
    expect(eventTitleKey('Token expired')).toBe('tokenExpired')
    expect(eventTitleKey('Low balance')).toBe('lowBalance')
    expect(eventTitleKey('checkin failed (cloudflare challenge)')).toBe(
      'checkinFailedCloudflare'
    )
  })

  it('returns undefined for unknown titles so they render as-is', () => {
    expect(eventTitleKey('Some new event')).toBeUndefined()
    expect(eventTitleKey('')).toBeUndefined()
  })
})

describe('parseEventMessage', () => {
  const enriched =
    'model=gpt-4o, reason=Upstream request failed\n' +
    'Affected routes: GPT-4o 主路由, GPT-4o 全量通配\n' +
    'Alternative sites: NewAPI 公益站(3), OneAPI 聚合(1)\n' +
    'Panel: /observability?section=health'

  it('splits enriched alert messages into base + structured parts', () => {
    const parts = parseEventMessage(enriched)
    expect(parts.base).toBe('model=gpt-4o, reason=Upstream request failed')
    expect(parts.routes).toBe('GPT-4o 主路由, GPT-4o 全量通配')
    expect(parts.sites).toBe('NewAPI 公益站(3), OneAPI 聚合(1)')
    expect(parts.panelPath).toBe('/observability?section=health')
  })

  it('returns only base for plain messages without enrichment lines', () => {
    const parts = parseEventMessage('alice @ site: request timed out')
    expect(parts.base).toBe('alice @ site: request timed out')
    expect(parts.routes).toBeNull()
    expect(parts.sites).toBeNull()
    expect(parts.panelPath).toBeNull()
  })

  it('handles missing enrichment sections independently', () => {
    const parts = parseEventMessage(
      'base line\nAffected routes: only-route\nPanel: /observability?section=health'
    )
    expect(parts.base).toBe('base line')
    expect(parts.routes).toBe('only-route')
    expect(parts.sites).toBeNull()
    expect(parts.panelPath).toBe('/observability?section=health')
  })

  it('does not treat lookalike base text as enrichment lines', () => {
    const parts = parseEventMessage(
      'site says "Affected routes: none" literally'
    )
    expect(parts.base).toBe('site says "Affected routes: none" literally')
    expect(parts.routes).toBeNull()
  })
})

describe('splitEnrichmentNames', () => {
  it('splits comma-separated names and trims whitespace', () => {
    expect(splitEnrichmentNames('A,  B ,C')).toEqual(['A', 'B', 'C'])
  })

  it('drops empty items', () => {
    expect(splitEnrichmentNames('A,,B,')).toEqual(['A', 'B'])
  })
})

describe('parsePanelPath', () => {
  it('parses an internal path with query params', () => {
    expect(parsePanelPath('/observability?section=health')).toEqual({
      to: '/observability',
      search: { section: 'health' },
    })
  })

  it('accepts a bare path without query', () => {
    expect(parsePanelPath('/settings/operations/program-logs')).toEqual({
      to: '/settings/operations/program-logs',
      search: {},
    })
  })

  it('rejects non-path or whitespace-containing values', () => {
    expect(parsePanelPath('https://evil.example')).toBeNull()
    expect(parsePanelPath('no slash')).toBeNull()
    expect(parsePanelPath('/path with space')).toBeNull()
  })
})
