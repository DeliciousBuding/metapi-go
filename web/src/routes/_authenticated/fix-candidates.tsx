// metapi-go/routes — redirect fix-candidates review + one-click apply.

import { createFileRoute } from '@tanstack/react-router'

import { FixCandidatesPage } from '@/features/models/fix-candidates/components/fix-candidates-page'

export const Route = createFileRoute('/_authenticated/fix-candidates')({
  component: FixCandidatesPage,
})
