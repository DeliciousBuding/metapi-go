// metapi-go/routes — about (static info page).
//
// No `validateSearch` and no `loader`: the about page renders build-time
// constants resolved synchronously by `useAboutInfo()` (no network round
// trip), so there is nothing to prefetch. The component is declared directly;
// the router plugin's `autoCodeSplitting` splits it in production.

import { createFileRoute } from '@tanstack/react-router'

import { AboutPage } from '@/features/about/components/about-page'

export const Route = createFileRoute('/_authenticated/about')({
  staticData: { title: 'about.title' },
  component: AboutPage,
})
