import { request } from './transport'
import type {
  CatalogSource,
  CatalogSourceInput,
  CatalogSyncStatus,
} from './types'

/**
 * Model-catalog data source registry + sync control
 * (GET/POST/PUT/DELETE /api/models/catalog-sources,
 *  POST /api/models/catalog-sync, GET /api/models/catalog-sync,
 *  PUT /api/models/catalog-sync/config).
 */

export type { CatalogSource, CatalogSourceInput, CatalogSyncStatus }

export const catalogSourcesApi = {
  getCatalogSync: () =>
    request<CatalogSyncStatus>('/api/models/catalog-sync', {
      timeoutMs: 20_000,
    }),
  syncCatalog: (sourceId?: number) =>
    request<CatalogSyncStatus>('/api/models/catalog-sync', {
      method: 'POST',
      body: JSON.stringify(sourceId ? { sourceId } : {}),
      timeoutMs: 120_000,
    }),
  updateCatalogSyncConfig: (autoSync: boolean) =>
    request<CatalogSyncStatus>('/api/models/catalog-sync/config', {
      method: 'PUT',
      body: JSON.stringify({ autoSync }),
    }),
  createCatalogSource: (input: CatalogSourceInput) =>
    request<{ source: CatalogSource }>('/api/models/catalog-sources', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  updateCatalogSource: (id: number, input: CatalogSourceInput) =>
    request<{ source: CatalogSource }>(`/api/models/catalog-sources/${id}`, {
      method: 'PUT',
      body: JSON.stringify(input),
    }),
  deleteCatalogSource: (id: number) =>
    request<{ deleted: number }>(`/api/models/catalog-sources/${id}`, {
      method: 'DELETE',
    }),
}
