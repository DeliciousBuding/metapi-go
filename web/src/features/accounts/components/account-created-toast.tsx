// metapi-go features/accounts/components — the guided "next step" toast shown
// after an account is created. This is step 2 of the site → account → route
// guided configuration chain (research/10-user-flows.md §4.2): the account
// is added, and the operator is nudged to configure routes for it next.
//
// The toast also reports the post-create token sync truthfully (#1002):
// synced (with the real persisted count), empty (no upstream tokens found),
// skipped (original copy — API-key connections or legacy responses without a
// report) and failed (partial initialization, downgraded to a warning). The
// sync report never changes the fact that the account was persisted — the
// CTA to configure routes stays in every state where a sync report exists.
//
// Navigation goes through the shared router instance (lib/router) because
// the toast action outlives the account form component that created it —
// a router.navigate keeps it a SPA transition instead of a hard reload.
// The router is imported lazily inside the click handler: a static import
// here would cycle back through routeTree.gen (router → routes → features
// → this toast).

import i18n from '@/i18n/config'
import { toast } from '@/lib/toast'

import type { TokenSyncStatus } from '../api'

export interface AccountTokenSyncInfo {
  tokenCount?: number
  tokenSyncStatus?: TokenSyncStatus
  tokenSyncMessage?: string
}

type AccountToastCopy = {
  successTitle: string
  fallbackDescription?: string
  partialTitle: string
}

function createRoutesAction(accountId?: number, siteId?: number) {
  return {
    label: i18n.t('accounts.created.action'),
    onClick: async () => {
      const { router } = await import('@/lib/router')
      await router.navigate({
        to: '/token-routes',
        search: { accountId, siteId },
      })
    },
  }
}

function showTokenSyncToast(
  accountId: number | undefined,
  siteId: number | undefined,
  sync: AccountTokenSyncInfo,
  copy: AccountToastCopy
): void {
  const action = createRoutesAction(accountId, siteId)

  if (sync.tokenSyncStatus === 'failed') {
    toast.warning(copy.partialTitle, {
      description: i18n.t('accounts.tokenSync.partialDescription', {
        message: sync.tokenSyncMessage ?? '',
      }),
      duration: 8000,
      action,
    })
    return
  }

  if (sync.tokenSyncStatus === 'synced') {
    toast.success(copy.successTitle, {
      description: i18n.t('accounts.tokenSync.syncedDescription', {
        count: sync.tokenCount ?? 0,
      }),
      duration: 8000,
      action,
    })
    return
  }

  if (sync.tokenSyncStatus === 'empty') {
    toast.success(copy.successTitle, {
      description: i18n.t('accounts.tokenSync.emptyDescription'),
      duration: 8000,
      action,
    })
    return
  }

  // `skipped` is intentional (API-key connection / no upstream token source):
  // keep the original guided copy rather than implying a sync was attempted.
  toast.success(copy.successTitle, {
    description: copy.fallbackDescription ?? '',
    duration: 8000,
    action,
  })
}

/**
 * Fire the post-create guided toast. Call from the create-account form's
 * success handler. The primary CTA carries the operator to the route
 * configuration page with the new account (and its site) preselected.
 *
 * `sync` is the backend post-create token sync report; when absent (legacy
 * or batch responses) the toast keeps its original guided copy.
 */
export function showAccountCreatedToast(
  accountId?: number,
  siteId?: number,
  sync?: AccountTokenSyncInfo
): void {
  showTokenSyncToast(accountId, siteId, sync ?? {}, {
    successTitle: i18n.t('accounts.created.title'),
    fallbackDescription: i18n.t('accounts.created.description'),
    partialTitle: i18n.t('accounts.tokenSync.partialTitle'),
  })
}

/**
 * Report a password-mode bind/rebind using the same token-sync truth as the
 * create flow. Legacy/skipped responses retain the original compact toast;
 * a real sync report gets the count/empty/failure detail and route CTA.
 */
export function showAccountLoginToast(
  accountId: number | undefined,
  siteId: number | undefined,
  reusedAccount: boolean | undefined,
  sync?: AccountTokenSyncInfo
): void {
  const successTitle = i18n.t(
    reusedAccount
      ? 'accounts.toast.loginRelogged'
      : 'accounts.toast.loginSucceeded'
  )

  const status = sync?.tokenSyncStatus
  if (
    !sync ||
    (status !== 'failed' && status !== 'synced' && status !== 'empty')
  ) {
    // `skipped`, an omitted status, or an unknown future status should not
    // imply that a sync ran. Keep the established compact login copy.
    toast.success(successTitle)
    return
  }

  showTokenSyncToast(accountId, siteId, sync, {
    successTitle,
    partialTitle: i18n.t('accounts.tokenSync.loginPartialTitle'),
  })
}
