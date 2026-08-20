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
  useRouterState,
} from '@tanstack/react-router'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Toaster } from '@/components/ui/sonner'
import { bootstrapAuthentication } from '@/lib/auth-session'
import {
  resolveDocumentTitleKeys,
  type RouteTitleSpec,
} from '@/lib/helpers/document-title'
import { metapiIdentity } from '@/lib/identity-branding'
import { useAuthStore } from '@/stores/auth-store'

let authBootstrapped = false

function isDevtoolsEnabled() {
  if (import.meta.env.MODE !== 'development') return false
  try {
    return window.localStorage.getItem('metapi-devtools') === '1'
  } catch {
    return false
  }
}

/**
 * Keep `document.title` in sync with the current route: the deepest match's
 * `staticData.title` holds an i18n key (or a resolver over the route params
 * for `$section` / `$subarea/$section` routes); keys are translated, joined
 * with " · " and suffixed with the product name
 * (`General · Site & Branding · Metapi`). Routes without a title fall back
 * to the bare product name. Re-runs on navigation and on language change.
 */
function useDocumentTitle() {
  const { t, i18n } = useTranslation()
  const titleKeys = useRouterState({
    select: (state) => {
      const lastMatch = state.matches.at(-1)
      const title = lastMatch?.staticData?.title as
        | RouteTitleSpec
        | undefined
      return resolveDocumentTitleKeys(
        title,
        (lastMatch?.params ?? {}) as Record<string, string>
      )
    },
  })

  useEffect(() => {
    const labels = titleKeys.map((key) => t(key)).filter(Boolean)
    document.title =
      labels.length > 0
        ? `${labels.join(' · ')} · ${metapiIdentity.name}`
        : metapiIdentity.name
  }, [titleKeys, t, i18n.language])
}

function RootComponent() {
  const queryClient = useQueryClient()
  const showDevtools = isDevtoolsEnabled()
  useDocumentTitle()

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
      <Toaster closeButton position='top-center' />
      {showDevtools && (
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
    try {
      const outcome = await bootstrapAuthentication()
      if (outcome.kind === 'authenticated' && outcome.bundle) {
        const { auth } = useAuthStore.getState()
        if (!auth.accessToken) {
          auth.setBundle(outcome.bundle)
        }
      }
    } catch {
      // localStorage can throw (SecurityError) in sandboxed/blocked-storage
      // contexts; treat as anonymous rather than crashing the boot.
    }
    useAuthStore.getState().auth.setBootstrapState('complete')
    authBootstrapped = true
  },
  component: RootComponent,
})
