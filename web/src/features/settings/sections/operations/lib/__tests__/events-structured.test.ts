// F5 structured events — registry/locale consistency.
//
// The Go events registry (service/events) and the frontend locale both
// enumerate event keys. The register() helper panics on duplicate keys;
// the backend test asserts the key set. This test pins the FRONTEND side:
// every key the backend registry exposes must have BOTH a `events.titles.*`
// and a `events.messages.*` entry in en + zh-CN, so a registry key without
// a locale entry (or vice versa) fails here — and the backend side fails in
// service/events/events_test.go from the same source-of-truth list.

import { describe, expect, it } from 'vitest'

import i18n from '@/i18n/config'

/** Mirror of service/events registry keys (checkin family, batch 1 of F5). */
const REGISTRY_KEYS = [
  'checkinSuccess',
  'checkinFailed',
  'checkinFailedCloudflare',
  'checkinSkipped',
]

describe('F5 structured events — registry/locale consistency', () => {
  it('declares every registry key under events.titles in both locales', () => {
    for (const key of REGISTRY_KEYS) {
      expect(i18n.exists(`events.titles.${key}`), `titles.${key}`).toBe(true)
    }
  })

  it('declares every registry key under events.messages in both locales', () => {
    for (const key of REGISTRY_KEYS) {
      expect(i18n.exists(`events.messages.${key}`), `messages.${key}`).toBe(
        true
      )
    }
  })

  it('every locale translations of the same message template use the SAME {{vars}}', () => {
    const zh = i18n.getFixedT('zhCN')
    const en = i18n.getFixedT('en')
    for (const key of REGISTRY_KEYS) {
      const zhMsg = zh(`events.messages.${key}`)
      const enMsg = en(`events.messages.${key}`)
      const zhVars = [...zhMsg.matchAll(/\{\{(\w+)\}\}/g)]
        .map((m) => m[1])
        .sort()
      const enVars = [...enMsg.matchAll(/\{\{(\w+)\}\}/g)]
        .map((m) => m[1])
        .sort()
      expect(zhVars, `zh vars for ${key}`).toEqual(enVars)
    }
  })
})
