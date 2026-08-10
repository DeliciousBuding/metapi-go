// metapi-go/routes — settings pathless layout.
//
// Parent layout for `/settings/*`. Renders the shared Outlet; each subarea's
// sidebar + section content is rendered by `SettingsPage` (in the sibling
// `$subarea.$section` route). Mirrors the dashboard feature's
// `dashboard/route.tsx`.

import { createFileRoute, Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/settings')({
  component: SettingsLayout,
})

function SettingsLayout() {
  return <Outlet />
}
