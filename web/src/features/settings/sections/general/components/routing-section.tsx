// metapi-go/features/settings/sections/general/components — routing strategy
// section. Fallback unit cost, cooldown (stored as seconds on the wire),
// first-byte timeout, cross-protocol fallback toggle, and the five
// routing-weight factors. Three presets (balanced / stable / cost) apply
// canonical weight profiles.

import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import {
  asBoolean,
  asNumber,
  useRuntimeSettings,
  useUpdateRuntimeSettings,
} from '../../../lib/runtime-settings'

const FORM_ID = 'settings-general-routing-form'

const routingSchema = z.object({
  routingFallbackUnitCost: z.coerce.number().min(0).optional(),
  tokenRouterFailureCooldownMaxSec: z.coerce.number().int().min(0).optional(),
  proxyFirstByteTimeoutSec: z.coerce.number().int().min(0).optional(),
  disableCrossProtocolFallback: z.boolean().optional(),
  routingWeights: z.object({
    baseWeightFactor: z.coerce.number().optional(),
    valueScoreFactor: z.coerce.number().optional(),
    costWeight: z.coerce.number().optional(),
    balanceWeight: z.coerce.number().optional(),
    usageWeight: z.coerce.number().optional(),
  }),
})

type RoutingFormValues = z.infer<typeof routingSchema>

type RoutingPreset = {
  id: 'balanced' | 'stable' | 'cost'
  weights: NonNullable<RoutingFormValues['routingWeights']>
}

const ROUTING_PRESETS: readonly RoutingPreset[] = [
  {
    id: 'balanced',
    weights: {
      baseWeightFactor: 0.5,
      valueScoreFactor: 0.5,
      costWeight: 0.4,
      balanceWeight: 0.3,
      usageWeight: 0.3,
    },
  },
  {
    id: 'stable',
    weights: {
      baseWeightFactor: 0.7,
      valueScoreFactor: 0.3,
      costWeight: 0.2,
      balanceWeight: 0.5,
      usageWeight: 0.2,
    },
  },
  {
    id: 'cost',
    weights: {
      baseWeightFactor: 0.3,
      valueScoreFactor: 0.4,
      costWeight: 0.7,
      balanceWeight: 0.2,
      usageWeight: 0.4,
    },
  },
]

const DEFAULT_WEIGHTS: RoutingPreset['weights'] = ROUTING_PRESETS[0].weights

export function RoutingSection() {
  const { t } = useTranslation()
  const { data, isLoading } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const form = useForm<RoutingFormValues>({
    resolver: zodResolver(routingSchema) as never,
    defaultValues: {
      routingFallbackUnitCost: 1,
      tokenRouterFailureCooldownMaxSec: 30 * 24 * 60 * 60,
      proxyFirstByteTimeoutSec: 0,
      disableCrossProtocolFallback: false,
      routingWeights: { ...DEFAULT_WEIGHTS },
    },
  })

  useEffect(() => {
    if (!data) {
      return
    }
    const incomingWeights = (data.routingWeights ?? {}) as Record<
      string,
      unknown
    >
    form.reset(
      {
        routingFallbackUnitCost: asNumber(data.routingFallbackUnitCost) ?? 1,
        tokenRouterFailureCooldownMaxSec:
          asNumber(data.tokenRouterFailureCooldownMaxSec) ?? 30 * 24 * 60 * 60,
        proxyFirstByteTimeoutSec: asNumber(data.proxyFirstByteTimeoutSec) ?? 0,
        disableCrossProtocolFallback: asBoolean(
          data.disableCrossProtocolFallback
        ),
        routingWeights: {
          baseWeightFactor:
            asNumber(incomingWeights.baseWeightFactor) ??
            DEFAULT_WEIGHTS.baseWeightFactor,
          valueScoreFactor:
            asNumber(incomingWeights.valueScoreFactor) ??
            DEFAULT_WEIGHTS.valueScoreFactor,
          costWeight:
            asNumber(incomingWeights.costWeight) ?? DEFAULT_WEIGHTS.costWeight,
          balanceWeight:
            asNumber(incomingWeights.balanceWeight) ??
            DEFAULT_WEIGHTS.balanceWeight,
          usageWeight:
            asNumber(incomingWeights.usageWeight) ??
            DEFAULT_WEIGHTS.usageWeight,
        },
      },
      { keepDirtyValues: true }
    )
  }, [data, form])

  function applyPreset(preset: RoutingPreset) {
    form.setValue(
      'routingWeights',
      { ...preset.weights },
      { shouldDirty: true }
    )
    toast.info(t(`settings.general.routing.preset.${preset.id}`))
  }

  function onSubmit(values: RoutingFormValues) {
    updateMutation.mutate(values as never, {
      onSuccess: () => toast.success(t('settings.general.routing.toast.saved')),
      onError: () =>
        toast.error(t('settings.general.routing.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return <SettingsSectionSkeleton />
  }

  return (
    <SettingsSectionCard
      title={t('settings.general.routing.title')}
      description={t('settings.general.routing.description')}
    >
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='routingFallbackUnitCost'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t(
                      'settings.general.routing.fields.routingFallbackUnitCost'
                    )}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      type='number'
                      min={0}
                      step={0.000001}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'settings.general.routing.fields.routingFallbackUnitCostHint'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='tokenRouterFailureCooldownMaxSec'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.general.routing.fields.routeFailureCooldown')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      type='number'
                      min={0}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'settings.general.routing.fields.routeFailureCooldownHint'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='proxyFirstByteTimeoutSec'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.general.routing.fields.proxyFirstByteTimeout')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      type='number'
                      min={0}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'settings.general.routing.fields.proxyFirstByteTimeoutHint'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='disableCrossProtocolFallback'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center gap-3 pt-6'>
                  <FormControl>
                    <Checkbox
                      checked={Boolean(field.value)}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <div className='space-y-1'>
                    <FormLabel className='cursor-pointer'>
                      {t(
                        'settings.general.routing.fields.disableCrossProtocolFallback'
                      )}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'settings.general.routing.fields.disableCrossProtocolFallbackHint'
                      )}
                    </FormDescription>
                  </div>
                </FormItem>
              )}
            />
          </div>

          <div className='space-y-3 rounded-lg border p-4'>
            <div className='flex items-center justify-between gap-2'>
              <h4 className='text-sm font-medium'>
                {t('settings.general.routing.weightsGroup')}
              </h4>
              <div className='flex gap-2'>
                {ROUTING_PRESETS.map((preset) => (
                  <Button
                    key={preset.id}
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => applyPreset(preset)}
                  >
                    {t(`settings.general.routing.preset.${preset.id}`)}
                  </Button>
                ))}
              </div>
            </div>
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='routingWeights.baseWeightFactor'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('settings.general.routing.fields.baseWeightFactor')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        type='number'
                        min={0}
                        step={0.05}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='routingWeights.valueScoreFactor'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('settings.general.routing.fields.valueScoreFactor')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        type='number'
                        min={0}
                        step={0.05}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='routingWeights.costWeight'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('settings.general.routing.fields.costWeight')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        type='number'
                        min={0}
                        step={0.05}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='routingWeights.balanceWeight'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('settings.general.routing.fields.balanceWeight')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        type='number'
                        min={0}
                        step={0.05}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='routingWeights.usageWeight'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('settings.general.routing.fields.usageWeight')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        type='number'
                        min={0}
                        step={0.05}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <p className='text-muted-foreground text-xs'>
              {t('settings.general.routing.weightsHint')}
            </p>
          </div>

          <Button
            type='submit'
            form={FORM_ID}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending
              ? t('settings.common.saving')
              : t('settings.common.save')}
          </Button>
        </form>
      </Form>
    </SettingsSectionCard>
  )
}
