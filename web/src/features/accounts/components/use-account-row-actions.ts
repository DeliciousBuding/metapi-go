import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { notifyCheckinResult, useCheckinAccount } from '@/features/checkin'
import { toast } from '@/lib/toast'

import {
  useRefreshAccount,
  useToggleAccountCheckin,
  useToggleAccountPin,
  useToggleAccountStatus,
} from '../api'
import { resolveAccountDisplayName } from '../lib/accounts-display-name'
import type { AccountRowActions } from '../types'

export function useAccountRowActions({
  openEdit,
  openDelete,
  openDetail,
}: {
  openEdit: AccountRowActions['onEdit']
  openDelete: AccountRowActions['onDelete']
  openDetail: AccountRowActions['onViewDetail']
}) {
  const { t } = useTranslation()
  const { mutate: refreshAccount } = useRefreshAccount()
  // Same shape as statusMutation: keep the full mutation object so the pin
  // dropdown item can derive `pendingPinId` for a per-row spinner (mirrors
  // the Power button's pendingStatusId flow; independent per toggle so
  // pin/check-in/status never cross-talk).
  const pinMutation = useToggleAccountPin()
  const toggleAccountPin = pinMutation.mutate
  // Keep the full mutation object so we can derive `pendingStatusId` from
  // `isPending` + `variables` — the inline enable/disable button in each row
  // uses it to show a per-row spinner without a separate state manager.
  const statusMutation = useToggleAccountStatus()
  // Destructure `mutate` to a stable top-level identifier. The
  // `react-hooks/exhaustive-deps` linter flags property accesses like
  // `statusMutation.mutate` inside the memo deps (it wants the whole
  // `statusMutation` object, which is NOT stable across renders and would
  // re-trigger the render loop). The mutate fn identity itself is stable,
  // so a top-level const keeps both the linter and the memo happy.
  const toggleStatusMutate = statusMutation.mutate
  // Same shape as statusMutation: keep the full mutation object so the inline
  // check-in badge button can derive `pendingCheckinId` for a per-row spinner
  // (mirrors the Power button's pendingStatusId flow).
  const checkinMutation = useToggleAccountCheckin()
  const toggleCheckinMutate = checkinMutation.mutate
  const manualCheckinMutation = useCheckinAccount()
  const triggerCheckin = manualCheckinMutation.mutateAsync

  // --- row actions (handed to the columns hook) ---
  // Memoized so the column defs keep a stable identity across renders; a
  // fresh object every render re-resolves the TanStack table instance and
  // re-runs its autoResetPageIndex effect (the old render-loop feedback).
  const rowActions = useMemo<AccountRowActions>(
    () => ({
      onEdit: openEdit,
      onDelete: openDelete,
      onRefresh: (account) => refreshAccount(account.id),
      onViewDetail: openDetail,
      onTogglePin: (account) =>
        toggleAccountPin(
          { id: account.id, isPinned: !account.isPinned },
          {
            onSuccess: () =>
              toast.success(
                t('accounts.toast.pinToggled', {
                  name: resolveAccountDisplayName(
                    account,
                    t('accounts.columns.fallbackApiKey'),
                    t('accounts.columns.fallbackUnnamed')
                  ),
                })
              ),
          }
        ),
      onToggleStatus: (account) =>
        toggleStatusMutate(
          {
            id: account.id,
            status: account.status === 'active' ? 'disabled' : 'active',
          },
          {
            onSuccess: () =>
              toast.success(
                t('accounts.toast.statusToggled', {
                  name: resolveAccountDisplayName(
                    account,
                    t('accounts.columns.fallbackApiKey'),
                    t('accounts.columns.fallbackUnnamed')
                  ),
                })
              ),
          }
        ),
      onToggleCheckin: (account) =>
        toggleCheckinMutate(
          {
            id: account.id,
            checkinEnabled: !account.checkinEnabled,
          },
          {
            onSuccess: () =>
              toast.success(
                t('accounts.toast.checkinToggled', {
                  name: resolveAccountDisplayName(
                    account,
                    t('accounts.columns.fallbackApiKey'),
                    t('accounts.columns.fallbackUnnamed')
                  ),
                })
              ),
          }
        ),
      onTriggerCheckin: (account) => {
        const toastId = toast.loading(
          t('accounts.columns.checkingIn', {
            name: resolveAccountDisplayName(
              account,
              t('accounts.columns.fallbackApiKey'),
              t('accounts.columns.fallbackUnnamed')
            ),
          })
        )
        void triggerCheckin(account.id)
          .then((result) => {
            toast.dismiss(toastId)
            notifyCheckinResult(result, t)
          })
          .catch(() => {
            toast.dismiss(toastId)
            // The HTTP client already displays transport/business failures.
          })
      },
    }),
    [
      openEdit,
      openDelete,
      openDetail,
      refreshAccount,
      toggleAccountPin,
      toggleStatusMutate,
      toggleCheckinMutate,
      triggerCheckin,
      t,
    ]
  )

  // Derive the per-row pending ids from each TanStack Query mutation's
  // `variables` (the last mutate input) — one id per toggle so pin,
  // check-in, and status spinners never cross-talk. Passing these as
  // SEPARATE args to useAccountsColumns is safe: useDataTable stabilizes
  // onChange callbacks via a ref, NOT the columns array ref — so the actions
  // handlers stay memoized and only the cell render closures capture the new
  // pending state. The table instance does NOT re-instantiate (no
  // autoResetPageIndex loop).
  const pendingStatusId = statusMutation.isPending
    ? (statusMutation.variables?.id ?? null)
    : null
  const pendingCheckinId = checkinMutation.isPending
    ? (checkinMutation.variables?.id ?? null)
    : null
  const pendingManualCheckinId = manualCheckinMutation.isPending
    ? (manualCheckinMutation.variables ?? null)
    : null
  const pendingPinId = pinMutation.isPending
    ? (pinMutation.variables?.id ?? null)
    : null

  return {
    rowActions,
    pendingStatusId,
    pendingCheckinId,
    pendingPinId,
    pendingManualCheckinId,
  }
}
