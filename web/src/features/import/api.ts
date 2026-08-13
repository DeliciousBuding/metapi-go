// metapi-go/features/import — TanStack Query hooks for the import wizard.
//
// Detection is per-URL (matching the backend /api/sites/detect contract), and
// the final commit is a single idempotent POST /api/sites/import. Import
// invalidates the sites list + accounts snapshot so list pages refresh after
// the wizard closes.

import {
  useMutation,
  useQueryClient,
  type UseMutationOptions,
} from '@tanstack/react-query'

import { api } from '@/lib/api'
import { accountQueryKeys } from '@/features/accounts'
import { sitesKeys } from '@/features/sites'

import type {
  ImportSitesPayload,
  ImportSitesResult,
  SiteDetectResult,
} from './types'

/** Detect the platform for a single URL. Returns an empty object on 400. */
export function useDetectSite(
  options?: UseMutationOptions<SiteDetectResult, Error, string>
) {
  return useMutation<SiteDetectResult, Error, string>({
    mutationFn: async (url) => {
      try {
        const detected = (await api.detectSite(url)) as SiteDetectResult
        return detected ?? {}
      } catch {
        // Unknown / undetectable URLs surface as a client error; the wizard
        // keeps them manually specifiable instead of failing the batch.
        return {}
      }
    },
    ...options,
  })
}

/** Submit the assembled import batch and refresh the affected list queries. */
export function useImportSites(
  options?: UseMutationOptions<ImportSitesResult, Error, ImportSitesPayload>
) {
  const queryClient = useQueryClient()
  return useMutation<ImportSitesResult, Error, ImportSitesPayload>({
    mutationFn: async (payload) => {
      const result = (await api.importSites(payload)) as ImportSitesResult
      return result ?? { imported: 0, skipped: 0, failed: 0, results: [] }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: sitesKeys.list() })
      queryClient.invalidateQueries({ queryKey: accountQueryKeys.snapshot() })
    },
    ...options,
  })
}
