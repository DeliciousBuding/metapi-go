// metapi-go/routes — authenticated layout route (auth guard).
// beforeLoad checks the localStorage session (storage is the source of truth
// for cold-load guards, avoiding a Zustand hydration race). On failure,
// redirects to /sign-in with the current href as the redirect target.

import { createFileRoute, redirect } from '@tanstack/react-router'

import { AuthenticatedLayout } from '@/components/layout'
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
})
