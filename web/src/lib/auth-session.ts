const MONITOR_AUTH_COOKIE_NAME = 'meta_monitor_auth'
const MONITOR_AUTH_COOKIE_PATH = '/monitor-proxy/'

function clearMonitorAuthCookie(
  doc: CookieWriter | null | undefined = typeof document !== 'undefined'
    ? document
    : null
): void {
  if (!doc) return
  try {
    const expire = 'Max-Age=0; expires=Thu, 01 Jan 1970 00:00:00 GMT'
    doc.cookie = `${MONITOR_AUTH_COOKIE_NAME}=; Path=${MONITOR_AUTH_COOKIE_PATH}; ${expire}`
    doc.cookie = `${MONITOR_AUTH_COOKIE_NAME}=; Path=/; ${expire}`
  } catch {
    // document may be restricted (sandboxed); ignore.
  }
}

function currentValidAuthBundle(
  storage?: StorageLike | null,
  nowMs: number = Date.now()
): AuthBundle | null {
  const token = getAuthToken(storage, nowMs)
  if (!token) return null
  const target = resolveStorage(storage)
  const expiresAtMs = Number(
    target?.getItem(AUTH_TOKEN_EXPIRES_AT_STORAGE_KEY) ?? 0
  )
  if (!Number.isFinite(expiresAtMs) || expiresAtMs <= nowMs) return null
  return {
    access_token: token,
    token_type: 'Bearer',
    access_expires_at: Math.floor(expiresAtMs / 1000),
    user: null,
    session: null,
  }
}

function persistAuthSession(
  storage: StorageLike | null | undefined,
  token: string,
  ttlMs: number = AUTH_SESSION_DURATION_MS,
  nowMs: number = Date.now()
): void {
  const target = resolveStorage(storage)
  if (!target) return

  const cleanToken = (token || '').trim()
  if (!cleanToken) {
    clearAuthSession(target)
    return
  }

  // A fresh session is now stored — any previously recorded expiry no
  // longer applies to the next guard check.
  lastAuthCheckExpired = false
  const expiresAt = nowMs + Math.max(1, Math.trunc(ttlMs))
  target.setItem(AUTH_TOKEN_STORAGE_KEY, cleanToken)
  target.setItem(AUTH_TOKEN_EXPIRES_AT_STORAGE_KEY, String(expiresAt))
}

/**
 * Authentication session module for metapi-go.
 *
 * Bound to metapi-go's existing auth contract: a Bearer access token
 * persisted to localStorage (`auth_token` + `auth_token_expires_at`, ms
 * epoch) and the HttpOnly `meta_monitor_auth` cookie used by the monitor
 * embed.
 *
 * The token storage keys are kept byte-for-byte compatible with the legacy
 * `authSession.ts` so existing sessions survive the frontend rewrite
 * (seamless-migration hard constraint).
 */

const AUTH_TOKEN_STORAGE_KEY = 'auth_token'
const AUTH_TOKEN_EXPIRES_AT_STORAGE_KEY = 'auth_token_expires_at'
export const AUTH_SESSION_DURATION_MS = 12 * 60 * 60 * 1000

/** Must match handler/admin/monitor.go monitorAuthCookie. */
export interface AuthUser {
  id: number
  username: string
  role: number
}

export interface LoginSession {
  sid: string
  current: boolean
  login_method: string
  ip: string
  user_agent: string
  created_at: number
  last_active_at: number
  expires_at: number
}

export interface AuthBundle {
  access_token: string
  token_type: string
  /** Unix seconds — newapi contract. Stored to localStorage as ms epoch. */
  access_expires_at: number
  user?: AuthUser | null
  session?: LoginSession | null
}

export type AuthenticationOutcome =
  | { kind: 'authenticated'; bundle: AuthBundle }
  | { kind: 'anonymous' }

export type AuthBootstrapState = 'idle' | 'checking' | 'complete'

type StorageLike = {
  getItem: (key: string) => string | null
  setItem: (key: string, value: string) => void
  removeItem: (key: string) => void
}

type CookieWriter = {
  cookie?: string
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

/**
 * Best-effort document.cookie clear for meta_monitor_auth.
 * Path must match createSession (/monitor-proxy/) or the browser ignores it.
 * Also clears legacy Path=/ in case an older mint used it.
 * Prefer DELETE /api/monitor/session for the HttpOnly cookie.
 */

export function clearAuthSession(storage?: StorageLike | null): void {
  const target = resolveStorage(storage)
  if (!target) return
  target.removeItem(AUTH_TOKEN_STORAGE_KEY)
  target.removeItem(AUTH_TOKEN_EXPIRES_AT_STORAGE_KEY)
  clearMonitorAuthCookie()
}

/**
 * Read the access token from storage, validating ms-based expiry.
 * Returns null when missing or expired (and clears stale state).
 */
export function getAuthToken(
  storage?: StorageLike | null,
  nowMs: number = Date.now()
): string | null {
  const target = resolveStorage(storage)
  if (!target) return null

  const token = (target.getItem(AUTH_TOKEN_STORAGE_KEY) || '').trim()
  if (!token) return null

  const expiresAtRaw = target.getItem(AUTH_TOKEN_EXPIRES_AT_STORAGE_KEY)
  if (!expiresAtRaw) {
    persistAuthSession(target, token, AUTH_SESSION_DURATION_MS, nowMs)
    return token
  }

  const expiresAt = Number(expiresAtRaw)
  if (!Number.isFinite(expiresAt) || expiresAt <= nowMs) {
    clearAuthSession(target)
    return null
  }

  return token
}

/** Convenience: just the token string, for interceptor + fetch wrappers. */
export function getAccessToken(
  storage?: StorageLike | null,
  nowMs: number = Date.now()
): string | null {
  return getAuthToken(storage, nowMs)
}

/**
 * True only when a token exists and its expiry has already passed (or the
 * expiry value is corrupt). Missing token and live token both return false,
 * so a caller can distinguish "no session at all" from "session expired" —
 * {@link getAuthToken} collapses both to null and cannot tell them apart
 * ({@link clearAuthSession} already wiped the stale entry by then).
 */
export function isAuthSessionExpired(
  storage?: StorageLike | null,
  nowMs: number = Date.now()
): boolean {
  const target = resolveStorage(storage)
  if (!target) return false

  const token = (target.getItem(AUTH_TOKEN_STORAGE_KEY) || '').trim()
  if (!token) return false

  const expiresAtRaw = target.getItem(AUTH_TOKEN_EXPIRES_AT_STORAGE_KEY)
  if (!expiresAtRaw) return false

  const expiresAt = Number(expiresAtRaw)
  return !Number.isFinite(expiresAt) || expiresAt <= nowMs
}

/**
 * Records whether the most recent top-level auth check (startup bootstrap or
 * post-401 re-read) ended anonymous because a stored session was expired.
 *
 * Both checks call {@link getAuthToken}, which wipes the stale entry as a
 * side effect — after that "expired" is indistinguishable from "never logged
 * in", so the fact is stashed here first. The record is set just before the
 * storage-clearing check runs and cleared whenever a fresh session is
 * persisted (login), so a later in-app navigation never misreports an
 * expired session the user already replaced by signing in again.
 */
let lastAuthCheckExpired = false

/**
 * True when the startup bootstrap (or a post-401 re-read) ended anonymous
 * because the stored session had passed its TTL. Used by the authenticated
 * route guard to attach `reason=sessionExpired` to the sign-in redirect.
 */
export function wasAuthSessionExpiredOnLastBoot(): boolean {
  return lastAuthCheckExpired
}

export function hasValidAuthSession(
  storage?: StorageLike | null,
  nowMs: number = Date.now()
): boolean {
  return Boolean(getAuthToken(storage, nowMs))
}

/**
 * Guarded variant of {@link hasValidAuthSession}: storage access can throw
 * (SecurityError) in sandboxed/blocked-storage contexts. Treat that as
 * unauthenticated so route guards (sign-in beforeLoad, authenticated
 * layout) never crash on a hostile localStorage.
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

/**
 * Persist an AuthBundle to storage. Converts the bundle's
 * access_expires_at (unix seconds) to the ms epoch the legacy keys expect.
 */
export function setAuthBundle(
  bundle: AuthBundle,
  storage?: StorageLike | null
): void {
  const nowMs = Date.now()
  const ttlMs = Math.max(1, Math.trunc(bundle.access_expires_at * 1000) - nowMs)
  persistAuthSession(storage, bundle.access_token, ttlMs, nowMs)
}

export function clearAuthentication(): void {
  clearAuthSession()
}

/**
 * Re-read storage after a 401 so a token replaced by another tab can be used
 * for one retry. The backend has no refresh endpoint; without a newer valid
 * token the session is cleared and the caller redirects to sign-in.
 */
export function resolveAuthenticationAfterUnauthorized(): AuthenticationOutcome {
  // Record expiry before getAuthToken wipes the stale entry (the route
  // guard reads this to distinguish "session expired" from "never logged in").
  lastAuthCheckExpired = isAuthSessionExpired()
  const bundle = currentValidAuthBundle()
  if (bundle) {
    return { kind: 'authenticated', bundle }
  }

  clearAuthentication()
  return { kind: 'anonymous' }
}

export function bootstrapAuthentication(): AuthenticationOutcome {
  // Record expiry before getAuthToken wipes the stale entry (the route
  // guard reads this to distinguish "session expired" from "never logged in").
  lastAuthCheckExpired = isAuthSessionExpired()
  const bundle = currentValidAuthBundle()
  if (bundle) {
    return { kind: 'authenticated', bundle }
  }

  clearAuthentication()
  return { kind: 'anonymous' }
}
