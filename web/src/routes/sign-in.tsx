// metapi-go/routes — sign-in route (public).
// validateSearch accepts an optional `redirect` param (sanitized before
// navigation) and an optional `reason` param (contextual notice, e.g. a
// token rotation elsewhere ended the session). beforeLoad short-circuits to
// the redirect target if already authenticated.

import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'

import { SignInPage } from '@/features/auth'
import { hasValidAuthSessionSafe } from '@/lib/auth-session'
import { sanitizeAuthRedirect } from '@/lib/helpers/sanitize-auth-redirect'
import { asStringParam, stringSearchParam } from '@/lib/helpers/searchParams'

export const signInSearchSchema = z.object({
  redirect: stringSearchParam,
  reason: stringSearchParam,
})

function SignInComponent() {
  const { redirect, reason } = Route.useSearch()
  return (
    <SignInPage
      redirectTo={asStringParam(redirect)}
      noticeReason={asStringParam(reason)}
    />
  )
}

export const Route = createFileRoute('/sign-in')({
  component: SignInComponent,
  validateSearch: signInSearchSchema,
  beforeLoad: ({ search }) => {
    // localStorage can throw (SecurityError) in sandboxed/blocked-storage
    // contexts; treat that as unauthenticated rather than crashing the guard.
    if (hasValidAuthSessionSafe(localStorage)) {
      const target =
        sanitizeAuthRedirect(search?.redirect, window.location.origin) ?? '/'
      throw redirect({ href: target, replace: true })
    }
  },
})
