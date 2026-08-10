// metapi-go/routes — settings index (→ default subarea).
//
// Bare `/settings` redirects to the first subarea (`general`). The main
// sidebar links to `/settings`, so this index closes the loop. From there
// the `$subarea` route redirects to the subarea's default section.

import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/settings/')({
  beforeLoad: () => {
    throw redirect({
      to: '/settings/$subarea',
      params: { subarea: 'general' },
    })
  },
})
