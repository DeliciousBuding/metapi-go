// metapi-go/routes — dashboard index (→ default section).
//
// Bare `/dashboard` redirects to the default section (`/dashboard/overview`).
// The main sidebar links to `/` (which `_authenticated/index.tsx` redirects to
// the same place); this index covers direct `/dashboard` entry too.

import { createFileRoute, redirect } from '@tanstack/react-router'

import { DASHBOARD_DEFAULT_SECTION } from '@/features/dashboard'

export const Route = createFileRoute('/_authenticated/dashboard/')({
  beforeLoad: () => {
    throw redirect({
      to: '/dashboard/$section',
      params: { section: DASHBOARD_DEFAULT_SECTION },
    })
  },
})
