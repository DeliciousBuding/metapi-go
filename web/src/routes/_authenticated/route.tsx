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

import { createFileRoute, redirect } from '@tanstack/react-router'

import { AuthenticatedLayout, LayoutErrorBoundary } from '@/components/layout'
import { hasValidAuthSession } from '@/lib/auth-session'

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: ({ location }) => {
    if (!hasValidAuthSession(localStorage)) {
      throw redirect({
        to: '/sign-in',
        search: { redirect: location.href },
      })
    }
  },
  component: AuthenticatedLayout,
  errorComponent: LayoutErrorBoundary,
})
