// metapi-go/routes — standalone cross-site price comparison page.
// The page owns its model-filter state (debounced server-side `model` param)
// and reads the sorted candidate list directly from usePriceCompare.
//
// `validateSearch` syncs the model filter to the URL so a refresh or shared
// link restores the filtered view (W19-T1 P2-l). The router JSON-parses search
// values, so a stale `?model=123` arrives as a number and must not throw.

import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { PriceComparePage } from '@/features/models/price-compare/components/price-compare-page'
import { stringSearchParam } from '@/lib/helpers/searchParams'

const priceCompareSearchSchema = z.object({
  model: stringSearchParam,
})

export const Route = createFileRoute('/_authenticated/price-compare')({
  validateSearch: priceCompareSearchSchema,
  staticData: { title: 'priceCompare.page.title' },
  component: PriceComparePage,
})
