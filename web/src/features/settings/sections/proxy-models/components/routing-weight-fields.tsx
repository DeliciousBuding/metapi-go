import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { toast } from '@/lib/toast'

import { ROUTING_PRESETS, type RoutingFormValues } from './routing-form'

const weightFields = [
  'baseWeightFactor',
  'valueScoreFactor',
  'costWeight',
  'balanceWeight',
  'usageWeight',
] as const

export function RoutingWeightFields() {
  const { t } = useTranslation()
  const form = useFormContext<RoutingFormValues>()
  function applyPreset(preset: (typeof ROUTING_PRESETS)[number]) {
    form.setValue(
      'routingWeights',
      { ...preset.weights },
      { shouldDirty: true }
    )
    toast.info(t(`settings.proxyModels.routing.preset.${preset.id}`))
  }
  return (
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
        {weightFields.map((weight) => (
          <FormField
            key={weight}
            control={form.control}
            name={`routingWeights.${weight}`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t(`settings.proxyModels.routing.fields.${weight}`)}
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
                  {t(`settings.proxyModels.routing.fields.${weight}Hint`)}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        ))}
      </div>
      <p className='text-muted-foreground text-xs'>
        {t('settings.proxyModels.routing.weightsHint')}
      </p>
    </div>
  )
}
