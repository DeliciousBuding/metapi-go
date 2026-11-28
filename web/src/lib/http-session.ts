import { sanitizeAuthRedirect } from '@/lib/helpers/sanitize-auth-redirect'

import { extractApiErrorBody } from './http-error-message'

function buildSignInHref(): string {
  if (typeof window === 'undefined') return '/sign-in'
  // Preserve the interrupted location so sign-in can send the user back.
  // The value is built from the current location (always same-origin) but
  // still runs through the redirect sanitizer — the same whitelist the
  // sign-in page applies when consuming the param.
  const returnTarget = sanitizeAuthRedirect(
    `${window.location.pathname}${window.location.search}`,
    window.location.origin
  )
  if (!returnTarget) return '/sign-in'
  return `/sign-in?redirect=${encodeURIComponent(returnTarget)}`
}

export function redirectToSignIn(): void {
  if (
    typeof window !== 'undefined' &&
    window.location.pathname !== '/sign-in'
  ) {
    window.location.replace(buildSignInHref())
  }
}

/** True when a response body asks for master-token re-confirmation. */
export function isReauthRequiredBody(data: unknown): boolean {
  return (
    !!data &&
    typeof data === 'object' &&
    (data as { reauthRequired?: unknown }).reauthRequired === true
  )
}

/** Thrown by the fetch wrapper when a sensitive op needs re-confirmation. */
export class ReauthRequiredError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'ReauthRequiredError'
  }
}

/**
 * True for errors (axios or fetch-wrapper) that represent a sensitive-op
 * re-confirmation demand rather than a session/auth failure. Callers surface
 * the master-token prompt and replay the request with X-Admin-Confirm-Token.
 */
export function isReauthRequired(error: unknown): boolean {
  if (error instanceof ReauthRequiredError) return true
  return isReauthRequiredBody(extractApiErrorBody(error))
}
