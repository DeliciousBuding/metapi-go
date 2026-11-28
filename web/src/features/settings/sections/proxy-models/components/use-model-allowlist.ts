// Brand toggles are instant operations: switches are disabled while a toggle
// is in flight (serializing concurrent clicks so the last click wins), and a
// failed toggle refetches runtime settings so the local state realigns with
// the server instead of silently diverging.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { useRebuildRoutes } from '@/features/token-routes'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
import {
  runtimeSettingsQueryKeys,
  useRuntimeSettings,
  useUpdateRuntimeSettings,
  type RuntimeSettings,
} from '../../../lib/runtime-settings'

const allowlistSchema = z.object({
  globalAllowedModels: z.string().optional(),
})

type AllowlistFormValues = z.infer<typeof allowlistSchema>

const DEFAULT_VALUES: AllowlistFormValues = { globalAllowedModels: '' }

type BrandListResponse = { brands: string[] }
type TokenCandidates = { models: Record<string, unknown> }

const brandsQueryKeys = {
  all: ['settings-brand-list'] as const,
}
const candidatesQueryKeys = {
  all: ['model-token-candidates'] as const,
}

function splitAllowed(raw: unknown): string[] {
  if (Array.isArray(raw)) {
    return raw.map((item) => String(item)).filter(Boolean)
  }
  if (typeof raw === 'string') {
    return raw
      .split(/\r?\n|,/)
      .map((item) => item.trim())
      .filter(Boolean)
  }
  return []
}

function deriveServerValues(
  data: RuntimeSettings | undefined
): AllowlistFormValues | null {
  if (!data) {
    return null
  }
  return {
    globalAllowedModels: splitAllowed(data.globalAllowedModels).join('\n'),
  }
}

export function useModelAllowlist() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()
  const rebuildMutation = useRebuildRoutes()

  const brandsQuery = useQuery<BrandListResponse>({
    queryKey: brandsQueryKeys.all,
    queryFn: async () => (await api.getBrandList()) as BrandListResponse,
    staleTime: 5 * 60 * 1000,
  })

  const candidatesQuery = useQuery<TokenCandidates>({
    queryKey: candidatesQueryKeys.all,
    queryFn: async () =>
      (await api.getModelTokenCandidates()) as TokenCandidates,
    staleTime: 5 * 60 * 1000,
  })

  const serverValues = deriveServerValues(data)
  const { form, baseline, syncFromServer } =
    useSettingsForm<AllowlistFormValues>({
      schema: allowlistSchema,
      defaultValues: DEFAULT_VALUES,
      serverValues,
    })

  const [pendingModel, setPendingModel] = useState('')
  const [allowedModels, setAllowedModels] = useState<string[]>([])

  const initialAllowed = useMemo(
    () => splitAllowed(data?.globalAllowedModels),
    [data]
  )

  useEffect(() => {
    if (!form.formState.isDirty) {
      setAllowedModels(initialAllowed)
    }
  }, [form.formState.isDirty, initialAllowed])

  const candidateModels = useMemo(
    () => Object.keys(candidatesQuery.data?.models ?? {}),
    [candidatesQuery.data]
  )

  const [blockedBrands, setBlockedBrands] = useState<string[]>([])

  useEffect(() => {
    setBlockedBrands(splitAllowed(data?.globalBlockedBrands))
  }, [data])

  const brandToggleMutation = useMutation({
    mutationFn: async (nextBlocked: string[]) => {
      await api.updateRuntimeSettings({ globalBlockedBrands: nextBlocked })
      try {
        await rebuildMutation.mutateAsync({ refreshModels: false, wait: false })
      } catch {
        // The setting was saved; only the rebuild acknowledgement is unknown.
        toast.warning(t('settings.proxyModels.allowlist.toast.rebuildFailed'))
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: runtimeSettingsQueryKeys.all,
      })
      // Derived routing queries refresh when the shared task observer verifies
      // completion, not when the queue merely acknowledges the request.
      toast.success(t('settings.proxyModels.allowlist.toast.brandsSaved'))
    },
    onError: () => {
      // Re-align local state with the server so a failed toggle does not
      // leave the switches diverged from the persisted allowlist.
      void queryClient.invalidateQueries({
        queryKey: runtimeSettingsQueryKeys.all,
      })
      toast.error(t('settings.proxyModels.allowlist.toast.brandsSaveFailed'))
    },
  })

  function toggleBrand(brand: string) {
    if (brandToggleMutation.isPending) {
      return
    }
    const next = blockedBrands.includes(brand)
      ? blockedBrands.filter((item) => item !== brand)
      : [...blockedBrands, brand]
    setBlockedBrands(next)
    brandToggleMutation.mutate(next)
  }

  function addAllowedModel(model: string) {
    const trimmed = model.trim()
    if (!trimmed || allowedModels.includes(trimmed)) {
      setPendingModel('')
      return
    }
    const next = [...allowedModels, trimmed]
    setAllowedModels(next)
    form.setValue('globalAllowedModels', next.join('\n'), { shouldDirty: true })
    setPendingModel('')
  }

  function removeAllowedModel(model: string) {
    const next = allowedModels.filter((item) => item !== model)
    setAllowedModels(next)
    form.setValue('globalAllowedModels', next.join('\n'), { shouldDirty: true })
  }

  function onSubmit(values: AllowlistFormValues) {
    const changed = collectChangedFields(values, baseline)
    if (!hasChanges(changed) && changed.globalAllowedModels === undefined) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    updateMutation.mutate(
      { globalAllowedModels: splitAllowed(values.globalAllowedModels) },
      {
        onSuccess: () => {
          void rebuildMutation
            .mutateAsync({ refreshModels: false, wait: false })
            .catch(() => {
              toast.warning(
                t('settings.proxyModels.allowlist.toast.rebuildFailed')
              )
            })
          toast.success(t('settings.proxyModels.allowlist.toast.saved'))
        },
        onError: () =>
          toast.error(t('settings.proxyModels.allowlist.toast.saveFailed')),
      }
    )
  }

  return {
    data,
    isLoading,
    isError,
    refetch,
    form,
    onSubmit,
    reset: () => syncFromServer(deriveServerValues(data) ?? DEFAULT_VALUES),
    isSaving: updateMutation.isPending,
    models: {
      pendingModel,
      setPendingModel,
      allowedModels,
      candidateModels,
      addAllowedModel,
      removeAllowedModel,
    },
    brands: brandsQuery.data?.brands ?? [],
    loadingBrands: brandsQuery.isLoading,
    blockedBrands,
    toggleBrand,
    brandTogglePending: brandToggleMutation.isPending,
  }
}
