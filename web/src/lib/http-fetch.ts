import i18n from '@/i18n/config'
import { clearAuthSession } from '@/lib/auth-session'

import type { FetchAuthenticatedOptions } from './http-client'
import { classify403 } from './http-fetch-response'
import { redirectToSignIn, ReauthRequiredError } from './http-session'

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
