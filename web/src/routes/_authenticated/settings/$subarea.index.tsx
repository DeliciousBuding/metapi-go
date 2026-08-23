// metapi-go/routes — settings subarea index (bare subarea → default section).
//
// `/settings/basic` (no section) redirects to the subarea's default section
// so the sidebar tree's active state resolves against a real section URL
// (`/settings/basic/site`). Legacy bare subarea URLs (`/settings/general`,
// `/settings/models`, `/settings/system-info`) map through the legacy table
// to their new subarea's default section. The `$subarea` layout's
// `beforeLoad` already guarantees the subarea is valid (or legacy) by the
// time this runs.

import { createFileRoute, redirect } from '@tanstack/react-router'

import { getSettingsSubarea, resolveDefaultSection } from '@/features/settings'
import { resolveLegacySubareaRedirect } from '@/features/settings/lib/legacy-redirects'

export const Route = createFileRoute('/_authenticated/settings/$subarea/')({
  staticData: {
    title: ({ subarea }) => getSettingsSubarea(subarea)?.title,
  },
  beforeLoad: ({ params }) => {
    const legacyTarget = resolveLegacySubareaRedirect(params.subarea)
    const target = legacyTarget ?? params.subarea
    const defaultSection = resolveDefaultSection(target) ?? 'site'
    throw redirect({
      to: '/settings/$subarea/$section',
      params: { subarea: target, section: defaultSection },
    })
  },
})
