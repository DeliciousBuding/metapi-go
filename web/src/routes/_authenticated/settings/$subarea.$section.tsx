// metapi-go/routes — settings section dispatcher.
//
// Leaf route for `/settings/$subarea/$section` (e.g. `/settings/basic/site`).
// `beforeLoad` first resolves legacy URLs (wave 9 lane B regroup:
// `/settings/general/scheduling` → `/settings/operations/scheduling` …),
// then validates both the subarea id (against the settings manifest) and the
// section id (against the subarea's `sectionIds` via `isValidSection`),
// redirecting to the default section on mismatch. The component renders
// `SettingsPage` with the resolved subarea config + the active section.

import { createFileRoute, redirect } from '@tanstack/react-router'

import {
  SettingsPage,
  getSettingsSubarea,
  isValidSection,
  resolveDefaultSection,
} from '@/features/settings'
import { resolveLegacySectionRedirect } from '@/features/settings/lib/legacy-redirects'

export const Route = createFileRoute(
  '/_authenticated/settings/$subarea/$section'
)({
  // Document title combines the subarea and section labels
  // (`Basic · Site & Branding · Metapi`), resolved from the settings
  // registry so the route file carries no hard-coded key mapping.
  staticData: {
    title: ({ subarea, section }) => {
      const subareaConfig = getSettingsSubarea(subarea)
      if (!subareaConfig) return undefined
      return [subareaConfig.title, subareaConfig.getSectionMeta(section).title]
    },
  },
  beforeLoad: ({ params }) => {
    // Downstream keys were promoted to a first-class left-nav route
    // (/downstream-keys). Redirect any stale `/settings/downstream/keys`
    // bookmark / deep link so it lands on the new home instead of bouncing
    // to the subarea's default section.
    if (params.subarea === 'downstream' && params.section === 'keys') {
      throw redirect({ to: '/downstream-keys' })
    }
    const legacy = resolveLegacySectionRedirect(params.subarea, params.section)
    if (legacy) {
      throw redirect({
        to: '/settings/$subarea/$section',
        params: { subarea: legacy[0], section: legacy[1] },
      })
    }
    const subareaConfig = getSettingsSubarea(params.subarea)
    if (!subareaConfig) {
      throw redirect({
        to: '/settings/$subarea',
        params: { subarea: 'basic' },
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
