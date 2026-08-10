// metapi-go/routes — dashboard section dispatcher.
//
// Leaf route for `/dashboard/$section`. `beforeLoad` validates the section id
// against the dashboard manifest (redirects to the default section on
// mismatch), mirroring newapi's `dashboard/$section.tsx`. The component
// renders `DashboardPage` with the active section + a navigate-backed
// `onSectionChange` so the Tabs switch routes (the dashboard feature stays
// presentational; the route layer owns URL state).
//
// Note: `validateSearch` is intentionally absent — the dashboard has no URL
// search state today. Section validation lives in `beforeLoad` because
// `$section` is a path param, not a search param.

import {
  createFileRoute,
  redirect,
  useNavigate,
} from '@tanstack/react-router'

import {
  DASHBOARD_DEFAULT_SECTION,
  DASHBOARD_SECTION_IDS,
  DashboardPage,
} from '@/features/dashboard'

export const Route = createFileRoute('/_authenticated/dashboard/$section')({
  beforeLoad: ({ params }) => {
    const knownSections = DASHBOARD_SECTION_IDS as readonly string[]
    if (!knownSections.includes(params.section)) {
      throw redirect({
        to: '/dashboard/$section',
        params: { section: DASHBOARD_DEFAULT_SECTION },
      })
    }
  },
  component: DashboardSectionRoute,
})

function DashboardSectionRoute() {
  const { section } = Route.useParams()
  const navigate = useNavigate()

  return (
    <DashboardPage
      activeSection={section}
      onSectionChange={(sectionId) =>
        navigate({
          to: '/dashboard/$section',
          params: { section: sectionId },
        })
      }
    />
  )
}
