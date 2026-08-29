// metapi-go/lib — attention-label tests (F3).
//
// The backend keeps an English `label` for API compat and ships structured
// `params` so the SPA can re-localize. These tests pin the re-localization
// contract: known categories render through i18n in the active language,
// missing params and unknown categories/event titles fall back to the raw
// label (honest residual — never a half-translation).

import { beforeEach, describe, expect, it } from 'vitest'

import i18n from '@/i18n/config'

import { attentionLabel, type AttentionItem } from '../attention-label'

function item(overrides: Partial<AttentionItem>): AttentionItem {
  return {
    severity: 'warning',
    category: 'event',
    label: 'raw label',
    target: '/x',
    createdAt: '2026-08-30T00:00:00Z',
    ...overrides,
  }
}

beforeEach(async () => {
  await i18n.changeLanguage('zhCN')
})

describe('attentionLabel', () => {
  it('re-localizes a known category through params in the active language', async () => {
    await i18n.changeLanguage('zhCN')
    expect(
      attentionLabel(
        item({
          category: 'balance_unknown',
          label: 'Balance unknown: svc-onea',
          params: { username: 'svc-onea' },
        }),
        i18n.t.bind(i18n)
      )
    ).toBe('余额未知：svc-onea')

    await i18n.changeLanguage('en')
    expect(
      attentionLabel(
        item({
          category: 'balance_unknown',
          label: 'Balance unknown: svc-onea',
          params: { username: 'svc-onea' },
        }),
        i18n.t.bind(i18n)
      )
    ).toBe('Balance unknown: svc-onea')
  })

  it('formats low_balance amounts to two decimals', async () => {
    await i18n.changeLanguage('en')
    expect(
      attentionLabel(
        item({
          category: 'low_balance',
          label: 'Low balance: relay (0.5)',
          params: { username: 'relay', balance: 0.5 },
        }),
        i18n.t.bind(i18n)
      )
    ).toBe('Low balance: relay (0.50)')
  })

  it('maps known persisted event titles to their i18n keys', async () => {
    await i18n.changeLanguage('zhCN')
    expect(
      attentionLabel(
        item({ category: 'event', label: 'All proxies failed' }),
        i18n.t.bind(i18n)
      )
    ).toBe('全部代理失败')
    expect(
      attentionLabel(
        item({ category: 'event', label: 'checkin failed' }),
        i18n.t.bind(i18n)
      )
    ).toBe('签到失败')
  })

  it('falls back to the raw label for unknown event titles', async () => {
    await i18n.changeLanguage('zhCN')
    expect(
      attentionLabel(
        item({ category: 'event', label: 'Upstream rate limited' }),
        i18n.t.bind(i18n)
      )
    ).toBe('Upstream rate limited')
  })

  it('falls back to the raw label when params are missing or empty', async () => {
    await i18n.changeLanguage('zhCN')
    expect(
      attentionLabel(
        item({ category: 'expired_account', label: 'Account expired: ops' }),
        i18n.t.bind(i18n)
      )
    ).toBe('Account expired: ops')
    expect(
      attentionLabel(
        item({
          category: 'disabled_site',
          label: 'Site disabled: edge',
          params: { name: '' },
        }),
        i18n.t.bind(i18n)
      )
    ).toBe('Site disabled: edge')
  })

  it('falls back to the raw label for unknown categories', async () => {
    await i18n.changeLanguage('zhCN')
    expect(
      attentionLabel(
        item({ category: 'something_new', label: 'A future category' }),
        i18n.t.bind(i18n)
      )
    ).toBe('A future category')
  })
})
