// metapi-go/lib/helpers — sanitizeAuthRedirect: the open-redirect guard behind
// every post-authentication jump.
//
// Three callers depend on it agreeing with itself, which is why the rule lives in
// one function instead of at each site: the sign-in route validates its
// `redirect` search param, the login form validates the value it is about to
// navigate to, and the shared HTTP client (lib/http-client.ts) validates the
// return target it builds for a 401. It sits in lib/ rather than features/auth
// because lib must never import from features/.
//
// Accepted: a root-relative path, or an absolute http(s) URL on this exact
// origin. Returned: pathname + search + hash only, so no origin ever travels back
// into a location the router would re-parse.
//
// Rejected (null): anything else — another origin, a `//host` protocol-relative
// form, a backslash (browsers normalise `/\evil.example` into `//evil.example`),
// a non-http(s) scheme such as `javascript:` or `data:`, non-string input, and an
// `origin` that is not itself a trustworthy http(s) URL.

const SAFE_PROTOCOLS = new Set(['http:', 'https:'])

/**
 * Sanitise a post-login redirect target against `origin`.
 *
 * @returns the path, query and fragment to navigate to, or null when the value
 *          must not be followed.
 */
export function sanitizeAuthRedirect(
  value: unknown,
  origin: string
): string | null {
  if (typeof value !== 'string') return null

  const target = value.trim()
  if (target === '' || target.includes('\\') || target.startsWith('//')) {
    return null
  }

  const trustedOrigin = parseTrustedOrigin(origin)
  if (trustedOrigin === null) return null

  const resolved = resolveTarget(target, trustedOrigin)
  if (resolved === null) return null
  if (!SAFE_PROTOCOLS.has(resolved.protocol)) return null
  if (resolved.origin !== trustedOrigin) return null

  return `${resolved.pathname}${resolved.search}${resolved.hash}`
}

/**
 * The origin to trust, or null when `origin` is not an http(s) URL. Checked
 * first so a caller that hands over a hostile or malformed origin cannot turn
 * every absolute target into a "same-origin" one.
 */
function parseTrustedOrigin(origin: string): string | null {
  try {
    const url = new URL(origin)
    return SAFE_PROTOCOLS.has(url.protocol) ? url.origin : null
  } catch {
    return null
  }
}

/** Resolves a root-relative or absolute target; null when it does not parse. */
function resolveTarget(target: string, trustedOrigin: string): URL | null {
  try {
    return target.startsWith('/')
      ? new URL(target, trustedOrigin)
      : new URL(target)
  } catch {
    return null
  }
}
