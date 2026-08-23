// metapi-go/features/settings/sections/basic/components — routing strategy
// section. Fallback unit cost, cooldown (stored as seconds on the wire),
// first-byte timeout, cross-protocol fallback toggle, and the five
// routing-weight factors. Three presets (balanced / stable / cost) fill the
// weight fields; saving is explicit through the shared actions row.

import { useTranslation } from 'react-i18next'
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
  asBoolean,
  asNumber,
  useRuntimeSettings,
  useUpdateRuntimeSettings,
  type RuntimeSettings,
} from '../../../lib/runtime-settings'

const FORM_ID = 'settings-general-routing-form'

const routingSchema = z.object({
  routingFallbackUnitCost: z.coerce.number().min(0),
  tokenRouterFailureCooldownMaxSec: z.coerce.number().int().min(0),
  proxyFirstByteTimeoutSec: z.coerce.number().int().min(0),
  disableCrossProtocolFallback: z.boolean(),
  routingWeights: z.object({
    baseWeightFactor: z.coerce.number(),
    valueScoreFactor: z.coerce.number(),
    costWeight: z.coerce.number(),
    balanceWeight: z.coerce.number(),
    usageWeight: z.coerce.number(),
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

const DEFAULT_VALUES: RoutingFormValues = {
  routingFallbackUnitCost: 1,
  tokenRouterFailureCooldownMaxSec: 30 * 24 * 60 * 60,
  proxyFirstByteTimeoutSec: 0,
  disableCrossProtocolFallback: false,
  routingWeights: { ...DEFAULT_WEIGHTS },
}

function deriveServerValues(
  data: RuntimeSettings | undefined
): RoutingFormValues | null {
  if (!data) {
    return null
  }
  const incomingWeights = (data.routingWeights ?? {}) as Record<string, unknown>
  return {
    routingFallbackUnitCost: asNumber(data.routingFallbackUnitCost) ?? 1,
    tokenRouterFailureCooldownMaxSec:
      asNumber(data.tokenRouterFailureCooldownMaxSec) ?? 30 * 24 * 60 * 60,
    proxyFirstByteTimeoutSec: asNumber(data.proxyFirstByteTimeoutSec) ?? 0,
    disableCrossProtocolFallback: asBoolean(data.disableCrossProtocolFallback),
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
        asNumber(incomingWeights.usageWeight) ?? DEFAULT_WEIGHTS.usageWeight,
    },
  }
}

export function RoutingSection() {
  const { t } = useTranslation()
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const serverValues = deriveServerValues(data)
  const { form, baseline, syncFromServer } = useSettingsForm<RoutingFormValues>(
    {
      schema: routingSchema,
      defaultValues: DEFAULT_VALUES,
      serverValues,
    }
  )

  function applyPreset(preset: RoutingPreset) {
    form.setValue(
      'routingWeights',
      { ...preset.weights },
      { shouldDirty: true }
    )
    toast.info(t(`settings.proxyModels.routing.preset.${preset.id}`))
  }

  function onSubmit(values: RoutingFormValues) {
    const changed = collectChangedFields(
      values as unknown as Record<string, unknown>,
      baseline as unknown as Record<string, unknown> | null
    ) as Partial<RoutingFormValues>
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    updateMutation.mutate(changed as never, {
      onSuccess: () =>
        toast.success(t('settings.proxyModels.routing.toast.saved')),
      onError: () =>
        toast.error(t('settings.proxyModels.routing.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return (
      <SettingsSectionCard
        title={t('settings.proxyModels.routing.title')}
        description={t('settings.proxyModels.routing.description')}
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
        title={t('settings.proxyModels.routing.title')}
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SettingsSectionCard
      title={t('settings.proxyModels.routing.title')}
      description={t('settings.proxyModels.routing.description')}
    >
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <div className='space-y-3 rounded-lg border p-4'>
            <h3 className='text-sm font-medium'>
              {t('settings.proxyModels.routing.fallbackGroup')}
            </h3>
            <FormField
              control={form.control}
              name='routingFallbackUnitCost'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t(
                      'settings.proxyModels.routing.fields.routingFallbackUnitCost'
                    )}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      type='number'
                      min={0}
                      step={0.01}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'settings.proxyModels.routing.fields.routingFallbackUnitCostHint'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='tokenRouterFailureCooldownMaxSec'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t(
                      'settings.proxyModels.routing.fields.routeFailureCooldown'
                    )}
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
                      'settings.proxyModels.routing.fields.routeFailureCooldownHint'
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
                    {t(
                      'settings.proxyModels.routing.fields.proxyFirstByteTimeout'
                    )}
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
                      'settings.proxyModels.routing.fields.proxyFirstByteTimeoutHint'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='disableCrossProtocolFallback'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center gap-3'>
                <FormControl>
                  <Checkbox
                    checked={Boolean(field.value)}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
                <div className='space-y-1'>
                  <FormLabel className='cursor-pointer'>
                    {t(
                      'settings.proxyModels.routing.fields.disableCrossProtocolFallback'
                    )}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'settings.proxyModels.routing.fields.disableCrossProtocolFallbackHint'
                    )}
                  </FormDescription>
                </div>
              </FormItem>
            )}
          />

          <div className='space-y-3 rounded-lg border p-4'>
            <div className='flex items-center justify-between gap-2'>
              <h3 className='text-sm font-medium'>
                {t('settings.proxyModels.routing.weightsGroup')}
              </h3>
              <div className='flex gap-2'>
                {ROUTING_PRESETS.map((preset) => (
                  <Button
                    key={preset.id}
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => applyPreset(preset)}
                  >
                    {t(`settings.proxyModels.routing.preset.${preset.id}`)}
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
                      {t(
                        'settings.proxyModels.routing.fields.baseWeightFactor'
                      )}
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
                    <FormDescription>
                      {t(
                        'settings.proxyModels.routing.fields.baseWeightFactorHint'
                      )}
                    </FormDescription>
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
                      {t(
                        'settings.proxyModels.routing.fields.valueScoreFactor'
                      )}
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
                    <FormDescription>
                      {t(
                        'settings.proxyModels.routing.fields.valueScoreFactorHint'
                      )}
                    </FormDescription>
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
                      {t('settings.proxyModels.routing.fields.costWeight')}
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
                    <FormDescription>
                      {t('settings.proxyModels.routing.fields.costWeightHint')}
                    </FormDescription>
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
                      {t('settings.proxyModels.routing.fields.balanceWeight')}
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
                    <FormDescription>
                      {t(
                        'settings.proxyModels.routing.fields.balanceWeightHint'
                      )}
                    </FormDescription>
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
                      {t('settings.proxyModels.routing.fields.usageWeight')}
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
                    <FormDescription>
                      {t('settings.proxyModels.routing.fields.usageWeightHint')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <p className='text-muted-foreground text-xs'>
              {t('settings.proxyModels.routing.weightsHint')}
            </p>
          </div>

          <SettingsFormActions
            formId={FORM_ID}
            isDirty={isDirty}
            isPending={updateMutation.isPending}
            onReset={() =>
              syncFromServer(deriveServerValues(data) ?? DEFAULT_VALUES)
            }
          />
        </form>
      </Form>
      <FormNavigationGuard enabled={isDirty} />
    </SettingsSectionCard>
  )
}
