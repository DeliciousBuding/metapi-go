// metapi-go/src/__tests__ — one table-driven gate for every feature-level
// status → badge-variant wiring assertion that used to live in seven
// copy-pasted `status-badge-variants.test.tsx` files. These are wiring
// assertions (feature X's column cell renders semantic variant Y for status
// Z), not Badge-primitive recipe assertions; the primitive contract stays in
// components/data-table/__tests__/status-badge-copy.test.tsx.
//
// Housed at src/__tests__ (not src/components/data-table/__tests__) because
// the package boundary gate forbids src/components/ from importing feature
// column hooks/components.

import '@testing-library/jest-dom/vitest'
import type { ColumnDef } from '@tanstack/react-table'
import { cleanup, render, renderHook } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'
import { useAccountsColumns } from '@/features/accounts/components/accounts-columns'
import type { Account, AccountRowActions } from '@/features/accounts/types'
import { useChannelsColumns } from '@/features/channels/components/channels-columns'
import type { ChannelRow, ChannelStatus } from '@/features/channels/types'
import { useCheckinColumns } from '@/features/checkin/components/checkin-columns'
import { FailureReasonBadge } from '@/features/checkin/components/failure-reason-badge'
import type { CheckinLogRow, CheckinRowActions } from '@/features/checkin/types'
import {
  useOAuthColumns,
  type OAuthColumnActions,
} from '@/features/oauth/components/oauth-columns'
import type { OAuthClient, OAuthClientStatus } from '@/features/oauth/types'
import {
  useSitesColumns,
  type SitesColumnActions,
} from '@/features/sites/components/sites-columns'
import type { Site, SiteStatus } from '@/features/sites/types'
import { useRoutesColumns } from '@/features/token-routes/components/routes-columns'
import type {
  RouteRowActions,
  RouteSummaryRow,
} from '@/features/token-routes/types'

const noop = () => {}

const noopAccountActions: AccountRowActions = {
  onViewDetail: noop,
  onRefresh: noop,
  onTogglePin: noop,
  onToggleStatus: noop,
  onToggleCheckin: noop,
  onEdit: noop,
  onDelete: noop,
}

const noopCheckinActions: CheckinRowActions = {
  onViewDetail: noop,
  onTriggerAccount: noop,
}

const noopOAuthActions: OAuthColumnActions = {
  onViewDetails: noop,
  onRefreshQuota: noop,
  onRebind: noop,
  onDelete: noop,
}

const noopSiteActions: SitesColumnActions = {
  onEdit: noop,
  onView: noop,
  onToggleStatus: noop,
  onTogglePin: noop,
  onDelete: noop,
}

const noopRouteActions: RouteRowActions = {
  onEdit: noop,
  onDelete: noop,
  onToggleEnabled: noop,
  onViewDetail: noop,
  onClearCooldown: noop,
  onRefreshDecision: noop,
}

function cellFrom<T>(columns: ColumnDef<T>[], columnId: string, row: T) {
  const column = columns.find((entry) => entry.id === columnId)
  if (!column?.cell) throw new Error(`${columnId} column cell missing`)
  const cell = column.cell as unknown as (context: {
    row: { original: T }
  }) => ReactElement
  return cell({ row: { original: row } })
}

const accountCell = (status: string) => {
  const { result } = renderHook(() => useAccountsColumns(noopAccountActions))
  const row = {
    status: status === 'expired' || status === 'disabled' ? status : 'active',
    runtimeHealth:
      status === 'expired' ? undefined : { state: status, reason: '' },
  } as unknown as Account
  return cellFrom(result.current, 'status', row)
}

const channelCell = (status: string) => {
  const { result } = renderHook(() => useChannelsColumns())
  return cellFrom(result.current, 'status', {
    status: status as ChannelStatus,
  } as ChannelRow)
}

const checkinCell = (status: string) => {
  const { result } = renderHook(() => useCheckinColumns(noopCheckinActions))
  return cellFrom(result.current, 'status', {
    checkin_logs: { id: 1, accountId: 1, status },
    failureReason: null,
  } as unknown as CheckinLogRow)
}

const failureReasonCell = (category: string) => (
  <FailureReasonBadge
    reason={{
      code: 'probe_code',
      category,
      title: 'Reason title',
      actionHint: '',
      detailHint: '',
    }}
  />
)

const oauthCell = (status: string) => {
  const { result } = renderHook(() => useOAuthColumns(noopOAuthActions))
  return cellFrom(result.current, 'status', {
    status: status === 'missing' ? undefined : (status as OAuthClientStatus),
  } as OAuthClient)
}

const siteCell = (status: string) => {
  const { result } = renderHook(() => useSitesColumns(noopSiteActions))
  return cellFrom(result.current, 'status', {
    status: status as SiteStatus,
  } as Site)
}

// The enabled/total channel summary ladder, spelled as data: all enabled ->
// success, partially enabled -> warning, none enabled -> secondary.
const routeChannelLadder: Record<
  string,
  { channelCount: number; enabledChannelCount: number }
> = {
  'all enabled': { channelCount: 3, enabledChannelCount: 3 },
  'partially enabled': { channelCount: 3, enabledChannelCount: 1 },
  'all disabled': { channelCount: 2, enabledChannelCount: 0 },
}

const routeCell = (columnId: string, status: string) => {
  const { result } = renderHook(() => useRoutesColumns(noopRouteActions))
  const row =
    columnId === 'enabled'
      ? { enabled: status === 'enabled' }
      : routeChannelLadder[status]
  return cellFrom(result.current, columnId, row as unknown as RouteSummaryRow)
}

const routeEnabledCell = (status: string) => routeCell('enabled', status)
const routeChannelsCell = (status: string) => routeCell('channels', status)

function readBadgeVariant(container: HTMLElement): string | null {
  const badge = container.querySelector('[data-slot="badge"]')
  return badge?.getAttribute('data-variant') ?? null
}

afterEach(() => cleanup())

type BadgeWiringCase = [
  feature: string,
  column: string,
  status: string,
  wantVariant: string,
  makeCell: (status: string) => ReactElement,
]

const badgeWiringCases: BadgeWiringCase[] = [
  ['accounts', 'status', 'healthy', 'success', accountCell],
  ['accounts', 'status', 'degraded', 'warning', accountCell],
  ['accounts', 'status', 'unhealthy', 'destructive', accountCell],
  ['accounts', 'status', 'expired', 'destructive', accountCell],
  ['accounts', 'status', 'disabled', 'secondary', accountCell],
  ['channels', 'status', 'enabled', 'success', channelCell],
  ['channels', 'status', 'cooldown', 'warning', channelCell],
  ['channels', 'status', 'breaker_open', 'destructive', channelCell],
  ['channels', 'status', 'manually_disabled', 'secondary', channelCell],
  ['checkin', 'status', 'success', 'success', checkinCell],
  ['checkin', 'status', 'skipped', 'secondary', checkinCell],
  ['checkin', 'status', 'failed', 'destructive', checkinCell],
  ['checkin', 'reason', 'auth', 'destructive', failureReasonCell],
  ['checkin', 'reason', 'verification', 'warning', failureReasonCell],
  ['checkin', 'reason', 'network', 'info', failureReasonCell],
  ['checkin', 'reason', 'site', 'outline', failureReasonCell],
  ['checkin', 'reason', 'state', 'success', failureReasonCell],
  ['checkin', 'reason', 'unknown', 'outline', failureReasonCell],
  ['checkin', 'reason', 'unrecognized', 'outline', failureReasonCell],
  ['oauth', 'status', 'healthy', 'success', oauthCell],
  ['oauth', 'status', 'abnormal', 'destructive', oauthCell],
  ['oauth', 'status', 'missing', 'success', oauthCell],
  ['sites', 'status', 'active', 'success', siteCell],
  ['sites', 'status', 'disabled', 'secondary', siteCell],
  ['routes', 'enabled', 'enabled', 'success', routeEnabledCell],
  ['routes', 'enabled', 'disabled', 'secondary', routeEnabledCell],
  ['routes', 'channels', 'all enabled', 'success', routeChannelsCell],
  ['routes', 'channels', 'partially enabled', 'warning', routeChannelsCell],
  ['routes', 'channels', 'all disabled', 'secondary', routeChannelsCell],
]

describe('status badge variant wiring', () => {
  it.each(badgeWiringCases)(
    '%s %s maps %s to the %s variant',
    (_feature, _column, status, wantVariant, makeCell) => {
      const { container } = render(makeCell(status))
      expect(readBadgeVariant(container)).toBe(wantVariant)
    }
  )
})
