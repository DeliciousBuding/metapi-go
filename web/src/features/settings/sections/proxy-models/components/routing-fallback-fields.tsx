import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import type { RoutingFormValues } from './routing-form'

export function RoutingFallbackFields() {
  const { t } = useTranslation()
  const form = useFormContext<RoutingFormValues>()
  return (
    <>
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

      <div className='space-y-3 rounded-lg border p-4'>
        <h3 className='text-sm font-medium'>
          {t('settings.proxyModels.routing.statusCodeGroup')}
        </h3>
        <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
          <FormField
            control={form.control}
            name='proxyRetryStatusRanges'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t(
                    'settings.proxyModels.routing.fields.proxyRetryStatusRanges'
                  )}
                </FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    placeholder='401,403,408,409,425,429,500-599'
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'settings.proxyModels.routing.fields.proxyRetryStatusRangesHint'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='proxyDisableStatusRanges'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t(
                    'settings.proxyModels.routing.fields.proxyDisableStatusRanges'
                  )}
                </FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    placeholder='401'
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'settings.proxyModels.routing.fields.proxyDisableStatusRangesHint'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>
      </div>

      <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
        <FormField
          control={form.control}
          name='tokenRouterFailureCooldownMaxSec'
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t('settings.proxyModels.routing.fields.routeFailureCooldown')}
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
                {t('settings.proxyModels.routing.fields.proxyFirstByteTimeout')}
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
    </>
  )
}
