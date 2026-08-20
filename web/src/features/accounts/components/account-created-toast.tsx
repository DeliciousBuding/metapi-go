// metapi-go features/accounts/components — the guided "next step" toast shown
// after an account is created. This is step 2 of the site → account → route
// guided configuration chain (research/10-user-flows.md §4.2): the account
// is added, and the operator is nudged to configure routes for it next.
//
// Navigation goes through the shared router instance (lib/router) because
// the toast action outlives the account form component that created it —
// a router.navigate keeps it a SPA transition instead of a hard reload.
// The router is imported lazily inside the click handler: a static import
// here would cycle back through routeTree.gen (router → routes → features
// → this toast).

import i18n from '@/i18n/config'
import { toast } from '@/lib/toast'

/**
 * Fire the post-create guided toast. Call from the create-account form's
 * success handler. The primary CTA carries the operator to the route
 * configuration page with the new account (and its site) preselected.
 */
export function showAccountCreatedToast(
  accountId?: number,
  siteId?: number
): void {
  toast.success(i18n.t('accounts.created.title'), {
    description: i18n.t('accounts.created.description'),
    duration: 8000,
    action: {
      label: i18n.t('accounts.created.action'),
      onClick: async () => {
        const { router } = await import('@/lib/router')
        await router.navigate({
          to: '/token-routes',
          search: { accountId, siteId },
        })
      },
    },
  })
}
