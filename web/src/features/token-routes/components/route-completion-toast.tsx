// metapi-go features/token-routes/components — the guided "configuration
// complete" toast shown after a route is created. This is the **final step**
// of the site → account → route guided configuration chain.
//
// Navigation uses window.location.assign rather than TanStack Router's
// type-safe `navigate` because the dashboard route registration is owned by
// another subagent; a hard navigation keeps the deep-link working today.

import { toast } from '@/lib/toast'

import i18n from '@/i18n/config'

/**
 * Fire the post-create guided toast. Call from the create-route form's
 * success handler. The primary CTA carries the operator back to the
 * Dashboard to verify the configuration is live.
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
      label: i18n.t('tokenRoutes.completion.action'),
      onClick: () => window.location.assign('/'),
    },
  })
}
