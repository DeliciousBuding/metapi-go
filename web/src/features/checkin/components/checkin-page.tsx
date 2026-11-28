// metapi-go features/checkin/components — the checkin log list page.
// i18n: all user-visible strings migrated to t() calls.
//
// Server-side pagination + filtering (mirrors /api/stats/proxy-logs): the
// table runs in manualPagination/manualFiltering/manualSorting mode and the
// backend returns the real total, so date-range / status / reason / site /
// search filters see the full log history instead of the legacy 500-row
// client cap that silently dropped older records.
//
// URL state uses the shared useUrlTableState hook (same as sites/oauth/
// models/accounts): the URL is the single source of truth and every control
// navigates in one transaction (filter + page reset together), so there is no
// local `useState` mirror + `useEffect` write-back that can feed back into an
// infinite render loop.

import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useAccounts } from '@/features/accounts'

import { useCheckinAccount } from '../api'
import { notifyCheckinResult } from '../lib/notify-checkin-result'
import type { CheckinLogRow } from '../types'
import { CheckinDetailSheet } from './checkin-detail-sheet'
import { CheckinFilterBar } from './checkin-filter-bar'
import { CheckinLogTable } from './checkin-log-table'
import { CheckinPageHeader } from './checkin-page-header'
import { ManualCheckinDialog } from './manual-checkin-dialog'
import { useCheckinLogTable } from './use-checkin-log-table'

export function CheckinPage() {
  const { t } = useTranslation()
  // `isLoading` distinguishes "snapshot still fetching" from "genuinely no
  // accounts": during the fetch window `accountOptions` is `[]`, and pinning
  // the manual check-in button / empty CTA to that raw array kept them
  // flashing disabled-then-enabled (same fix as the accounts page's Add
  // account button — see the comment there).
  const { data: accountsSnapshot, isLoading: accountsLoading } = useAccounts()
  const accountOptions = accountsSnapshot?.accounts ?? []
  const triggerOneMutation = useCheckinAccount()
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailRow, setDetailRow] = useState<CheckinLogRow | null>(null)
  const [manualOpen, setManualOpen] = useState(false)
  const handleTriggerOne = useCallback(
    async (row: CheckinLogRow) => {
      const targetAccountId = row.checkin_logs.accountId
      try {
        const result = await triggerOneMutation.mutateAsync(targetAccountId)
        notifyCheckinResult(result, t)
      } catch {
        // http-client toasted
      }
    },
    [triggerOneMutation, t]
  )

  // Memoized so the column defs keep a stable identity across renders.
  const rowActions = useMemo(
    () => ({
      onViewDetail: (row: CheckinLogRow) => {
        setDetailRow(row)
        setDetailOpen(true)
      },
      onTriggerAccount: handleTriggerOne,
    }),
    [handleTriggerOne]
  )

  // Per-row pending state (accounts-columns' pendingStatusId pattern): only
  // the row whose single-account check-in is in flight shows a spinner, so
  // the trigger stays a seconds-long external request without a global lock.
  const pendingCheckinAccountId = triggerOneMutation.isPending
    ? (triggerOneMutation.variables ?? null)
    : null

  const logs = useCheckinLogTable(rowActions, pendingCheckinAccountId)
  const accountsEmpty = !accountsLoading && accountOptions.length === 0
  const openManualCheckin = () => setManualOpen(true)
  return (
    <div className='page-padding flex h-full flex-col gap-3'>
      <CheckinPageHeader
        query={logs.query}
        accountsEmpty={accountsEmpty}
        onManualCheckin={openManualCheckin}
      />
      {!logs.query.error && (
        <CheckinFilterBar accountOptions={accountOptions} {...logs.filters} />
      )}
      <CheckinLogTable
        table={logs.table}
        query={logs.query}
        handleResetFilters={logs.filters.handleResetFilters}
        accountsEmpty={accountsEmpty}
        onManualCheckin={openManualCheckin}
      />
      <CheckinDetailSheet
        row={detailRow}
        open={detailOpen}
        onOpenChange={setDetailOpen}
      />
      <ManualCheckinDialog open={manualOpen} onOpenChange={setManualOpen} />
    </div>
  )
}
