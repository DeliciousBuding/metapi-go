// metapi-go/routes — sign-in route (public).
// validateSearch accepts an optional `redirect` param (sanitized before
// navigation). beforeLoad short-circuits to the redirect target if already
// authenticated.

import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'

import { SignInPage } from '@/features/auth'
import { sanitizeAuthRedirect } from '@/features/auth/lib/auth-redirect'
import { hasValidAuthSession } from '@/lib/auth-session'

const searchSchema = z.object({
  redirect: z.string().optional(),
})

function SignInComponent() {
  const { redirect } = Route.useSearch()
  return <SignInPage redirectTo={redirect} />
}

export const Route = createFileRoute('/sign-in')({
  component: SignInComponent,
  validateSearch: searchSchema,
  beforeLoad: ({ search }) => {
    if (hasValidAuthSession(localStorage)) {
      const target =
        sanitizeAuthRedirect(search?.redirect, window.location.origin) ?? '/'
      throw redirect({ href: target, replace: true })
    }
  },
})
