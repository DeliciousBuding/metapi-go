// metapi-go/lib/helpers — auth-redirect sanitizer.
// Prevents open-redirect attacks by constraining the `redirect` search param
// to same-origin paths. Ported from newapi (simplified: no getSavedLanguage
// since metapi-go tokens carry no user profile in the skeleton).
//
// Lives in lib/ (not features/auth) because the shared HTTP client
// (src/lib/http-client.ts) applies the same whitelist when building its 401
// sign-in redirect — lib must never import from features/.

const allowedRedirectProtocols = new Set(['http:', 'https:'])

/**
 * Sanitize a post-login redirect target. Accepts only same-origin absolute
 * or root-relative paths; returns the path+search+hash or null if unsafe.
 */
export function sanitizeAuthRedirect(
  value: unknown,
  origin: string
): string | null {
  if (typeof value !== 'string') return null

  const target = value.trim()
  if (!target || target.includes('\\') || target.startsWith('//')) return null

  let trustedOrigin: URL
  try {
    trustedOrigin = new URL(origin)
  } catch {
    return null
  }
  if (!allowedRedirectProtocols.has(trustedOrigin.protocol)) return null

  let redirectURL: URL
  try {
    redirectURL = target.startsWith('/')
      ? new URL(target, trustedOrigin.origin)
      : new URL(target)
  } catch {
    return null
  }

  if (
    !allowedRedirectProtocols.has(redirectURL.protocol) ||
    redirectURL.origin !== trustedOrigin.origin
  ) {
    return null
  }

  return `${redirectURL.pathname}${redirectURL.search}${redirectURL.hash}`
}
