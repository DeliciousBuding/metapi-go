// metapi-go/routes — observability workspace.
//
// Single route for the Observability hub. The active section lives in the
// `section` search param (validated by `observabilitySearchSchema`) so a
// deep link restores the exact section. The component renders
// `ObservabilityPage` with a navigate-backed `onSectionChange`, mirroring the
// dashboard `$section` dispatcher.

import { createFileRoute, useNavigate } from '@tanstack/react-router'

import {
  ObservabilityPage,
  observabilitySearchSchema,
  type ObservabilitySectionId,
} from '@/features/observability'

export const Route = createFileRoute('/_authenticated/observability')({
  validateSearch: observabilitySearchSchema,
  component: ObservabilityRoute,
})

function ObservabilityRoute() {
  const { section } = Route.useSearch()
  const navigate = useNavigate()

  return (
    <ObservabilityPage
      activeSection={section}
      onSectionChange={(sectionId: ObservabilitySectionId) =>
        navigate({
          to: '/observability',
          search: { section: sectionId },
        })
      }
    />
  )
}
