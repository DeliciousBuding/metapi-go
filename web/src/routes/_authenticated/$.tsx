// metapi-go/routes — authenticated catch-all (404 inside the app shell).
//
// Without this, an unknown path under `/_authenticated` (e.g. `/nope`) matches
// the pathless `_authenticated` route but no child, so TanStack Router falls
// back to the router-level `defaultNotFoundComponent` and renders the 404
// *outside* the sidebar/header shell. This catch-all matches any unmatched
// path, so the 404 renders inside `AuthenticatedLayout`'s SidebarInset — the
// shell (and the auth guard above) stay intact.
//
// `NotFoundPage` uses `min-h-full` so it fills the SidebarInset container
// rather than overflowing it.

import { createFileRoute } from '@tanstack/react-router'

import { NotFoundPage } from '@/components/layout/not-found-page'

export const Route = createFileRoute('/_authenticated/$')({
  component: NotFoundPage,
})
