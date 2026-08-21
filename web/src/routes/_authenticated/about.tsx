// metapi-go/routes — about (project + build info page).
//
// No `validateSearch` and no `loader`: the page shell renders the curated
// project metadata immediately and `useAboutInfo()` fetches the binary's build
// provenance (`GET /api/about`) into the Build Info card, which owns its own
// skeleton/error states. The component is declared directly; the router
// plugin's `autoCodeSplitting` splits it in production.

import { createFileRoute } from '@tanstack/react-router'

import { AboutPage } from '@/features/about/components/about-page'

export const Route = createFileRoute('/_authenticated/about')({
  staticData: { title: 'about.title' },
  component: AboutPage,
})
