// Failure-copy contract for the account Models panel mutations.
//
// `assertBusinessOk` throws `i18n.t(fallbackKey)` when the backend answers
// `success:false` WITHOUT a message, and the panel injects that message into
// `accounts.models.refreshFailedInline` / `manualFailedInline`. Both fallback
// keys shipped missing from the locales, so the failure path rendered the raw
// key ("Refresh failed: accounts.models.refreshFailed") — invisible to a
// `t('literal')` scan because the key never sits inside a `t(` call.
//
// These cases pin the whole chain (guard → thrown message → inline copy) in
// BOTH languages, reading the bundles directly so the en fallback cannot mask
// a missing zh-CN entry.

import { describe, expect, it } from 'vitest'

import i18n from '@/i18n/config'
import { assertBusinessOk } from '@/lib/assert-business-ok'

const CASES = [
  {
    fallbackKey: 'accounts.models.refreshFailed',
    inlineKey: 'accounts.models.refreshFailedInline',
  },
  {
    fallbackKey: 'accounts.models.manualFailed',
    inlineKey: 'accounts.models.manualFailedInline',
  },
] as const

/** Run the guard against a message-less failure envelope; return its throw. */
function thrownMessage(fallbackKey: string): string {
  try {
    assertBusinessOk({ success: false }, fallbackKey)
  } catch (error) {
    return error instanceof Error ? error.message : String(error)
  }
  throw new Error(
    `assertBusinessOk accepted a failure envelope (fallback ${fallbackKey})`
  )
}

describe('account models failure copy', () => {
  it('renders translated inline copy when the backend omits the reason', async () => {
    // Locale bundles are lazy-loaded per language — load both before reading.
    await i18n.changeLanguage('zhCN')

    for (const language of ['en', 'zhCN']) {
      await i18n.changeLanguage(language)
      for (const { fallbackKey, inlineKey } of CASES) {
        expect(
          i18n.getResource(language, 'translation', fallbackKey),
          `${language} missing ${fallbackKey}`
        ).toBeTruthy()

        const message = thrownMessage(fallbackKey)
        // The guard must translate, never echo the key it was handed.
        expect(message, `${language}: ${fallbackKey}`).not.toBe(fallbackKey)
        expect(message, `${language}: untranslated fallback`).not.toContain(
          'accounts.models.'
        )

        const inline = i18n.t(inlineKey, { message })
        expect(inline, `${language}: ${inlineKey}`).toContain(message)
        expect(inline, `${language}: unresolved placeholder`).not.toContain(
          '{{message}}'
        )
        expect(inline, `${language}: raw key in panel copy`).not.toContain(
          'accounts.models.'
        )
      }
    }

    await i18n.changeLanguage('en')
  })
})
