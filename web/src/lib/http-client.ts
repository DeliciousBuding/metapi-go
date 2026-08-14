import axios, { type AxiosRequestConfig } from 'axios'

import {
  applyAuthRotation,
  clearAuthentication,
  clearAuthSession,
  getAccessToken,
  getAuthSession,
  refreshAuthentication,
} from '@/lib/auth-session'
import { toast } from '@/lib/toast'

import i18n from '@/i18n/config'

declare module 'axios' {
  export interface AxiosRequestConfig {
    /** Skip the business-level `success: false` toast on the response body. */
    skipBusinessError?: boolean
    /** Skip all error toasts (caller will surface the error itself). */
    skipErrorHandler?: boolean
    /** Skip GET request dedup for this call. */
    disableDuplicate?: boolean
    /** Skip the 401 → refresh → replay flow (used by the refresh call itself). */
    skipAuthRefresh?: boolean
    /** Marker set on a request that is already a 401 retry, to avoid loops. */
    authRetry?: boolean
    /** Apply an auth-rotation bundle found in `response.data.data`. */
    acceptAuthRotation?: boolean
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
// GET dedup — collapse identical in-flight GETs onto one promise per
// (session SID, url, params). Disabled per-request via `disableDuplicate`.
// ---------------------------------------------------------------------------

const inFlightGet = new Map<string, Promise<unknown>>()
const originalGet = apiClient.get.bind(apiClient)

apiClient.get = ((url: string, config: ApiRequestConfig = {}) => {
  if (config.disableDuplicate) return originalGet(url, config)

  const params = config.params ? JSON.stringify(config.params) : '{}'
  const sessionSID = getAuthSession()?.sid || 'anonymous'
  const key = `${sessionSID}:${url}?${params}`
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
// Response + error interceptors — business error toasts, 401 refresh+replay,
// and a catch-all error toast.
// ---------------------------------------------------------------------------

function redirectToSignIn(): void {
  if (
    typeof window !== 'undefined' &&
    window.location.pathname !== '/sign-in'
  ) {
    window.location.replace('/sign-in')
  }
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
    if (response.config.acceptAuthRotation && response.data?.success === true) {
      applyAuthRotation(response.data.data)
    }

    if (
      !response.config.skipBusinessError &&
      typeof response.data?.success === 'boolean' &&
      !response.data.success
    ) {
      const message = resolveResponseMessage(response.data) || i18n.t('common.requestFailed')
      toast.error(message)
    }
    return response
  },
  async (error) => {
    const config = error?.config as ApiRequestConfig | undefined
    const skipErrorHandler = config?.skipErrorHandler
    const status = error?.response?.status

    if (status === 401) {
      if (config && !config.skipAuthRefresh && !config.authRetry) {
        config.authRetry = true
        const outcome = await refreshAuthentication()
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

        // anonymous / out_of_sync / transient_error — treat as signed out.
        if (!skipErrorHandler) toast.error(i18n.t('common.sessionExpired'))
        redirectToSignIn()
      } else if (config?.authRetry) {
        clearAuthentication(false)
        if (!skipErrorHandler) toast.error(i18n.t('common.sessionExpired'))
        redirectToSignIn()
      } else if (!skipErrorHandler) {
        toast.error(i18n.t('common.sessionExpired'))
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
// Auth + timeout + 401 handling mirror the legacy `fetchAuthenticatedResponse`
// contract so the streaming method bodies in api.ts stay byte-for-byte
// faithful. 401/403 here clear the session and reload the page (legacy
// behaviour); the axios path above does refresh+replay instead.
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
    if (
      typeof window !== 'undefined' &&
      typeof window.location?.reload === 'function'
    ) {
      window.location.reload()
    }
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
      clearAuthSession()
      if (
        typeof window !== 'undefined' &&
        typeof window.location?.reload === 'function'
      ) {
        window.location.reload()
      }
      throw new Error('Session expired')
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
