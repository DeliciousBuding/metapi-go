// metapi-go/features/models — barrel re-exports.
//
// Page component is the primary surface; the rest is exported for the future
// `/models` route file (validateSearch schema + types) and for cross-feature
// reuse (the model-tester imports `useModels` to populate its model picker).

export { fetchModelsPage, modelsPageQueryKey, useModels, useModelsPage } from './api'
export { modelsSearchSchema } from './lib/models-schema'

export { modelsKeys } from './types'
