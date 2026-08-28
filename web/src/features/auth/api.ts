// metapi-go/features/auth — login API + useLogin hook.
//
// #1034 session model: the operator pastes the admin token once; the backend
// validates it (POST /api/auth/login) and mints a server-side session as an
// HttpOnly, SameSite=Strict cookie. The token is never stored client-side —
// localStorage only mirrors the session expiry for cold-load guards.

import { useMutation } from '@tanstack/react-query'
import axios from 'axios'

import { sessionApi } from '@/lib/api/session'
import { persistSessionMeta } from '@/lib/auth-session'
import { useAuthStore } from '@/stores/auth-store'

import type { LoginError, LoginPayload } from './types'

/**
 * Map a failed login response to an i18n message key.
 * `status === 0` covers axios network errors (no response at all — server
 * down or unreachable), which previously fell through to the generic
 * "login failed" copy.
 */
export function resolveLoginErrorMessageKey(
  status: number,
  reason: string
): string {
  const normalized = (reason || '').trim().toLowerCase()
  if (status === 0) {
    return 'errors.login.serverUnreachable'
  }
  if (status === 403 && normalized.includes('ip not allowed')) {
    return 'errors.login.ipNotAllowed'
  }
  if (
    status === 401 ||
    (status === 403 && normalized.includes('invalid token'))
  ) {
    return 'errors.login.invalidToken'
  }
  if (status >= 500) {
    return 'errors.login.serverError'
  }
  return 'errors.login.failed'
}

/**
 * Login mutation. Exchanges the master token for a session cookie, mirrors
 * the (non-sensitive) expiry into localStorage, and hydrates the auth-store.
 * Throws a typed LoginError (with an i18next messageKey) on failure so the
 * form can display it via setError + FormMessage.
 */
export function useLogin() {
  const setSession = useAuthStore((state) => state.auth.setSession)

  return useMutation<number, LoginError, LoginPayload>({
    mutationFn: async ({ token }) => {
      try {
        const response = await sessionApi.login(token.trim())
        const expiresAtMs = Date.parse(response.expiresAt)
        if (!response.authenticated || !Number.isFinite(expiresAtMs)) {
          throw {
            messageKey: 'errors.login.failed',
            status: 0,
          } as LoginError
        }
        persistSessionMeta(expiresAtMs, localStorage)
        setSession(expiresAtMs)
        return expiresAtMs
      } catch (error: unknown) {
        if (error && typeof error === 'object' && 'messageKey' in error) {
          throw error as LoginError
        }
        if (axios.isAxiosError(error)) {
          const status = error.response?.status ?? 0
          const data = error.response?.data
          const reason =
            typeof data === 'object' && data !== null
              ? String(
                  (data as { message?: unknown; error?: unknown }).message ??
                    (data as { error?: unknown }).error ??
                    ''
                )
              : ''
          throw {
            messageKey: resolveLoginErrorMessageKey(status, reason),
            status,
          } as LoginError
        }
        throw {
          messageKey: 'errors.login.serverUnreachable',
          status: 0,
        } as LoginError
      }
    },
  })
}
