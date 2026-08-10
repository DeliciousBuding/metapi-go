// metapi-go/stores — Zustand auth-store.
// Minimal in-memory auth state for the UI layer. The storage source-of-truth
// (localStorage token + expiry) lives in @/lib/auth-session; this store holds
// the React-visible view (user, token, session) and is hydrated by the login
// flow and the root-route bootstrap. Mirrors the newapi auth-store shape
// (simplified: no 2FA, no permissions, no admin-capabilities).

import { create } from 'zustand'

import type {
  AuthBootstrapState,
  AuthBundle,
  AuthUser,
  LoginSession,
} from '@/lib/auth-session'

interface AuthState {
  auth: {
    user: AuthUser | null
    accessToken: string | null
    accessExpiresAt: number | null
    session: LoginSession | null
    bootstrapState: AuthBootstrapState
    setBundle: (bundle: AuthBundle) => void
    setUser: (user: AuthUser | null) => void
    setBootstrapState: (bootstrapState: AuthBootstrapState) => void
    reset: (bootstrapState?: AuthBootstrapState) => void
  }
}

export const useAuthStore = create<AuthState>()((set) => ({
  auth: {
    user: null,
    accessToken: null,
    accessExpiresAt: null,
    session: null,
    bootstrapState: 'idle',
    setBundle: (bundle) =>
      set((state) => ({
        ...state,
        auth: {
          ...state.auth,
          user: bundle.user ?? null,
          accessToken: bundle.access_token,
          accessExpiresAt: bundle.access_expires_at,
          session: bundle.session ?? null,
          bootstrapState: 'complete',
        },
      })),
    setUser: (user) =>
      set((state) => ({ ...state, auth: { ...state.auth, user } })),
    setBootstrapState: (bootstrapState) =>
      set((state) => ({ ...state, auth: { ...state.auth, bootstrapState } })),
    reset: (bootstrapState = 'complete') =>
      set((state) => ({
        ...state,
        auth: {
          ...state.auth,
          user: null,
          accessToken: null,
          accessExpiresAt: null,
          session: null,
          bootstrapState,
        },
      })),
  },
}))
