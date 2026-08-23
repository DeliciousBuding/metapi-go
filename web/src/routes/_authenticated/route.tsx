// metapi-go/routes — authenticated layout route (auth guard).
// beforeLoad checks the localStorage session (storage is the source of truth
// for cold-load guards, avoiding a Zustand hydration race). On failure,
// redirects to /sign-in with the current href as the redirect target.
//
// errorComponent mounts LayoutErrorBoundary so a render-time crash in any
// authenticated page replaces only the page content — the sidebar/nav shell
// (AppHeader + AppSidebar + SidebarProvider) stays mounted and interactive.
// Without this, a page crash would bubble to the router's
// defaultErrorComponent (ErrorPage) and the whole shell would be lost.
//
// notFoundComponent mirrors that for `notFound()` throws from a loader /
// beforeLoad (a missing resource, not a missing path — the latter is handled
// by the `$` catch-all): the 404 renders inside the shell instead of falling
// back to the router-level defaultNotFoundComponent.

import { createFileRoute, redirect } from '@tanstack/react-router'

import {
  AuthenticatedLayout,
  LayoutErrorBoundary,
  NotFoundPage,
} from '@/components/layout'
import {
  hasValidAuthSession,
  isAuthSessionExpired,
  wasAuthSessionExpiredOnLastBoot,
} from '@/lib/auth-session'

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: ({ location }) => {
    // localStorage can throw (SecurityError) in sandboxed/blocked-storage
    // contexts; treat that as unauthenticated rather than crashing the guard.
    let authenticated = false
    let sessionExpired = false
    try {
      // Two expiry records must agree on "expired" for the notice to render:
      // (1) the root bootstrap already ran and cleared the stale entry
      // (cold load: /accounts deep link after the 12h TTL passed), so its
      // pre-clear record is the only survivor — `wasAuthSessionExpiredOnLastBoot`.
      // (2) in-app navigation after the TTL passed without a reload: storage
      // still holds the expired entry, so the raw probe below catches it.
      // Either way the probe runs BEFORE hasValidAuthSession, whose
      // getAuthToken wipes the expired entry via clearAuthSession.
      sessionExpired =
        wasAuthSessionExpiredOnLastBoot() || isAuthSessionExpired(localStorage)
      authenticated = hasValidAuthSession(localStorage)
    } catch {
      authenticated = false
    }
    if (!authenticated) {
      throw redirect({
        to: '/sign-in',
        search: sessionExpired
          ? { redirect: location.href, reason: 'sessionExpired' }
          : { redirect: location.href },
      })
    }
  },
  component: AuthenticatedLayout,
  errorComponent: LayoutErrorBoundary,
  notFoundComponent: NotFoundPage,
})
