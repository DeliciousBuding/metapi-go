// metapi-go/features/channels — domain types for the Channels read-only list.
// The backend `/api/channels` surface returns an aggregated projection across
// account/token/oauth_unit dimensions; this contract stays 1:1 with the
// handler/admin/channels.go payload (camelCase).

export type ChannelStatus =
  | 'enabled'
  | 'cooldown'
  | 'breaker_open'
  | 'manually_disabled'

type ChannelType = 'account' | 'token' | 'oauth_unit'

export type ChannelRow = {
  id: number
  routeId: number
  name: string
  site: { id: number; name: string }
  type: ChannelType
  status: ChannelStatus
  models: string
  priority: number
  weight: number
  responseMs: number | null
  cooldownUntil: string | null
  /**
   * Structured cooldown reason (P0-3). All three fields are null when the
   * channel cooled down before the reason columns existed — the UI reports
   * that honestly as "reason not recorded" instead of guessing.
   */
  cooldownReasonCode: string | null
  cooldownReason: string | null
  cooldownReasonAt: string | null
  enabled: boolean
  manualOverride: boolean
}

/**
 * One server-side channels page returned by GET /api/channels when page/pageSize
 * are present.
 */
export type ChannelsPageData = {
  items: ChannelRow[]
  total: number
}

/** Fleet-wide runtime status counts from GET /api/channels/error-summary. */
export type ChannelsErrorSummary = {
  total: number
  errorCount: number
  byStatus: Record<ChannelStatus, number>
}

/**
 * TanStack Query key factory. The channels page now uses a server-paginated
 * key (page/pageSize/status), while the legacy full-list key remains for the
 * one-shot channel drilldown and any non-page consumers.
 */
export const channelsKeys = {
  all: ['channels'] as const,
  list: () => [...channelsKeys.all, 'list'] as const,
  page: (pageIndex: number, pageSize: number, status?: string) =>
    [...channelsKeys.all, 'page', { pageIndex, pageSize, status }] as const,
  errorSummary: () => [...channelsKeys.all, 'error-summary'] as const,
}
