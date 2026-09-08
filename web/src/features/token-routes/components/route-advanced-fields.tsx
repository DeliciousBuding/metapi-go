import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

import type { RouteFormValues } from '../lib/routes-schema'
import type { RouteRoutingStrategy } from '../types'
import { ROUTE_ICON_NONE_VALUE } from '../utils'

export function RouteAdvancedFields({
  form,
}: {
  form: UseFormReturn<RouteFormValues>
}) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-5 pt-4'>
      <FormField
        control={form.control}
        name='displayIcon'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('tokenRoutes.form.displayIcon')}</FormLabel>
            <FormControl>
              {/* The storage protocol sentinel ROUTE_ICON_NONE_VALUE is
                  mapped to a friendly "none" token at the input
                  boundary so users never see the internal value. */}
              <Input
                placeholder={t('tokenRoutes.form.displayIconPlaceholder')}
                name={field.name}
                ref={field.ref}
                onBlur={field.onBlur}
                value={
                  field.value === ROUTE_ICON_NONE_VALUE
                    ? 'none'
                    : (field.value ?? '')
                }
                onChange={(event) => {
                  const raw = event.target.value
                  field.onChange(
                    raw.trim().toLowerCase() === 'none'
                      ? ROUTE_ICON_NONE_VALUE
                      : raw
                  )
                }}
              />
            </FormControl>
            <FormDescription>
              {t('tokenRoutes.form.displayIconHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name='contextLength'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('tokenRoutes.form.contextLength')}</FormLabel>
            <FormControl>
              <Input
                type='number'
                placeholder='128000'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>
              {t('tokenRoutes.form.contextLengthHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name='routingStrategy'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('tokenRoutes.form.routingStrategy')}</FormLabel>
            <Select
              value={field.value ?? 'weighted'}
              onValueChange={(value) =>
                field.onChange(value as RouteRoutingStrategy)
              }
            >
              <FormControl>
                <SelectTrigger>
                  <SelectValue>
                    {(selected) =>
                      t(
                        `tokenRoutes.strategies.${String(selected ?? 'weighted')}`
                      )
                    }
                  </SelectValue>
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value='weighted'>
                  {t('tokenRoutes.strategies.weighted')}
                </SelectItem>
                <SelectItem value='round_robin'>
                  {t('tokenRoutes.strategies.round_robin')}
                </SelectItem>
                <SelectItem value='stable_first'>
                  {t('tokenRoutes.strategies.stable_first')}
                </SelectItem>
                <SelectItem value='least_busy'>
                  {t('tokenRoutes.strategies.least_busy')}
                </SelectItem>
                <SelectItem value='lowest_latency'>
                  {t('tokenRoutes.strategies.lowest_latency')}
                </SelectItem>
                <SelectItem value='lowest_cost'>
                  {t('tokenRoutes.strategies.lowest_cost')}
                </SelectItem>
              </SelectContent>
            </Select>
            <FormDescription>
              {t('tokenRoutes.form.routingStrategyHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name='modelMapping'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('tokenRoutes.form.modelMapping')}</FormLabel>
            <FormControl>
              <Textarea
                placeholder={t('tokenRoutes.form.modelMappingPlaceholder')}
                rows={2}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>
              {t('tokenRoutes.form.modelMappingHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  )
}
