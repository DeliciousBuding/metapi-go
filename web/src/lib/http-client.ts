import axios, { type AxiosRequestConfig } from 'axios'

import { sanitizeAuthRedirect } from '@/features/auth/lib/auth-redirect'
import i18n from '@/i18n/config'
import {
  clearAuthentication,
  clearAuthSession,
  getAccessToken,
  resolveAuthenticationAfterUnauthorized,
} from '@/lib/auth-session'
import { toast } from '@/lib/toast'

declare module 'axios' {
  export interface AxiosRequestConfig {
    /** Skip the business-level `success: false` toast on the response body. */
    skipBusinessError?: boolean
    /** Skip all error toasts (caller will surface the error itself). */
    skipErrorHandler?: boolean
    /** Skip GET request dedup for this call. */
    disableDuplicate?: boolean
    /** Skip the 401 → storage re-read → one replay flow. */
    skipAuthRetry?: boolean
    /** Marker set on a request that is already a 401 retry, to avoid loops. */
    authRetry?: boolean
  }
}

export type ApiRequestConfig = AxiosRequestConfig

/**
 * Shared axios instance for all admin/proxy JSON requests.
 *
 * `baseURL` is intentionally empty: the dev proxy (rsbuild) routes both
 * `/api` (admin) and `/v1` (proxy) prefixes to the Go backend on port 4000,
 * and the business API methods carry their full prefix in each URL. Setting
 * baseURL to `/api` would double-prefix `/api/...` and break `/v1/...`.
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
// Request interceptor — inject Authorization Bearer from the auth session.
// ---------------------------------------------------------------------------

apiClient.interceptors.request.use((config) => {
  const accessToken = getAccessToken()
  if (accessToken) {
    config.headers.Authorization = `Bearer ${accessToken}`
  }
  return config
})

// ---------------------------------------------------------------------------
// Response + error interceptors — business error toasts, one 401 replay when
// another tab replaced the token, and a catch-all error toast.
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
 * (rotated/expired admin token) rather than an IP-allowlist rejection.
 * Mirrors the classification in features/auth/api.ts.
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

    // A 403 with the backend's "Invalid token" body ends the session exactly
    // like a 401 (the token was rotated elsewhere). IP-allowlist 403s carry
    // "IP not allowed" and must NOT clear the session — they fall through to
    // the generic error toast below.
    const sessionInvalid =
      status === 401 ||
      (status === 403 &&
        isInvalidTokenMessage(resolveResponseMessage(error?.response?.data)))
    const sessionInvalidMessageKey =
      status === 401 ? 'common.sessionExpired' : 'common.tokenInvalid'

    if (sessionInvalid) {
      if (config && !config.skipAuthRetry && !config.authRetry) {
        config.authRetry = true
        const outcome = resolveAuthenticationAfterUnauthorized()
        if (outcome.kind === 'authenticated') {
          const token = getAccessToken()
          if (token) {
            config.headers = {
              ...config.headers,
              Authorization: `Bearer ${token}`,
            }
          }
          return apiClient.request(config)
        }

        if (!skipErrorHandler) toast.error(i18n.t(sessionInvalidMessageKey))
        redirectToSignIn()
      } else if (config?.authRetry) {
        clearAuthentication()
        if (!skipErrorHandler) toast.error(i18n.t(sessionInvalidMessageKey))
        redirectToSignIn()
      } else if (!skipErrorHandler) {
        toast.error(i18n.t(sessionInvalidMessageKey))
      }
    } else if (!skipErrorHandler) {
      const message =
        resolveResponseMessage(error?.response?.data) ||
        error?.message ||
        i18n.t('common.requestFailed')
      toast.error(message)
    }
    throw error
  }
)

// ---------------------------------------------------------------------------
// fetchAuthenticated — fetch wrapper for the few streaming / raw-Response
// endpoints (SSE log streams, /v1/files content, test chat/proxy streams)
// that cannot flow through the axios JSON interceptors.
//
// Auth failure handling matches the axios interceptor contract: a 401 or an
// "Invalid token" 403 clears the session and redirects to /sign-in (keeping
// the return path). Reloading the page instead would loop: the reload lands
// on the same authenticated page, which re-requests with the same dead token.
// An IP-allowlist 403 does NOT end the session — it surfaces as a plain
// error so callers can toast it.
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

async function isInvalidTokenResponse(res: Response): Promise<boolean> {
  try {
    // Clone so classification never consumes the body the caller may need.
    const text = await res.clone().text()
    return text.toLowerCase().includes('invalid token')
  } catch {
    return false
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

  const token = getAccessToken()
  if (!token) {
    clearAuthSession()
    redirectToSignIn()
    throw new Error('Session expired')
  }

  const headers = new Headers(fetchOptions.headers ?? {})
  headers.set('Authorization', `Bearer ${token}`)
  if (fetchOptions.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  try {
    const res = await fetch(url, {
      ...fetchOptions,
      signal: controller.signal,
      headers,
    })
    if (res.status === 401 || res.status === 403) {
      const sessionEnded =
        res.status === 401 || (await isInvalidTokenResponse(res))
      if (sessionEnded) {
        clearAuthSession()
        redirectToSignIn()
        throw new Error('Session expired')
      }
      // IP-allowlist (or other) 403: keep the session, surface the body.
      throw new Error(await extractResponseErrorMessage(res))
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
