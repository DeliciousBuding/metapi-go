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

function isAuthBundle(value: unknown): value is AuthBundle {
  if (!isRecord(value)) return false
  return (
    typeof value.access_token === 'string' &&
    value.access_token.length > 0 &&
    typeof value.token_type === 'string' &&
    value.token_type.length > 0 &&
    typeof value.access_expires_at === 'number' &&
    Number.isFinite(value.access_expires_at) &&
    value.access_expires_at > 0
  )
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

  const expiresAt = nowMs + Math.max(1, Math.trunc(ttlMs))
  target.setItem(AUTH_TOKEN_STORAGE_KEY, cleanToken)
  target.setItem(AUTH_TOKEN_EXPIRES_AT_STORAGE_KEY, String(expiresAt))
}

function publishAuthSessionEvent(
  event: AuthSessionEventName,
  _sid?: string
): void {
  // TODO(tab-sync): include the SID payload and have the store reconcile on
  // receipt. Storage-only tokens don't need it yet, so this is a no-op-ish
  // ping that at least wakes other tabs to re-read storage.
  const channel = resolveAuthSessionChannel()
  if (!channel) return
  try {
    channel.postMessage({ event })
  } catch {
    // channel closed / sandboxed; ignore.
  }
}

/**
 * Authentication session module for metapi-go.
 *
 * Adapted from the newapi auth-session design (single-flight refresh, tab
 * sync, bundle shape) but bound to metapi-go's existing auth contract: a
 * Bearer access token persisted to localStorage (`auth_token` +
 * `auth_token_expires_at`, ms epoch) and the HttpOnly `meta_monitor_auth`
 * cookie used by the monitor embed.
 *
 * The token storage keys are kept byte-for-byte compatible with the legacy
 * `authSession.ts` so existing sessions survive the frontend rewrite
 * (seamless-migration hard constraint, plan §3.1).
 *
 * TODO(stores): once `@/stores/auth-store` (Zustand) lands, delegate
 * getAccessToken / getAuthSession / setAuthBundle / clearAuthentication to it
 * and keep this module as the refresh + tab-sync facade. Until then this
 * module is the interim source of truth so the lib/ layer is self-contained.
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

export type RefreshOutcome =
  | { kind: 'authenticated'; bundle: AuthBundle }
  | { kind: 'anonymous' }
  | { kind: 'transient_error'; error: unknown }
  | { kind: 'out_of_sync'; code?: string }

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

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object'
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

export function hasValidAuthSession(
  storage?: StorageLike | null,
  nowMs: number = Date.now()
): boolean {
  return Boolean(getAuthToken(storage, nowMs))
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

/**
 * Build an AuthBundle from the currently stored token, when it is still
 * valid. user/session are unavailable from storage alone (TODO: hydrate from
 * /api/settings/auth/info once the auth-store bootstrap lands).
 */

export function getAuthSession(
  storage?: StorageLike | null,
  nowMs: number = Date.now()
): LoginSession | null {
  // TODO(stores): return the live LoginSession from the Zustand auth-store.
  // Storage-only tokens carry no session metadata, so the SID used for GET
  // dedup falls back to 'anonymous' until the store lands.
  void storage
  void nowMs
  return null
}

export function clearAuthentication(
  synchronizeTabs = true,
  _bootstrapState: AuthBootstrapState = 'complete'
): void {
  clearAuthSession()
  authEpoch += 1
  if (synchronizeTabs) {
    publishAuthSessionEvent('signed_out')
  }
}

/**
 * Apply a token rotation received in a response body (newapi-style).
 * metapi-go's backend does not currently rotate tokens, so this is a parity
 * stub that persists the bundle when one is present.
 */
export function applyAuthRotation(value: unknown): void {
  if (!isAuthBundle(value)) return
  setAuthBundle(value)
  authEpoch += 1
  publishAuthSessionEvent('authenticated')
}

// ---------------------------------------------------------------------------
// refreshAuthentication — single-flight + backoff scaffold (STUB)
//
// metapi-go has no /auth/refresh endpoint yet: the legacy client simply
// cleared the token and reloaded the page on 401. To preserve that behaviour
// while leaving the newapi-style refresh structure in place, the stub:
//   1. single-flights concurrent callers onto one promise,
//   2. returns 'authenticated' if the stored token is still valid,
//   3. otherwise clears and returns 'anonymous' (so the 401 path redirects
//      to /sign-in, matching the legacy reload-on-expiry behaviour).
//
// TODO(refresh): wire the real refresh endpoint (and 409 race/mismatch
// handling) once the backend exposes one. The race-delay constants below are
// kept from newapi so the backoff ladder is ready to fill in.
// ---------------------------------------------------------------------------

const refreshRaceDelays = [80, 200, 500] as const
let refreshPromise: Promise<RefreshOutcome> | null = null
let authEpoch = 0

function waitForRefreshRace(delay: number): Promise<void> {
  return new Promise((resolve) => globalThis.setTimeout(resolve, delay))
}

export function refreshAuthentication(): Promise<RefreshOutcome> {
  if (!refreshPromise) {
    refreshPromise = (async (): Promise<RefreshOutcome> => {
      const bundle = currentValidAuthBundle()
      if (bundle) {
        return { kind: 'authenticated', bundle }
      }

      // TODO(refresh): call the real refresh endpoint here, then parse the
      // AuthBundle and applyAuthRotation it. For now, no refresh endpoint
      // exists, so clear and report anonymous — the 401 interceptor will
      // redirect to /sign-in, matching the legacy reload-on-expiry flow.
      void refreshRaceDelays
      void waitForRefreshRace
      clearAuthentication(true)
      return { kind: 'anonymous' }
    })().finally(() => {
      refreshPromise = null
    })
  }
  return refreshPromise
}

export async function bootstrapAuthentication(): Promise<RefreshOutcome> {
  const bundle = currentValidAuthBundle()
  if (bundle) {
    return { kind: 'authenticated', bundle }
  }
  return refreshAuthentication()
}

// ---------------------------------------------------------------------------
// Tab sync (STUB)
//
// newapi uses a dedicated auth-session-sync module + BroadcastChannel to keep
// the Zustand auth-store in step across tabs. metapi-go's storage-only token
// means cross-tab sync is mostly handled by the storage event already; the
// structured publish/subscribe here is a parity scaffold for when the store
// lands.
//
// TODO(tab-sync): wire auth-session-sync events into the Zustand store.
// ---------------------------------------------------------------------------

type AuthSessionEventName = 'authenticated' | 'signed_out'
const AUTH_SESSION_CHANNEL_NAME = 'metapi-go:auth-session'

let authSessionChannel: BroadcastChannel | null = null

function resolveAuthSessionChannel(): BroadcastChannel | null {
  if (authSessionChannel) return authSessionChannel
  if (typeof BroadcastChannel === 'undefined') return null
  authSessionChannel = new BroadcastChannel(AUTH_SESSION_CHANNEL_NAME)
  return authSessionChannel
}
