import axios, { type AxiosRequestConfig } from 'axios'

import i18n from '@/i18n/config'
import { clearAuthentication } from '@/lib/auth-session'
import { toast } from '@/lib/toast'

import {
  resolveErrorMessage,
  resolveResponseMessage,
} from './http-error-message'
import { redirectToSignIn, isReauthRequiredBody } from './http-session'

export {
  resolveResponseMessage,
  resolveResponseErrorCode,
  extractApiErrorBody,
} from './http-error-message'
export { extractResponseErrorMessage } from './http-fetch-response'
export { fetchAuthenticatedResponse } from './http-fetch'
export { isReauthRequired } from './http-session'

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

export type FetchAuthenticatedOptions = RequestInit & {
  timeoutMs?: number
}

/**
 * Unified non-2xx error body sent by the Go backend (#1065; contract:
 * docs/api.md "Error (non-2xx)"): `{ error, errorCode?, request_id? }`.
 * A few legacy TS-era endpoints still answer `{ message }`, so both display
 * keys are read by `resolveResponseMessage`.
 *
 * `errorCode` is optional and additive: only endpoints with a registered
 * code send it (registry: docs/api.md "errorCode convention and registry";
 * backend constants: handler/admin/error_codes.go). When a code exists for
 * a failure class, clients must branch on it — never on `error` text.
 */
export type ApiErrorBody = {
  error?: string
  errorCode?: string
  request_id?: string
  /** Legacy TS-era error shape still answered by a few endpoints. */
  message?: string
}

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

/**
 * True when a 403 body carries the backend's "Invalid token" semantics
 * (rotated master token presented via the legacy Bearer track) rather than
 * an IP-allowlist rejection.
 */
function isInvalidTokenMessage(message: string | undefined): boolean {
  return !!message && message.toLowerCase().includes('invalid token')
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
      // Stable id keyed on the message: when the backend is down, N parallel
      // queries each fail with the identical message and collapse into one
      // toast instead of stacking N duplicates.
      toast.error(message, { id: `api-error:${message}` })
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
      const message = resolveErrorMessage(data, error, status)
      // Same message-keyed dedupe as the business-error path.
      toast.error(message, { id: `api-error:${message}` })
    }
    throw error
  }
)
