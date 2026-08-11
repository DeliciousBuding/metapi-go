// metapi-go/routes — about (static info page).
//
// No `validateSearch` and no `loader`: the about page renders build-time
// constants resolved synchronously by `useAboutInfo()` (no network round
// trip), so there is nothing to prefetch. `lazyRouteComponent` code-splits
// the page.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/about')({
  component: lazyRouteComponent(
    () => import('@/features/about/components/about-page'),
    'AboutPage'
  ),
})
