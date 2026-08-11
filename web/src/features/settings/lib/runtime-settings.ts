// metapi-go/features/settings/lib — TanStack Query hooks wrapping the
// runtime-settings endpoint (GET/PUT /api/settings/runtime). All 18 sections
// share this single cache entry; writes invalidate it so every mounted
// section refetches the merged server state.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'

import { api, type RuntimeSettingsPayload } from '@/lib/api'

/**
 * Loose view of the runtime-settings payload. The backend may return extra
 * branding/site keys (systemName / logo / footer / about / homePageContent /
 * serverAddress) that are not declared on {@link RuntimeSettingsPayload}; we
 * treat the object as a string-indexed bag so the section forms can read
 * those keys without fighting the type system.
 */
export type RuntimeSettings = Record<string, unknown>

const runtimeSettingsQueryKeys = {
  all: ['runtime-settings'] as const,
  detail: () => [...runtimeSettingsQueryKeys.all, 'detail'] as const,
}

/**
 * Read the merged runtime-settings document. All mounted sections share this
 * cache entry; `useUpdateRuntimeSettings` invalidates it after writes.
 */
export function useRuntimeSettings(
  options?: Omit<UseQueryOptions<RuntimeSettings>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: runtimeSettingsQueryKeys.detail(),
    queryFn: async () => {
      const data = await api.getRuntimeSettings()
      return (data ?? {}) as RuntimeSettings
    },
    staleTime: 15 * 1000,
    ...options,
  })
}

/**
 * Partial update against PUT /api/settings/runtime. The backend accepts a
 * partial payload, so callers pass only the keys they own. On success the
 * shared runtime-settings cache is invalidated so every mounted section
 * refetches the new server state.
 */
export function useUpdateRuntimeSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: Partial<RuntimeSettingsPayload>) =>
      api.updateRuntimeSettings(payload),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: runtimeSettingsQueryKeys.all,
      })
    },
  })
}

/**
 * Split a runtime list field that the backend serializes as either a
 * newline/comma string or a string[]. Returns a clean string[] with empty
 * entries removed. Used by adminIpAllowlist, proxyErrorKeywords,
 * globalBlockedBrands, globalAllowedModels, etc.
 */
export function splitListField(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean)
  }
  if (typeof value === 'string') {
    return value
      .split(/\r?\n|,/)
      .map((item) => item.trim())
      .filter(Boolean)
  }
  return []
}

/**
 * Join a string[] back into the newline-separated form expected by the
 * textarea inputs (adminIpAllowlist / proxyErrorKeywords).
 */
export function joinListField(items: readonly string[]): string {
  return items.join('\n')
}

/** Coerce an unknown runtime value to a string (empty string when missing). */
export function asString(value: unknown): string {
  if (value === null || value === undefined) {
    return ''
  }
  return String(value)
}

/** Coerce an unknown runtime value to a number (NaN→undefined for Zod). */
export function asNumber(value: unknown): number | undefined {
  if (value === null || value === undefined || value === '') {
    return undefined
  }
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

/** Coerce an unknown runtime value to a boolean. */
export function asBoolean(value: unknown): boolean {
  return Boolean(value)
}
