// metapi-go/features/settings/sections/models/components — global allowlist
// + brand-blocking section (legacy cards 10-11). globalAllowedModels is an
// inline text input + badges; globalBlockedBrands is a grid of toggle
// switches sourced from api.getBrandList(). Both saves trigger a routes
// rebuild (api.rebuildRoutes(false)) so the channel graph stays consistent.

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { api } from '@/lib/api'

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
import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import {
  useRuntimeSettings,
  useUpdateRuntimeSettings,
} from '../../../lib/runtime-settings'

const ALLOWLIST_FORM_ID = 'settings-models-allowlist-form'

const allowlistSchema = z.object({
  globalAllowedModels: z.string().optional(),
})

type AllowlistFormValues = z.infer<typeof allowlistSchema>

type BrandListResponse = { brands: string[] }
type TokenCandidates = { models: Record<string, unknown> }

const brandsQueryKeys = {
  all: ['settings-brand-list'] as const,
}
const candidatesQueryKeys = {
  all: ['model-token-candidates'] as const,
}

export function AllowlistSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const brandsQuery = useQuery<BrandListResponse>({
    queryKey: brandsQueryKeys.all,
    queryFn: async () => (await api.getBrandList()) as BrandListResponse,
    staleTime: 5 * 60 * 1000,
  })

  const candidatesQuery = useQuery<TokenCandidates>({
    queryKey: candidatesQueryKeys.all,
    queryFn: async () => (await api.getModelTokenCandidates()) as TokenCandidates,
    staleTime: 5 * 60 * 1000,
  })

  const form = useForm<AllowlistFormValues>({
    resolver: zodResolver(allowlistSchema) as never,
    defaultValues: { globalAllowedModels: '' },
  })

  const [pendingModel, setPendingModel] = useState('')

  // Active set of allowed models (string[]) held in local state so the
  // badges can be removed without a round-trip; synced to the form on save.
  const initialAllowed = useMemo(() => {
    const raw = data?.globalAllowedModels
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
  }, [data])

  const [allowedModels, setAllowedModels] = useState<string[]>([])

  useEffect(() => {
    setAllowedModels(initialAllowed)
    form.reset(
      { globalAllowedModels: initialAllowed.join('\n') },
      { keepDirtyValues: true },
    )
  }, [initialAllowed, form])

  const candidateModels = useMemo(
    () => Object.keys(candidatesQuery.data?.models ?? {}),
    [candidatesQuery.data],
  )

  const [blockedBrands, setBlockedBrands] = useState<string[]>([])

  useEffect(() => {
    const raw = data?.globalBlockedBrands
    if (Array.isArray(raw)) {
      setBlockedBrands(raw.map((item) => String(item)).filter(Boolean))
    } else if (typeof raw === 'string') {
      setBlockedBrands(
        raw
          .split(/\r?\n|,/)
          .map((item) => item.trim())
          .filter(Boolean),
      )
    } else {
      setBlockedBrands([])
    }
  }, [data])

  const brandToggleMutation = useMutation({
    mutationFn: async (nextBlocked: string[]) => {
      await api.updateRuntimeSettings({ globalBlockedBrands: nextBlocked })
      await api.rebuildRoutes(false)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['runtime-settings'] })
      toast.success(t('settings.models.allowlist.toast.brandsSaved'))
    },
    onError: () => toast.error(t('settings.models.allowlist.toast.brandsSaveFailed')),
  })

  function toggleBrand(brand: string) {
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
    const list = values.globalAllowedModels
      ? values.globalAllowedModels
          .split(/\r?\n|,/)
          .map((item) => item.trim())
          .filter(Boolean)
      : []
    updateMutation.mutate(
      { globalAllowedModels: list },
      {
        onSuccess: async () => {
          try {
            await api.rebuildRoutes(false)
          } catch {
            toast.warning(t('settings.models.allowlist.toast.rebuildFailed'))
            return
          }
          toast.success(t('settings.models.allowlist.toast.saved'))
        },
        onError: () => toast.error(t('settings.models.allowlist.toast.saveFailed')),
      },
    )
  }

  if (isLoading) {
    return <SettingsSectionSkeleton />
  }

  const brands = brandsQuery.data?.brands ?? []

  return (
    <SettingsSectionCard
      title={t('settings.models.allowlist.title')}
      description={t('settings.models.allowlist.description')}
    >
      <div className='space-y-6'>
        <Form {...form}>
          <form
            id={ALLOWLIST_FORM_ID}
            onSubmit={form.handleSubmit(onSubmit)}
            className='space-y-3'
          >
            <FormField
              control={form.control}
              name='globalAllowedModels'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.models.allowlist.fields.globalAllowedModels')}
                  </FormLabel>
                  <FormDescription>
                    {t('settings.models.allowlist.fields.globalAllowedModelsHint')}
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
                    {allowedModels.length === 0 ? (
                      <span className='text-xs text-muted-foreground'>
                        {t('settings.models.allowlist.allowAll')}
                      </span>
                    ) : null}
                  </div>
                  <div className='flex gap-2'>
                    <FormControl>
                      <Input
                        value={pendingModel}
                        onChange={(event) => setPendingModel(event.target.value)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter') {
                            event.preventDefault()
                            addAllowedModel(pendingModel)
                          }
                        }}
                        placeholder={t('settings.models.allowlist.addPlaceholder')}
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
            <Button
              type='submit'
              form={ALLOWLIST_FORM_ID}
              disabled={updateMutation.isPending}
            >
              {updateMutation.isPending
                ? t('settings.common.saving')
                : t('settings.common.save')}
            </Button>
          </form>
        </Form>

        <div className='space-y-3 rounded-lg border p-4'>
          <div>
            <h4 className='text-sm font-medium'>
              {t('settings.models.allowlist.fields.globalBlockedBrands')}
            </h4>
            <p className='text-xs text-muted-foreground'>
              {t('settings.models.allowlist.fields.globalBlockedBrandsHint')}
            </p>
          </div>
          {brandsQuery.isLoading ? (
            <p className='text-sm text-muted-foreground'>
              {t('settings.models.allowlist.loadingBrands')}
            </p>
          ) : null}
          {!brandsQuery.isLoading && brands.length === 0 ? (
            <p className='text-sm text-muted-foreground'>
              {t('settings.models.allowlist.noBrands')}
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
                    aria-label={brand}
                  />
                </div>
              ))}
            </div>
          ) : null}
          {blockedBrands.length > 0 ? (
            <p className='text-xs text-muted-foreground'>
              {t('settings.models.allowlist.blockedCount', { count: blockedBrands.length })}
            </p>
          ) : null}
        </div>
      </div>
    </SettingsSectionCard>
  )
}
