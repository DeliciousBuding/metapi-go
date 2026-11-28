import i18n from '@/i18n/config'

import type { ApiErrorBody } from './http-client'

export function resolveResponseMessage(data: unknown): string | undefined {
  if (data && typeof data === 'object') {
    const body = data as ApiErrorBody
    if (typeof body.message === 'string' && body.message) return body.message
    if (typeof body.error === 'string' && body.error) return body.error
  }
  return undefined
}

/**
 * Machine-readable `errorCode` of a unified error body (#1065). Returns
 * undefined when the body carries no code — the field is optional and only
 * endpoints with a registered code send it, so callers must keep the
 * human-readable message as their display fallback.
 */
export function resolveResponseErrorCode(data: unknown): string | undefined {
  if (data && typeof data === 'object') {
    const errorCode = (data as ApiErrorBody).errorCode
    if (typeof errorCode === 'string' && errorCode) return errorCode
  }
  return undefined
}

/**
 * errorCode → i18n key for codes the console should render localized instead
 * of the backend's English fallback text (auth-middleware rejections are the
 * highest-frequency toasts and fired long before any user locale context
 * existed on the backend). Codes not listed here keep the raw body message —
 * the map is additive per migrated endpoint family.
 */
const ERROR_CODE_I18N_KEYS: Record<string, string> = {
  authSessionExpired: 'errors.auth.sessionExpired',
  authMissingCredential: 'errors.auth.missingCredential',
  authInvalidToken: 'errors.auth.invalidToken',
  authIpBlocked: 'errors.auth.ipBlocked',
  authReauthRequired: 'errors.auth.reauthRequired',
  accountNotFound: 'errors.api.accountNotFound',
  tokenNotFound: 'errors.api.tokenNotFound',
  routeNotFound: 'errors.api.routeNotFound',
  channelNotFound: 'errors.api.channelNotFound',
  siteNotFound: 'errors.api.siteNotFound',
  operationNotSupported: 'errors.api.operationNotSupported',
  resourceDisabled: 'errors.api.resourceDisabled',
  invalidSettingsValue: 'errors.api.invalidSettingsValue',
  operationNotImplemented: 'errors.api.operationNotImplemented',
  resourceLoadFailed: 'errors.api.resourceLoadFailed',
}

/** Localized copy for a known errorCode; undefined when unmapped. */
function resolveErrorCodeMessage(data: unknown): string | undefined {
  const code = resolveResponseErrorCode(data)
  if (!code) return undefined
  const key = ERROR_CODE_I18N_KEYS[code]
  return key ? i18n.t(key) : undefined
}

/**
 * Unified error body of an axios-shaped rejection (`error.response.data`),
 * or null when the failure carries no JSON body (network error, timeout).
 */
export function extractApiErrorBody(error: unknown): ApiErrorBody | null {
  const data = (error as { response?: { data?: unknown } } | null)?.response
    ?.data
  if (!data || typeof data !== 'object') return null
  return data as ApiErrorBody
}

/**
 * Resolve a user-facing message for a failed request. Precedence: a known
 * errorCode mapped to localized copy, then the backend body message, then a
 * status-aware 5xx message (so a bare 502/503/504 no longer leaks the raw
 * "Request failed with status code 502"), then the axios error message, then
 * a generic fallback.
 */
export function resolveErrorMessage(
  data: unknown,
  error: unknown,
  status: number | undefined
): string {
  const codeMessage = resolveErrorCodeMessage(data)
  if (codeMessage) return codeMessage
  const dataMessage = resolveResponseMessage(data)
  if (dataMessage) return dataMessage
  if (status !== undefined && status >= 500) {
    return resolveServerErrorMessage(status)
  }
  const axiosMessage = (error as { message?: unknown } | null)?.message
  return (
    (typeof axiosMessage === 'string' && axiosMessage) ||
    i18n.t('common.requestFailed')
  )
}

/** Status-specific copy for server errors with no backend body message. */
function resolveServerErrorMessage(status: number): string {
  if (status === 500) {
    return i18n.t('errors.internalServerError')
  }
  return i18n.t('errors.serverError', { status })
}
