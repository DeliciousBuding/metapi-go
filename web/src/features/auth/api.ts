// metapi-go/features/auth — login API + useLogin hook.
//
// metapi-go auth is token-based (plan §3.1 seamless-migration): the operator
// enters an admin token, we validate it by calling GET /api/settings/auth/info
// with `Authorization: Bearer <token>`. A 2xx response means the token is
// valid; 401/403 means it is invalid (or IP-blocked); 5xx is a server fault.
//
// The call uses skipAuthRefresh/skipErrorHandler/skipBusinessError to bypass
// the http-client's 401-refresh loop and global toast (the login form owns
// error display). On success we persist the session to localStorage (via
// setAuthBundle, byte-compatible with the legacy authSession.ts keys) and
// hydrate the Zustand auth-store.

import { useMutation } from '@tanstack/react-query'
import axios from 'axios'

import { apiClient } from '@/lib/http-client'
import {
  AUTH_SESSION_DURATION_MS,
  setAuthBundle,
} from '@/lib/auth-session'
import { useAuthStore } from '@/stores/auth-store'

import type { AuthBundle, LoginError, LoginPayload } from './types'



function resolveLoginErrorMessageKey(
  status: number,
  reason: string
): string {
  const normalized = (reason || '').trim().toLowerCase()
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

async function validateAdminToken(token: string): Promise<AuthBundle> {
  await apiClient.get('/api/settings/auth/info', {
    headers: { Authorization: `Bearer ${token}` },
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
    disableDuplicate: true,
  })

  const nowMs = Date.now()
  const bundle: AuthBundle = {
    access_token: token,
    token_type: 'Bearer',
    access_expires_at: Math.floor((nowMs + AUTH_SESSION_DURATION_MS) / 1000),
    user: null,
    session: null,
  }
  return bundle
}

/**
 * Validate an admin token against the backend and build an AuthBundle on
 * success. The backend only confirms validity (200 OK); it does not rotate
 * the token or return a user profile in this response, so the bundle carries
 * the original token + a 12h expiry + null user/session.
 */

/**
 * Map an HTTP failure status + backend reason string to an i18next key.
 * Mirrors the legacy `loginError.ts` decision tree (IP whitelist 403 /
 * invalid token 401 / 5xx / generic).
 */

/**
 * Login mutation. Validates the token, persists the session, hydrates the
 * auth-store. Throws a typed LoginError (with an i18next messageKey) on
 * failure so the form can display it via setError + FormMessage.
 */
export function useLogin() {
  const setBundle = useAuthStore((state) => state.auth.setBundle)

  return useMutation<AuthBundle, LoginError, LoginPayload>({
    mutationFn: async ({ token }) => {
      try {
        const bundle = await validateAdminToken(token)
        setAuthBundle(bundle, localStorage)
        setBundle(bundle)
        return bundle
      } catch (error: unknown) {
        if (axios.isAxiosError(error)) {
          const status = error.response?.status ?? 0
          const data = error.response?.data
          const reason =
            typeof data === 'object' && data !== null
              ? String(
                  (data as { message?: unknown; error?: unknown })
                    .message ??
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
