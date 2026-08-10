// metapi-go/features/models — barrel re-exports.
//
// Page component is the primary surface; the rest is exported for the future
// `/models` route file (validateSearch schema + types) and for cross-feature
// reuse (the model-tester imports `useModels` to populate its model picker).

export { ModelsPage } from './components/models-page'
export { ModelDetailSheet } from './components/model-detail-sheet'
export { useModelsColumns } from './components/models-columns'
export {
  buildBrandFilterOptions,
  buildCapabilityFilterOptions,
} from './components/models-columns'

export { useModels, useModelCapabilities, collectCapabilityFacets } from './api'
export {
  modelsSearchSchema,
  SORTING_ITEM_SCHEMA,
  PAGINATION_SCHEMA,
  type ModelsSearch,
} from './lib/models-schema'

export {
  modelsKeys,
  type ModelRow,
  type ModelTokenInfo,
  type ModelGroupPricing,
  type ModelPricingSource,
  type ModelAccountInfo,
  type ModelsMarketplaceResponse,
  type ModelCapabilitySummary,
} from './types'
