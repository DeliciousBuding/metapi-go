/**
 * metapi-go session API surface (#1034 session model).
 *
 * The admin UI authenticates with a server-side session: POST /api/auth/login
 * exchanges the master token for an HttpOnly cookie; logout revokes it; the
 * ws-ticket endpoint mints one-time tickets for the realtime ops WebSocket.
 * The master token itself is never stored client-side.
 */

import { request } from './transport'

export interface LoginResponse {
  authenticated: boolean
  /** RFC3339 UTC sliding expiry of the fresh session. */
  expiresAt: string
  ttlMinutes: number
}

export interface WsTicketResponse {
  ticket: string
  expiresInSeconds: number
}

export const sessionApi = {
  /** Exchange the master token for a server-side session cookie. */
  login: (token: string) =>
    request<LoginResponse>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ token }),
      skipAuthRetry: true,
      skipErrorHandler: true,
      skipBusinessError: true,
      disableDuplicate: true,
    }),

  /** Revoke the current session server-side and clear the cookie. */
  logout: () =>
    request<{ success: boolean }>('/api/auth/logout', {
      method: 'POST',
      skipErrorHandler: true,
    }),

  /** Bootstrap probe: is this browser authenticated? (never 401) */
  sessionStatus: () =>
    request<{ authenticated: boolean; source?: string; expiresAt?: string }>(
      '/api/auth/session',
      { skipErrorHandler: true, disableDuplicate: true }
    ),

  /** Mint a one-time 60s ticket for the realtime ops WebSocket. */
  requestWsTicket: () =>
    request<WsTicketResponse>('/api/auth/ws-ticket', {
      method: 'POST',
      skipErrorHandler: true,
    }),
}
