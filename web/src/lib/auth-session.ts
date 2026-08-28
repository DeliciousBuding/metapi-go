/**
 * Authentication session module for metapi-go (#1034 session model).
 *
 * The credential is an HttpOnly, SameSite=Strict cookie (`metapi_session`)
 * minted by POST /api/auth/login and managed entirely by the Go backend
 * (sliding TTL, logout revocation). JavaScript never sees the credential:
 * localStorage only carries NON-sensitive metadata — the session expiry —
 * so cold-load route guards and the "session expired" notice work without a
 * network round trip.
 *
 * Legacy plaintext keys (`auth_token` / `auth_token_expires_at` from the
 * token-in-localStorage era) are wiped on every session access, so existing
 * browsers drop the master token the first time this build loads.
 */

const LEGACY_TOKEN_STORAGE_KEY = 'auth_token'
const LEGACY_TOKEN_EXPIRES_AT_STORAGE_KEY = 'auth_token_expires_at'
const SESSION_META_STORAGE_KEY = 'metapi_session_meta'

export interface SessionMeta {
  /** Sliding session expiry, ms epoch (server is authoritative). */
  expiresAtMs: number
}

/** Server answer for GET /api/auth/session. */
export interface SessionStatus {
  authenticated: boolean
  /** "session" = cookie track, "token" = Bearer master token track. */
  source?: 'session' | 'token'
  /** RFC3339 UTC — present on the session track. */
  expiresAt?: string
}

export type AuthenticationOutcome =
  | { kind: 'authenticated'; expiresAtMs: number }
  | { kind: 'anonymous'; expired: boolean }

export type AuthBootstrapState = 'idle' | 'checking' | 'complete'

type StorageLike = {
  getItem: (key: string) => string | null
  setItem: (key: string, value: string) => void
  removeItem: (key: string) => void
}

function resolveStorage(storage?: StorageLike | null): StorageLike | null {
  if (storage) return storage
  if (
    typeof localStorage !== 'undefined' &&
    typeof (localStorage as { getItem?: unknown }).getItem === 'function'
  ) {
    return localStorage
  }
  return null
}

/** Result of the one-time startup probe (root route beforeLoad). */
let bootOutcome: AuthenticationOutcome | null = null

/**
 * Delete the legacy plaintext master-token keys. Any browser that used the
 * pre-#1034 frontend still carries the token here; the first session access
 * under this build removes it.
 */
function wipeLegacyPlaintextToken(target: StorageLike | null): void {
  if (!target) return
  try {
    target.removeItem(LEGACY_TOKEN_STORAGE_KEY)
    target.removeItem(LEGACY_TOKEN_EXPIRES_AT_STORAGE_KEY)
  } catch {
    // Storage access can throw in sandboxed contexts; the wipe is best-effort.
  }
}

export function persistSessionMeta(
  expiresAtMs: number,
  storage?: StorageLike | null
): void {
  const target = resolveStorage(storage)
  if (!target) return
  wipeLegacyPlaintextToken(target)
  try {
    target.setItem(
      SESSION_META_STORAGE_KEY,
      JSON.stringify({ expiresAtMs } satisfies SessionMeta)
    )
  } catch {
    // Full/blocked storage degrades to in-memory boot state only.
  }
}

export function readSessionMeta(
  storage?: StorageLike | null
): SessionMeta | null {
  const target = resolveStorage(storage)
  if (!target) return null
  wipeLegacyPlaintextToken(target)
  try {
    const raw = target.getItem(SESSION_META_STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as { expiresAtMs?: unknown }
    if (typeof parsed.expiresAtMs !== 'number') return null
    return { expiresAtMs: parsed.expiresAtMs }
  } catch {
    return null
  }
}

/** Clear all client-side session state (metadata + legacy plaintext keys). */
export function clearAuthSession(storage?: StorageLike | null): void {
  const target = resolveStorage(storage)
  if (!target) return
  wipeLegacyPlaintextToken(target)
  try {
    target.removeItem(SESSION_META_STORAGE_KEY)
  } catch {
    // best-effort
  }
}

export function clearAuthentication(): void {
  clearAuthSession()
  bootOutcome = { kind: 'anonymous', expired: false }
}

/**
 * Probe the server once at startup. The cookie travels automatically; on
 * success the (non-sensitive) expiry is mirrored to localStorage for
 * synchronous cold-load guards. On failure/unreachable the local state is
 * cleared so the sign-in page shows.
 */
export async function bootstrapAuthentication(): Promise<AuthenticationOutcome> {
  const localMeta = readSessionMeta()
  // "Expired" covers both the TTL elapsing AND the server ending a session
  // that the client still considered live (logout elsewhere, revocation on
  // token rotation): either way the user had a session that is now over.
  const expiredLocally = localMeta !== null

  try {
    const res = await fetch('/api/auth/session', {
      method: 'GET',
      headers: { Accept: 'application/json' },
    })
    if (res.ok) {
      const status = (await res.json()) as SessionStatus
      if (status.authenticated && status.expiresAt) {
        const expiresAtMs = Date.parse(status.expiresAt)
        if (Number.isFinite(expiresAtMs)) {
          persistSessionMeta(expiresAtMs)
          bootOutcome = { kind: 'authenticated', expiresAtMs }
          return bootOutcome
        }
      }
    }
  } catch {
    // Server unreachable during boot: fall through to the anonymous path.
    // An authenticated cookie would still work once requests resume; the
    // 401 interceptor reconciles if the server actually rejects us later.
  }

  clearAuthSession()
  bootOutcome = { kind: 'anonymous', expired: expiredLocally }
  return bootOutcome
}

/**
 * Synchronous guard for route beforeLoad. Runs AFTER
 * {@link bootstrapAuthentication} (root route), so the boot outcome is the
 * source of truth; the stored meta covers the window before bootstrap
 * resolves (e.g. tests exercising the guard directly).
 */
export function hasValidAuthSession(
  storage?: StorageLike | null,
  nowMs: number = Date.now()
): boolean {
  if (bootOutcome) return bootOutcome.kind === 'authenticated'
  const meta = readSessionMeta(storage)
  return meta !== null && meta.expiresAtMs > nowMs
}

/**
 * Guarded variant of {@link hasValidAuthSession}: storage access can throw
 * (SecurityError) in sandboxed/blocked-storage contexts. Treat that as
 * unauthenticated so route guards never crash on a hostile localStorage.
 */
export function hasValidAuthSessionSafe(
  storage?: StorageLike | null,
  nowMs: number = Date.now()
): boolean {
  try {
    return hasValidAuthSession(storage, nowMs)
  } catch {
    return false
  }
}

/** True when stored session metadata exists but its expiry has passed. */
export function isAuthSessionExpired(
  storage?: StorageLike | null,
  nowMs: number = Date.now()
): boolean {
  try {
    const meta = readSessionMeta(storage)
    return meta !== null && meta.expiresAtMs <= nowMs
  } catch {
    return false
  }
}

/**
 * True when the startup bootstrap (or a post-401 re-read) ended anonymous
 * because a stored session had passed its TTL. Used by the authenticated
 * route guard to attach `reason=sessionExpired` to the sign-in redirect.
 */
export function wasAuthSessionExpiredOnLastBoot(): boolean {
  return (
    bootOutcome !== null &&
    bootOutcome.kind === 'anonymous' &&
    bootOutcome.expired
  )
}

/**
 * Called by the HTTP layer after a 401: the server-side session is gone, so
 * every client-side trace is cleared and the caller redirects to sign-in.
 * Kept as a named export for the http-client contract.
 */
export function resolveAuthenticationAfterUnauthorized(): AuthenticationOutcome {
  const expired =
    isAuthSessionExpired() || bootOutcome?.kind === 'authenticated'
  clearAuthSession()
  bootOutcome = { kind: 'anonymous', expired: Boolean(expired) }
  return bootOutcome
}
