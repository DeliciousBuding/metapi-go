// metapi-go/features/settings — unit tests for the program-log event row
// normalizer. The events handler returns raw DB rows (snake_case `created_at`,
// integer `read`); the section must map them to the camelCase ProgramEvent
// shape or timestamps render as — and unread detection becomes integer
// truthiness.

import { describe, expect, it } from 'vitest'

import {
  formatTimestamp,
  normalizeEvent,
} from '../sections/system-info/lib/event-normalize'

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
