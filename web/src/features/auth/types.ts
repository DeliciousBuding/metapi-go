// metapi-go/features/auth — types.
// Re-exports the auth-bundle shape from the lib layer (single SSOT) and
// defines feature-local payloads/errors for the login flow.

export type {
  AuthBundle,
} from '@/lib/auth-session'

/**
 * Login form payload. metapi-go uses token-based admin auth: the operator
 * pastes an admin token which is validated via GET /api/settings/auth/info
 * with `Authorization: Bearer <token>`. There is no username/password
 * endpoint (seamless-migration hard constraint, plan §3.1).
 */
export interface LoginPayload {
  token: string
}

/**
 * Error thrown by the login mutation when token validation fails. The
 * `messageKey` is an i18next key resolved by the form's FormMessage.
 */
export interface LoginError {
  messageKey: string
  status: number
}

