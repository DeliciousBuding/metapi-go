// metapi-go features/accounts/components — the guided "next step" toast shown
// after an account is created. This is step 2 of the site → account → route
// guided configuration chain (research/10-user-flows.md §4.2): the account
// is added, and the operator is nudged to configure routes for it next.
//
// Navigation uses a hard assignment because the toast action outlives the
// account form component that created it.

import i18n from '@/i18n/config'
import { toast } from '@/lib/toast'

function buildRouteTarget(accountId?: number, siteId?: number): string {
  const params = new URLSearchParams()
  if (accountId) params.set('accountId', String(accountId))
  if (siteId) params.set('siteId', String(siteId))
  const query = params.toString()
  return query ? `/token-routes?${query}` : '/token-routes'
}

/**
 * Fire the post-create guided toast. Call from the create-account form's
 * success handler. The primary CTA carries the operator to the route
 * configuration page with the new account (and its site) preselected.
 */
export function showAccountCreatedToast(
  accountId?: number,
  siteId?: number
): void {
  const target = buildRouteTarget(accountId, siteId)
  toast.success(i18n.t('accounts.created.title'), {
    description: i18n.t('accounts.created.description'),
    duration: 8000,
    action: {
      label: i18n.t('accounts.created.action'),
      onClick: () => window.location.assign(target),
    },
  })
}
