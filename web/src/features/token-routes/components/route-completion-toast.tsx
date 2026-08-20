// metapi-go features/token-routes/components — the guided "configuration
// complete" toast shown after a route is created. This is the **final step**
// of the site → account → route guided configuration chain.
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
 * success handler. The primary CTA takes the operator to Settings →
 * Downstream, where each API key now exposes the one-click client connect
 * surface (endpoint + key + Cherry Studio / CC Switch import).
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
        await router.navigate({
          to: '/settings/$subarea',
          params: { subarea: 'downstream' },
        })
      },
    },
  })
}
