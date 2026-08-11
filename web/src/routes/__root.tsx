// metapi-go/routes — root route.
// beforeLoad bootstraps auth (reads localStorage, hydrates the Zustand store
// once). RootComponent renders the Outlet + global Toaster + devtools. The
// 3 context providers (Theme/Direction/ThemeCustomization) wrap the
// RouterProvider in main.tsx, so they are always mounted.

import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import {
  createRootRouteWithContext,
  Outlet,
} from '@tanstack/react-router'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import { useEffect } from 'react'

import { Toaster } from '@/components/ui/sonner'
import { bootstrapAuthentication } from '@/lib/auth-session'
import { useAuthStore } from '@/stores/auth-store'

let authBootstrapped = false

function RootComponent() {
  const queryClient = useQueryClient()

  // Clear the query cache when the auth session changes (login/logout/tab
  // sync) so stale user-scoped data never leaks across sessions.
  useEffect(
    () =>
      useAuthStore.subscribe((state, previousState) => {
        if (state.auth.session?.sid !== previousState.auth.session?.sid) {
          queryClient.clear()
        }
      }),
    [queryClient]
  )

  return (
    <>
      <Outlet />
      <Toaster closeButton duration={5000} position='top-center' richColors />
      {import.meta.env.MODE === 'development' && (
        <>
          <ReactQueryDevtools buttonPosition='bottom-left' />
          <TanStackRouterDevtools position='bottom-right' />
        </>
      )}
    </>
  )
}

export const Route = createRootRouteWithContext<{
  queryClient: QueryClient
}>()({
  beforeLoad: async () => {
    if (authBootstrapped) return
    const outcome = await bootstrapAuthentication()
    if (outcome.kind === 'authenticated' && outcome.bundle) {
      const { auth } = useAuthStore.getState()
      if (!auth.accessToken) {
        auth.setBundle(outcome.bundle)
      }
    }
    useAuthStore.getState().auth.setBootstrapState('complete')
    authBootstrapped = true
  },
  component: RootComponent,
})
