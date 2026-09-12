// metapi-go/features/models — public barrel.
//
// Cross-feature consumers import from here and not from a subdirectory: what
// is exported below is frozen against their call sites, and what is not
// exported is free to move.
//
// The `/models` route reads the validateSearch schema from here and loads
// `ModelsPage` directly; the model-tester populates its picker with `useModels`;
// the token-routes detail sheet reads the price-compare contract below.
// `ModelsPage` is deliberately not exported here, so those consumers do not
// pull the page into their chunk.

export { fetchModelsPage, modelsPageQueryKey, useModels } from './api'
export { modelsSearchSchema } from './lib/models-schema'

export { modelsKeys } from './types'

// --- price-compare sub-module: the cross-site price contract and its grade
// badge, read by the token-routes detail sheet as well as the models page ---
export { priceCompareQueryOptions } from './price-compare/api'
export { PriceGradeBadge } from './price-compare/components/price-grade-badge'
export { normalizePriceGrade } from './price-compare/types'
export type { PriceCompareItem, PriceGrade } from './price-compare/types'
