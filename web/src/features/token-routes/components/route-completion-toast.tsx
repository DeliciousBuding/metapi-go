// metapi-go features/token-routes/components — the guided "route ready, issue
// a key" toast shown after a route is created. Step 3 → step 4 handoff of the
// site → account → route → key guided configuration chain: routes alone do not
// make /v1 callable, a downstream key does.
//
// Navigation goes through the shared router instance (lib/router) because
// the toast action outlives the route form component that created it — a
// router.navigate keeps it a SPA transition instead of a hard reload. The
// router is imported lazily inside the click handler: a static import here
// would cycle back through routeTree.gen (router → routes → features →
// this toast).

import i18n from '@/i18n/config'
import { toast } from '@/lib/toast'

/**
 * Fire the post-create guided toast. Call from the create-route form's
 * success handler. The primary CTA takes the operator to the first-class
 * Downstream Keys page, where keys are issued and each row exposes the
 * one-click client connect surface (endpoint + key + Cherry Studio /
 * CC Switch import).
 *
 * Destination note: this used to point at `/settings/downstream`, the pre-
 * promotion home of the same section. The left nav now links
 * `/downstream-keys`, so the guided chain lands where the operator will look
 * for it afterwards.
 */
export function showRouteCompletionToast(
  routeId?: number,
  context?: { accountId?: number; siteId?: number }
): void {
  const chainSuffix =
    context?.accountId || context?.siteId
      ? i18n.t('tokenRoutes.completion.chainComplete')
      : i18n.t('tokenRoutes.completion.flowComplete')
  const description = routeId
    ? i18n.t('tokenRoutes.completion.withRouteId', {
        id: routeId,
        suffix: chainSuffix,
      })
    : i18n.t('tokenRoutes.completion.withoutRouteId', { suffix: chainSuffix })

  toast.success(i18n.t('tokenRoutes.completion.title'), {
    description,
    duration: 8000,
    action: {
      label: i18n.t('tokenRoutes.completion.connectAction'),
      onClick: async () => {
        const { router } = await import('@/lib/router')
        await router.navigate({ to: '/downstream-keys' })
      },
    },
  })
}
