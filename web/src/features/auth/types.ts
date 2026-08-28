// metapi-go/features/auth — types.
// Login payload/error shapes for the token-exchange flow (#1034: the token
// is presented once at POST /api/auth/login and never stored client-side).

/**
 * Login form payload. metapi-go uses token-based admin auth: the operator
 * pastes an admin token which the backend exchanges for a server-side
 * session cookie. There is no username/password endpoint.
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
