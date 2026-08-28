import axios, { type AxiosRequestConfig } from 'axios'

import i18n from '@/i18n/config'
import { clearAuthentication, clearAuthSession } from '@/lib/auth-session'
import { sanitizeAuthRedirect } from '@/lib/helpers/sanitize-auth-redirect'
import { toast } from '@/lib/toast'

declare module 'axios' {
  export interface AxiosRequestConfig {
    /** Skip the business-level `success: false` toast on the response body. */
    skipBusinessError?: boolean
    /** Skip all error toasts (caller will surface the error itself). */
    skipErrorHandler?: boolean
    /** Skip GET request dedup for this call. */
    disableDuplicate?: boolean
    /** Skip the 401 -> clear session -> redirect flow (login owns errors). */
    skipAuthRetry?: boolean
  }
}

export type ApiRequestConfig = AxiosRequestConfig

/**
 * Shared axios instance for all admin/proxy JSON requests.
 *
 * Auth rides the HttpOnly `metapi_session` cookie (#1034): no Authorization
 * header is injected and no credential lives in localStorage. `baseURL` is
 * intentionally empty: the dev proxy (rsbuild) routes both `/api` (admin)
 * and `/v1` (proxy) prefixes to the Go backend on port 4000, and the
 * business API methods carry their full prefix in each URL.
 */
export const apiClient = axios.create({
  baseURL: '',
  withCredentials: true,
  headers: {
    'Cache-Control': 'no-store',
  },
})

// ---------------------------------------------------------------------------
// GET dedup — collapse identical in-flight GETs by URL and params.
// Disabled per-request via `disableDuplicate`.
// ---------------------------------------------------------------------------

const inFlightGet = new Map<string, Promise<unknown>>()
const originalGet = apiClient.get.bind(apiClient)

apiClient.get = ((url: string, config: ApiRequestConfig = {}) => {
  if (config.disableDuplicate) return originalGet(url, config)

  const params = config.params ? JSON.stringify(config.params) : '{}'
  const key = `${url}?${params}`
  const existingRequest = inFlightGet.get(key)
  if (existingRequest) return existingRequest

  const request = originalGet(url, config).finally(() => {
    inFlightGet.delete(key)
  })
  inFlightGet.set(key, request)
  return request
}) as typeof apiClient.get

// ---------------------------------------------------------------------------
// Response + error interceptors — business error toasts, session-end handling
// on 401, and reauthRequired pass-through for sensitive operations (#1034).
// ---------------------------------------------------------------------------

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

function redirectToSignIn(): void {
  if (
    typeof window !== 'undefined' &&
    window.location.pathname !== '/sign-in'
  ) {
    window.location.replace(buildSignInHref())
  }
}

/**
 * True when a 403 body carries the backend's "Invalid token" semantics
 * (rotated master token presented via the legacy Bearer track) rather than
 * an IP-allowlist rejection.
 */
function isInvalidTokenMessage(message: string | undefined): boolean {
  return !!message && message.toLowerCase().includes('invalid token')
}

function resolveResponseMessage(data: unknown): string | undefined {
  if (data && typeof data === 'object') {
    const message = (data as { message?: unknown }).message
    if (typeof message === 'string' && message) return message
    const error = (data as { error?: unknown }).error
    if (typeof error === 'string' && error) return error
  }
  return undefined
}

/** True when a response body asks for master-token re-confirmation. */
function isReauthRequiredBody(data: unknown): boolean {
  return (
    !!data &&
    typeof data === 'object' &&
    (data as { reauthRequired?: unknown }).reauthRequired === true
  )
}

/** Thrown by the fetch wrapper when a sensitive op needs re-confirmation. */
class ReauthRequiredError extends Error {
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
  const data = (error as { response?: { data?: unknown } } | null)?.response
    ?.data
  return isReauthRequiredBody(data)
}

apiClient.interceptors.response.use(
  (response) => {
    if (
      !response.config.skipBusinessError &&
      typeof response.data?.success === 'boolean' &&
      !response.data.success
    ) {
      const message =
        resolveResponseMessage(response.data) || i18n.t('common.requestFailed')
      toast.error(message)
    }
    return response
  },
  async (error) => {
    const config = error?.config as ApiRequestConfig | undefined
    const skipErrorHandler = config?.skipErrorHandler
    const status = error?.response?.status
    const data = error?.response?.data

    // Sensitive-op re-confirmation (#1034): the session is fine, the master
    // token must be presented again. Never clear/redirect — the caller opens
    // the reauth prompt and replays.
    if (status === 403 && isReauthRequiredBody(data)) {
      if (!skipErrorHandler) {
        toast.error(
          resolveResponseMessage(data) || i18n.t('common.requestFailed')
        )
      }
      throw error
    }

    // A 403 with the backend's "Invalid token" body ends the session exactly
    // like a 401 (legacy Bearer track after a rotation). IP-allowlist 403s
    // carry "IP not allowed" and must NOT clear the session — they fall
    // through to the generic error toast below.
    const sessionInvalid =
      status === 401 ||
      (status === 403 && isInvalidTokenMessage(resolveResponseMessage(data)))
    const sessionInvalidMessageKey =
      status === 401 ? 'common.sessionExpired' : 'common.tokenInvalid'

    if (sessionInvalid) {
      if (config?.skipAuthRetry) {
        // The login probe owns its error display; do not redirect either —
        // the user is already on the sign-in page.
        throw error
      }
      clearAuthentication()
      if (!skipErrorHandler) toast.error(i18n.t(sessionInvalidMessageKey))
      redirectToSignIn()
    } else if (!skipErrorHandler) {
      const message =
        resolveResponseMessage(data) ||
        error?.message ||
        i18n.t('common.requestFailed')
      toast.error(message)
    }
    throw error
  }
)

// ---------------------------------------------------------------------------
// fetchAuthenticated — fetch wrapper for the few streaming / raw-Response
// endpoints (SSE log streams, /v1/files content, test chat/proxy streams,
// backup export) that cannot flow through the axios JSON interceptors.
//
// Auth rides the session cookie (sent automatically for same-origin
// requests). Auth failure handling matches the axios interceptor contract:
// a 401 or an "Invalid token" 403 clears the session and redirects to
// /sign-in (keeping the return path); a reauthRequired 403 surfaces to the
// caller without touching the session; an IP-allowlist 403 surfaces as a
// plain error.
// ---------------------------------------------------------------------------

export type FetchAuthenticatedOptions = RequestInit & {
  timeoutMs?: number
}

async function extractResponseErrorMessage(res: Response): Promise<string> {
  let message = `HTTP ${res.status}`
  try {
    const text = await res.text()
    if (text) {
      try {
        const json = JSON.parse(text)
        if (json?.message && typeof json.message === 'string') {
          message = json.message
        } else if (json?.error && typeof json.error === 'string') {
          message = json.error
        } else if (
          json?.error?.message &&
          typeof json.error.message === 'string'
        ) {
          message = json.error.message
        } else {
          message = `${message}: ${text.slice(0, 120)}`
        }
      } catch {
        message = `${message}: ${text.slice(0, 120)}`
      }
    }
  } catch {
    // ignore body read errors
  }
  return message
}

type Classified403 =
  | { kind: 'invalid-token' }
  | { kind: 'reauth-required'; message: string }
  | { kind: 'other'; message: string }

async function classify403(res: Response): Promise<Classified403> {
  try {
    // Clone so classification never consumes the body the caller may need.
    const text = await res.clone().text()
    const lower = text.toLowerCase()
    if (lower.includes('"reauthrequired":true')) {
      let message = i18n.t('common.requestFailed')
      try {
        const json = JSON.parse(text)
        if (typeof json?.error === 'string' && json.error) message = json.error
      } catch {
        // fall back to the generic message
      }
      return { kind: 'reauth-required', message }
    }
    if (lower.includes('invalid token')) {
      return { kind: 'invalid-token' }
    }
    return { kind: 'other', message: await extractResponseErrorMessage(res) }
  } catch {
    return { kind: 'other', message: `HTTP ${res.status}` }
  }
}

export async function fetchAuthenticatedResponse(
  url: string,
  options: FetchAuthenticatedOptions = {}
): Promise<Response> {
  const {
    timeoutMs = 30_000,
    signal: externalSignal,
    ...fetchOptions
  } = options
  const controller = new AbortController()
  let timeoutHandle: ReturnType<typeof setTimeout> | null = setTimeout(() => {
    controller.abort()
  }, timeoutMs)
  let cleanupExternalSignal = () => {}

  if (externalSignal) {
    if (externalSignal.aborted) {
      controller.abort()
    } else {
      const abortHandler = () => controller.abort()
      externalSignal.addEventListener('abort', abortHandler, { once: true })
      cleanupExternalSignal = () =>
        externalSignal.removeEventListener('abort', abortHandler)
    }
  }

  const headers = new Headers(fetchOptions.headers ?? {})
  if (fetchOptions.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  try {
    const res = await fetch(url, {
      credentials: 'same-origin',
      ...fetchOptions,
      signal: controller.signal,
      headers,
    })
    if (res.status === 401) {
      clearAuthSession()
      redirectToSignIn()
      throw new Error('Session expired')
    }
    if (res.status === 403) {
      const verdict = await classify403(res)
      if (verdict.kind === 'reauth-required') {
        throw new ReauthRequiredError(verdict.message)
      }
      if (verdict.kind === 'invalid-token') {
        clearAuthSession()
        redirectToSignIn()
        throw new Error('Session expired')
      }
      // IP-allowlist (or other) 403: keep the session, surface the body.
      throw new Error(verdict.message)
    }
    return res
  } catch (error: unknown) {
    const name = (error as { name?: string } | null)?.name
    if (name === 'AbortError') {
      if (externalSignal?.aborted) throw error
      throw new Error(
        i18n.t('common.requestTimeout', {
          seconds: Math.max(1, Math.round(timeoutMs / 1000)),
        })
      )
    }
    throw error
  } finally {
    if (timeoutHandle) {
      clearTimeout(timeoutHandle)
      timeoutHandle = null
    }
    cleanupExternalSignal()
  }
}

export { extractResponseErrorMessage }
