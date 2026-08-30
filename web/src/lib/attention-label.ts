// metapi-go/lib — shared attention-item label localization (F3).
//
// The backend attention API (`GET /api/stats/attention`) keeps an English
// `label` for API compat and ships structured `params` alongside (username /
// site name / balance / announcement title) so the SPA can re-localize. Both
// consumers — the header bell popover and the dashboard availability panel —
// must render through `attentionLabel`; never render `item.label` verbatim.
// When the params a category needs are missing (old backend, malformed
// payload) the raw label is used instead of a string with empty
// placeholders.

/** One attention item from GET /api/stats/attention. */
export type AttentionItem = {
  severity: 'critical' | 'warning' | 'info'
  category: string
  label: string
  target: string
  createdAt: string
  /**
   * Structured label params from the backend (username / site name /
   * numeric balance) so the label can be rendered through i18n. Absent on
   * items from old backends → raw `label` is rendered instead.
   */
  params?: Record<string, string | number>
}

import { eventTitleSlug } from './event-titles'

/** Attention response envelope. */
export type AttentionResponse = {
  items: AttentionItem[]
  total: number
}

type TranslateFn = (key: string, options?: Record<string, unknown>) => string

/**
 * Localized label for an attention item, keyed off `category` + `params`.
 * The raw English `label` is the explicit fallback for unknown categories
 * and missing params.
 */
export function attentionLabel(item: AttentionItem, t: TranslateFn): string {
  switch (item.category) {
    case 'expired_account': {
      const name = item.params?.username
      return typeof name === 'string' && name !== ''
        ? t('dashboard.availability.monitors.expiredAccount', { name })
        : item.label
    }
    case 'low_balance': {
      const name = item.params?.username
      const balance = item.params?.balance
      return typeof name === 'string' && name !== '' && balance != null
        ? t('dashboard.availability.monitors.lowBalance', {
            name,
            amount: Number(balance).toFixed(2),
          })
        : item.label
    }
    case 'balance_unknown': {
      const name = item.params?.username
      return typeof name === 'string' && name !== ''
        ? t('dashboard.availability.monitors.balanceUnknown', { name })
        : item.label
    }
    case 'disabled_site': {
      const name = item.params?.name
      return typeof name === 'string' && name !== ''
        ? t('dashboard.availability.monitors.disabledSite', { name })
        : item.label
    }
    case 'site_announcement': {
      const title = item.params?.title
      return typeof title === 'string' && title !== ''
        ? t('dashboard.availability.monitors.siteAnnouncement', { title })
        : item.label
    }
    case 'event': {
      // Persisted event titles map through the shared slug → `events.titles.*`
      // namespace (same source as the program-logs page). Unknown/dynamic
      // titles fall through to the raw label.
      const slug = eventTitleSlug(item.label)
      return slug ? t(`events.titles.${slug}`) : item.label
    }
    default:
      return item.label
  }
}
