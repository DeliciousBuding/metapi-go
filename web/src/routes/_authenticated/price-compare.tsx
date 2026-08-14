// metapi-go/routes — standalone cross-site price comparison page.
// The page owns its model-filter state (debounced server-side `model` param)
// and reads the sorted candidate list directly from usePriceCompare.

import { createFileRoute } from '@tanstack/react-router'

import { PriceComparePage } from '@/features/models/price-compare/components/price-compare-page'

export const Route = createFileRoute('/_authenticated/price-compare')({
  component: PriceComparePage,
})
