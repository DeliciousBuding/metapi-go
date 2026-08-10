// metapi-go/routes — settings subarea index (bare subarea → default section).
//
// `/settings/general` (no section) redirects to the subarea's default section
// so the in-page `SettingsSidebar` active state resolves against a real
// section URL (`/settings/general/<default>`). The `$subarea` layout's
// `beforeLoad` already guarantees the subarea is valid by the time this runs.

import { createFileRoute, redirect } from '@tanstack/react-router'

import { resolveDefaultSection } from '@/features/settings'

export const Route = createFileRoute('/_authenticated/settings/$subarea/')({
  beforeLoad: ({ params }) => {
    const defaultSection = resolveDefaultSection(params.subarea) ?? 'site'
    throw redirect({
      to: '/settings/$subarea/$section',
      params: { subarea: params.subarea, section: defaultSection },
    })
  },
})
