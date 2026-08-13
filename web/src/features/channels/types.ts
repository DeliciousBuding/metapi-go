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
  enabled: boolean
  manualOverride: boolean
}

/**
 * TanStack Query key factory. The channels list is a single full-list query
 * (no server-side pagination), so one stable key is enough for invalidation.
 */
export const channelsKeys = {
  all: ['channels'] as const,
  list: () => [...channelsKeys.all, 'list'] as const,
}
