// metapi-go — application entry (main.tsx).
// Orchestrates: QueryClient (retry/401/403/500/staleTime) → RouterProvider
// (TanStack Router, routeTree.gen.ts) → 3-layer Provider stack
// (Theme → Direction → ThemeCustomization).
//
// Adapted from newapi web/src/main.tsx. Dropped system-branding bootstrap
// (status/favicon) and legacy-route resolver — phase 2 will wire those once
// the status hook + legacy-route map land.

import {
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import { AxiosError } from 'axios'
import i18next from 'i18next'
import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'
import { toast } from 'sonner'

import { DirectionProvider } from '@/context/direction-provider'
import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'

// i18next side-effect init (config.ts calls i18n.init)
import './i18n/config'
// Generated route tree (TanStack Router plugin overwrites on dev/build)
import { routeTree } from './routeTree.gen'
// Global styles (Tailwind 4 entry + theme tokens)
import './styles/index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        // Dev: never retry (fail fast for debugging).
        if (import.meta.env.DEV) return false
        // Prod: cap at 3 retries.
        if (failureCount > 3) return false
        // Never retry auth errors (401/403) — they won't recover.
        return !(
          error instanceof AxiosError &&
          [401, 403].includes(error.response?.status ?? 0)
        )
      },
      // Keep focused tabs from silently re-running heavy pages.
      refetchOnWindowFocus: false,
      staleTime: 10 * 1000, // 10s
    },
    mutations: {
      // Skeleton: no global mutation error handler — features own their error
      // display. TODO(phase2): wire @/lib/handle-server-error once it lands.
    },
  },
  queryCache: new QueryCache({
    onError: (error) => {
      if (error instanceof AxiosError && error.response?.status === 500) {
        toast.error(i18next.t('errors.internalServerError'))
        // TODO(phase2): router.navigate({ to: '/500' }) once error routes land
      }
    },
  }),
})

const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 0,
})

// Register the router instance for type safety.
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
  interface StaticDataRouteOption {
    title?: string
  }
}

const rootElement = document.querySelector<HTMLElement>('#root')
if (!rootElement) {
  throw new Error('Root element not found')
}

if (!rootElement.innerHTML) {
  const root = ReactDOM.createRoot(rootElement)
  root.render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <DirectionProvider>
            <ThemeCustomizationProvider>
              <RouterProvider router={router} />
            </ThemeCustomizationProvider>
          </DirectionProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </StrictMode>
  )
}
