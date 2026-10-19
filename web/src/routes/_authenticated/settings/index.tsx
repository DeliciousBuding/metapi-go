// metapi-go/routes — settings index.
//
// Bare `/settings` is a bookmark/legacy entry point, not a destination: the
// settings sidebar already lists every subarea with the same titles and
// descriptions, so a landing grid of those five links only duplicated it and
// left most of the page empty. Send the operator straight to the first
// subarea's default section and let the sidebar carry orientation.

import { createFileRoute, redirect } from '@tanstack/react-router'

import { getSettingsSubareas } from '@/features/settings'

export const Route = createFileRoute('/_authenticated/settings/')({
  staticData: { title: 'settings.subareas.basic' },
  beforeLoad: () => {
    const first = getSettingsSubareas()[0]
    throw redirect({
      to: '/settings/$subarea/$section',
      params: { subarea: first.id, section: first.defaultSection },
      replace: true,
    })
  },
})
