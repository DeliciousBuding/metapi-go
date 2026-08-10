// metapi-go/routes — settings section dispatcher.
//
// Leaf route for `/settings/$subarea/$section` (e.g. `/settings/general/site`).
// `beforeLoad` validates both the subarea id (against the settings manifest)
// and the section id (against the subarea's `sectionIds` via `isValidSection`),
// redirecting to the default section on mismatch. The component renders
// `SettingsPage` with the resolved subarea config + the active section; the
// in-page `SettingsSidebar` uses `<Link>` for section navigation, so no
// `onSectionChange` callback is needed.

import { createFileRoute, redirect } from '@tanstack/react-router'

import {
  SettingsPage,
  getSettingsSubarea,
  isValidSection,
  resolveDefaultSection,
} from '@/features/settings'

export const Route = createFileRoute(
  '/_authenticated/settings/$subarea/$section',
)({
  beforeLoad: ({ params }) => {
    const subareaConfig = getSettingsSubarea(params.subarea)
    if (!subareaConfig) {
      throw redirect({
        to: '/settings/$subarea',
        params: { subarea: 'general' },
      })
    }
    if (!isValidSection(params.subarea, params.section)) {
      const defaultSection =
        resolveDefaultSection(params.subarea) ?? subareaConfig.defaultSection
      throw redirect({
        to: '/settings/$subarea/$section',
        params: { subarea: params.subarea, section: defaultSection },
      })
    }
  },
  component: SettingsSectionRoute,
})

function SettingsSectionRoute() {
  const { subarea, section } = Route.useParams()
  const subareaConfig = getSettingsSubarea(subarea)
  // beforeLoad guarantees the subarea is valid; guard for TS narrowing only.
  if (!subareaConfig) return null
  return <SettingsPage subarea={subareaConfig} activeSection={section} />
}
