// metapi-go/routes — settings subarea pathless layout.
//
// TanStack Router's file-router treats `$subarea.tsx` as the parent layout of
// `$subarea.index.tsx` (bare subarea) and `$subarea.$section.tsx` (section).
// `beforeLoad` validates the subarea id once for the whole subtree
// (unknown ids fall back to `general`); the layout renders the shared Outlet
// so the child route (index redirect or section page) controls the surface.
//
// Note: subarea + section are path params, so validation lives in `beforeLoad`
// (not `validateSearch`, which is for search params).

import { createFileRoute, Outlet, redirect } from '@tanstack/react-router'

import { getSettingsSubarea } from '@/features/settings'

export const Route = createFileRoute('/_authenticated/settings/$subarea')({
  beforeLoad: ({ params }) => {
    if (!getSettingsSubarea(params.subarea)) {
      throw redirect({
        to: '/settings/$subarea',
        params: { subarea: 'general' },
      })
    }
  },
  component: SettingsSubareaLayout,
})

function SettingsSubareaLayout() {
  return <Outlet />
}
