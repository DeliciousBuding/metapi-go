// metapi-go/routes — settings index (overview landing).
//
// Bare `/settings` renders the subarea overview grid instead of redirecting,
// so the full configuration scope is visible before drilling in. Each card
// links to the subarea (which redirects to its default section) and lists the
// subarea's sections for direct jumps. The main sidebar links to `/settings`,
// so this index closes the loop.

import { createFileRoute } from '@tanstack/react-router'

import { SettingsOverview } from '@/features/settings'

export const Route = createFileRoute('/_authenticated/settings/')({
  staticData: { title: 'settings.overview.title' },
  component: SettingsOverviewRoute,
})

function SettingsOverviewRoute() {
  return <SettingsOverview />
}
