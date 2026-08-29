// metapi-go/features/settings/sections/proxy-models/components — global allowlist
// + brand-blocking section (legacy cards 10-11). globalAllowedModels is an
// inline text input + badges; globalBlockedBrands is a grid of toggle
// switches sourced from api.getBrandList(). Both saves trigger a routes
// rebuild (api.rebuildRoutes(false)) so the channel graph stays consistent.
//
// Brand toggles are instant operations: switches are disabled while a toggle
// is in flight (serializing concurrent clicks so the last click wins), and a
// failed toggle refetches runtime settings so the local state realigns with
// the server instead of silently diverging.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { SettingsSectionCard } from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
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

const ALLOWLIST_FORM_ID = 'settings-models-allowlist-form'

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

export function AllowlistSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

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
      await api.rebuildRoutes(false)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: runtimeSettingsQueryKeys.all,
      })
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
          void api.rebuildRoutes(false).catch(() => {
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

  if (isLoading) {
    return (
      <SettingsSectionCard
        title={t('settings.proxyModels.allowlist.title')}
        description={t('settings.proxyModels.allowlist.description')}
      >
        <p className='text-muted-foreground text-sm'>
          {t('settings.common.loading')}
        </p>
      </SettingsSectionCard>
    )
  }

  if (isError || !data) {
    return (
      <SettingsSectionError
        title={t('settings.proxyModels.allowlist.title')}
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty
  const brands = brandsQuery.data?.brands ?? []
  const brandTogglePending = brandToggleMutation.isPending

  return (
    <SettingsSectionCard
      title={t('settings.proxyModels.allowlist.title')}
      description={t('settings.proxyModels.allowlist.description')}
    >
      <div className='space-y-4'>
        <Form {...form}>
          <form
            id={ALLOWLIST_FORM_ID}
            onSubmit={form.handleSubmit(onSubmit)}
            className='space-y-4'
          >
            <FormField
              control={form.control}
              name='globalAllowedModels'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t(
                      'settings.proxyModels.allowlist.fields.globalAllowedModels'
                    )}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'settings.proxyModels.allowlist.fields.globalAllowedModelsHint'
                    )}
                  </FormDescription>
                  <div className='flex flex-wrap gap-2'>
                    {allowedModels.map((model) => (
                      <Badge
                        key={model}
                        variant='default'
                        className='cursor-pointer'
                        onClick={() => removeAllowedModel(model)}
                      >
                        {model} ×
                      </Badge>
                    ))}
                  </div>
                  <div className='flex gap-2'>
                    <FormControl>
                      <Input
                        value={pendingModel}
                        onChange={(event) =>
                          setPendingModel(event.target.value)
                        }
                        onKeyDown={(event) => {
                          if (event.key === 'Enter') {
                            event.preventDefault()
                            addAllowedModel(pendingModel)
                          }
                        }}
                        placeholder={t(
                          'settings.proxyModels.allowlist.addPlaceholder'
                        )}
                      />
                    </FormControl>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() => addAllowedModel(pendingModel)}
                    >
                      {t('settings.common.add')}
                    </Button>
                  </div>
                  {candidateModels.length > 0 ? (
                    <div className='flex flex-wrap gap-1'>
                      {candidateModels.slice(0, 20).map((model) => (
                        <button
                          type='button'
                          key={model}
                          className='cursor-pointer'
                          onClick={() => addAllowedModel(model)}
                        >
                          <Badge variant='secondary'>+ {model}</Badge>
                        </button>
                      ))}
                    </div>
                  ) : null}
                  <input type='hidden' {...field} />
                  <FormMessage />
                </FormItem>
              )}
            />
            <SettingsFormActions
              formId={ALLOWLIST_FORM_ID}
              isDirty={isDirty}
              isPending={updateMutation.isPending}
              onReset={() =>
                syncFromServer(deriveServerValues(data) ?? DEFAULT_VALUES)
              }
            />
          </form>
        </Form>

        <div className='space-y-3 rounded-lg border p-4'>
          <div>
            <h3 className='text-sm font-medium'>
              {t('settings.proxyModels.allowlist.fields.globalBlockedBrands')}
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'settings.proxyModels.allowlist.fields.globalBlockedBrandsHint'
              )}
            </p>
          </div>
          {brandsQuery.isLoading ? (
            <p className='text-muted-foreground text-sm'>
              {t('settings.proxyModels.allowlist.loadingBrands')}
            </p>
          ) : null}
          {!brandsQuery.isLoading && brands.length === 0 ? (
            <p className='text-muted-foreground text-sm'>
              {t('settings.proxyModels.allowlist.noBrands')}
            </p>
          ) : null}
          {!brandsQuery.isLoading && brands.length > 0 ? (
            <div className='grid grid-cols-2 gap-3 sm:grid-cols-3'>
              {brands.map((brand) => (
                <div
                  key={brand}
                  className='flex items-center justify-between gap-2 rounded-md border p-2'
                >
                  <span className='text-sm'>{brand}</span>
                  <Switch
                    checked={!blockedBrands.includes(brand)}
                    onCheckedChange={() => toggleBrand(brand)}
                    disabled={brandTogglePending}
                    aria-label={brand}
                  />
                </div>
              ))}
            </div>
          ) : null}
          {blockedBrands.length > 0 ? (
            <p className='text-muted-foreground text-xs'>
              {t('settings.proxyModels.allowlist.blockedCount', {
                count: blockedBrands.length,
              })}
            </p>
          ) : null}
        </div>
      </div>
      <FormNavigationGuard enabled={isDirty} />
    </SettingsSectionCard>
  )
}
