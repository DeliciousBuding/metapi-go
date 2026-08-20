// metapi-go/lib — shared QueryClient + Router instances.
//
// Lives outside main.tsx so non-React call sites (e.g. sonner toast action
// callbacks, which outlive the component that fired them) can navigate
// through the SPA router instead of hard-reloading the page. main.tsx
// imports both instances for the provider stack; nothing here depends on
// main.tsx.

import { QueryCache, QueryClient } from '@tanstack/react-query'
import { createRouter } from '@tanstack/react-router'
import { AxiosError } from 'axios'
import i18next from 'i18next'

import { ErrorPage } from '@/components/layout/error-page'
import { NotFoundPage } from '@/components/layout/not-found-page'
import { RoutePending } from '@/components/layout/route-pending'
import type { RouteTitleSpec } from '@/lib/helpers/document-title'
import { toast } from '@/lib/toast'
// Generated route tree (TanStack Router plugin overwrites on dev/build)
import { routeTree } from '@/routeTree.gen'

export const queryClient = new QueryClient({
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
  },
  queryCache: new QueryCache({
    onError: (error) => {
      if (error instanceof AxiosError && error.response?.status === 500) {
        toast.error(i18next.t('errors.internalServerError'))
      }
    },
  }),
})

export const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: 'intent',
  defaultNotFoundComponent: NotFoundPage,
  defaultErrorComponent: ErrorPage,
  // The window never scrolls — the app is a single pane and the scroll
  // container is #content (AuthenticatedLayout's SidebarInset). Reset it to
  // top on navigation so a new route never inherits the previous page's
  // offset.
  scrollRestoration: true,
  scrollToTopSelectors: ['#content'],
  // Leaf routes are code-split (rsbuild.config.ts `autoCodeSplitting`), so a
  // cold navigation suspends until the chunk + loader resolve. Without a
  // pending component the router renders `null` in that window — a black flash
  // in dark mode. RoutePending keeps the content shell visible instead.
  // `pendingMs` debounces the fallback (fast loads never show it) and
  // `pendingMinMs` stops the skeleton itself from flickering on near-misses.
  defaultPendingComponent: RoutePending,
  defaultPendingMs: 200,
  defaultPendingMinMs: 300,
})

// Register the router instance for type safety.
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
  interface StaticDataRouteOption {
    /**
     * i18n key(s) for the route's `document.title` (see __root.tsx
     * useDocumentTitle). Static routes use a plain key (or key list);
     * param-driven routes (`$section`, `$subarea/$section`) use a resolver
     * over the route params.
     */
    title?: RouteTitleSpec
  }
}
