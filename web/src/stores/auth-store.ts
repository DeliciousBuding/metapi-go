// metapi-go/stores — Zustand auth-store.
// Minimal in-memory auth state for the UI layer (#1034 session model). The
// credential itself is an HttpOnly cookie the JS layer never touches; this
// store holds the React-visible view of the session (its sliding expiry) and
// is hydrated by the login flow and the root-route bootstrap.

import { create } from 'zustand'

import type { AuthBootstrapState } from '@/lib/auth-session'

interface AuthState {
  auth: {
    /** Sliding session expiry (ms epoch) or null when signed out. */
    sessionExpiresAt: number | null
    bootstrapState: AuthBootstrapState
    setSession: (expiresAtMs: number) => void
    setBootstrapState: (bootstrapState: AuthBootstrapState) => void
    reset: (bootstrapState?: AuthBootstrapState) => void
  }
}

export const useAuthStore = create<AuthState>()((set) => ({
  auth: {
    sessionExpiresAt: null,
    bootstrapState: 'idle',
    setSession: (expiresAtMs) =>
      set((state) => ({
        ...state,
        auth: {
          ...state.auth,
          sessionExpiresAt: expiresAtMs,
          bootstrapState: 'complete',
        },
      })),
    setBootstrapState: (bootstrapState) =>
      set((state) => ({ ...state, auth: { ...state.auth, bootstrapState } })),
    reset: (bootstrapState = 'complete') =>
      set((state) => ({
        ...state,
        auth: {
          ...state.auth,
          sessionExpiresAt: null,
          bootstrapState,
        },
      })),
  },
}))
