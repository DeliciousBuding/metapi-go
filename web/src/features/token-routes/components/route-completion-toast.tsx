// metapi-go features/token-routes/components — the guided "configuration
// complete" toast shown after a route is created. This is the **final step**
// of the site → account → route guided configuration chain
// (research/10-user-flows.md §4.2): the operator has added a route, and the
// chain is now complete. The toast nudges them back to the Dashboard so they
// can verify the fleet and watch traffic flow.
//
// The legacy UI only showed a plain `群组已创建` toast with no next step —
// this completion toast is a net-new addition for the rewrite's guided flow.
//
// Navigation uses window.location.assign rather than TanStack Router's
// type-safe `navigate` because the dashboard route registration is owned by
// another subagent; a hard navigation keeps the deep-link working today and
// can be upgraded to `useNavigate({ to: '/' })` once the route tree lands.

import { toast } from 'sonner'

/**
 * Fire the post-create guided toast. Call from the create-route form's
 * success handler. The primary CTA carries the operator back to the
 * Dashboard to verify the configuration is live.
 *
 * `accountId` / `siteId` are the chain context (carried from the accounts
 * page's `?accountId=&siteId=` deep-link) — included so the operator can
 * confirm which account/site the new route serves if they re-open the toast.
 */
export function showRouteCompletionToast(
  routeId?: number,
  context?: { accountId?: number; siteId?: number },
): void {
  const chainSuffix =
    context?.accountId || context?.siteId
      ? ' 站点 → 账号 → 路由 配置动线已完成。'
      : ' 配置动线已完成。'
  const description = routeId
    ? `路由 #${routeId} 已创建，${chainSuffix.trim()}返回 Dashboard 查看运行状态。`
    : `路由已创建，${chainSuffix.trim()}返回 Dashboard 查看运行状态。`

  toast.success('配置动线完成', {
    description,
    duration: 8000,
    action: {
      label: '返回 Dashboard',
      onClick: () => window.location.assign('/'),
    },
  })
}
