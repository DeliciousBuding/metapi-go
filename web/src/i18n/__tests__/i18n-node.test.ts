// @vitest-environment node

import { describe, expect, it } from 'vitest'

import i18n, { initI18n } from '../config'

describe('headless translation initialization', () => {
  it('loads and switches shipped locales without a DOM', async () => {
    expect(typeof document).toBe('undefined')
    await initI18n()
    await i18n.changeLanguage('en')
    expect(i18n.t('common.retry')).toBe('Retry')
    await i18n.changeLanguage('zhCN')
    expect(i18n.t('common.retry')).toBe('重试')
  })
})
