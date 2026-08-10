// metapi-go/routes — authenticated index (→ dashboard).
//
// `/` (the authenticated root) redirects to the dashboard's default section
// (`/dashboard/overview`), replacing the phase-1 stub. The dashboard
// `$section` route validates the section param, so an unknown section still
// falls back to the default.

import { createFileRoute, redirect } from '@tanstack/react-router'

import { DASHBOARD_DEFAULT_SECTION } from '@/features/dashboard'

export const Route = createFileRoute('/_authenticated/')({
  beforeLoad: () => {
    throw redirect({
      to: '/dashboard/$section',
      params: { section: DASHBOARD_DEFAULT_SECTION },
    })
  },
})
