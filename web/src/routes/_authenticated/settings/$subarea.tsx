// metapi-go/routes — settings subarea pathless layout.
//
// TanStack Router's file-router treats `$subarea.tsx` as the parent layout of
// `$subarea.index.tsx` (bare subarea) and `$subarea.$section.tsx` (section).
// `beforeLoad` validates the subarea id once for the whole subtree; legacy
// subarea ids (general/models/system-info, wave 9 lane B regroup) fall
// through to the child routes, which know the `$section` param and map the
// whole old URL to its new home. Unknown ids fall back to `basic`; the layout
// renders the shared Outlet so the child route (index redirect or section
// page) controls the surface.
//
// Note: subarea + section are path params, so validation lives in `beforeLoad`
// (not `validateSearch`, which is for search params).

import { createFileRoute, Outlet, redirect } from '@tanstack/react-router'

import { getSettingsSubarea, resolveDefaultSection } from '@/features/settings'
import { isLegacySubarea } from '@/features/settings/lib/legacy-redirects'

export const Route = createFileRoute('/_authenticated/settings/$subarea')({
  // Document title follows the active subarea (the child routes refine it
  // with the section title; see `$subarea.$section.tsx`).
  staticData: {
    title: ({ subarea }) => getSettingsSubarea(subarea)?.title,
  },
  beforeLoad: ({ params }) => {
    // Legacy subareas are handled by the child routes (they carry the
    // `$section` param needed to map the exact old URL).
    if (isLegacySubarea(params.subarea)) return
    if (!getSettingsSubarea(params.subarea)) {
      // Redirect straight to the default section so an unknown subarea doesn't
      // bounce through the bare `/settings/basic` → section redirect.
      throw redirect({
        to: '/settings/$subarea/$section',
        params: {
          subarea: 'basic',
          section: resolveDefaultSection('basic') ?? 'site',
        },
      })
    }
  },
  component: SettingsSubareaLayout,
})

function SettingsSubareaLayout() {
  return <Outlet />
}
