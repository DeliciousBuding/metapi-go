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
import axios from 'axios'

import { accountQueryKeys } from '@/features/accounts'
import { sitesKeys } from '@/features/sites'
import { api } from '@/lib/api'

import type {
  ImportSitesPayload,
  ImportSitesResult,
  SiteDetectResult,
} from './types'

/**
 * HTTP status codes that mean "the backend looked and the platform is not
 * detectable" — a clean, expected negative result. The wizard keeps these URLs
 * manually specifiable instead of treating them as failures.
 */
const UNDETECTABLE_STATUS_CODES = new Set([400, 404])

/** Detect the platform for a single URL. Returns an empty object on 400/404. */
export function useDetectSite(
  options?: UseMutationOptions<SiteDetectResult, Error, string>
) {
  return useMutation<SiteDetectResult, Error, string>({
    mutationFn: async (url) => {
      try {
        const detected = (await api.detectSite(url)) as SiteDetectResult
        return detected ?? {}
      } catch (error: unknown) {
        // Only 400/404 are clean "not detectable" responses — swallow them so
        // the wizard renders the URL as manually specifiable. Transport
        // errors (network/timeout) and 5xx must propagate so react-query can
        // surface them instead of masquerading as a benign undetected URL.
        if (
          axios.isAxiosError(error) &&
          UNDETECTABLE_STATUS_CODES.has(error.response?.status ?? 0)
        ) {
          return {}
        }
        throw error
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
      if (!result) {
        // A malformed/empty response hides the real outcome. Never fabricate
        // an "everything failed" summary — let react-query surface the error
        // so the caller can report it instead of showing fake zero counts.
        throw new Error('Import response was empty')
      }
      return result
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: sitesKeys.list() })
      queryClient.invalidateQueries({ queryKey: accountQueryKeys.snapshot() })
    },
    ...options,
  })
}
