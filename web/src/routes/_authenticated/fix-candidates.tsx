// metapi-go/routes — redirect fix-candidates review + one-click apply.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/fix-candidates')({
  component: lazyRouteComponent(
    () =>
      import('@/features/models/fix-candidates/components/fix-candidates-page'),
    'FixCandidatesPage'
  ),
})
