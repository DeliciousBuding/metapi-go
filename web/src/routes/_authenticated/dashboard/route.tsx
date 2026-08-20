// metapi-go/routes — dashboard pathless layout.
//
// Parent layout for `/dashboard/*`. Renders the shared Outlet; the 4-section
// Tabs + section content live in `DashboardPage` (rendered by the sibling
// `$section` route). Keeping this layout thin lets future shared dashboard
// chrome (announcement banner, time-range filter) land here without touching
// the section route. Mirrors the settings feature's `settings/route.tsx`.

import { createFileRoute, Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/dashboard')({
  staticData: { title: 'dashboard.page.title' },
  component: DashboardLayout,
})

function DashboardLayout() {
  return <Outlet />
}
