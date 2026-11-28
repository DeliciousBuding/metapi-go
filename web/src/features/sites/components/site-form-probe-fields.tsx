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
import { Switch } from '@/components/ui/switch'

import type { SiteFormValues } from '../lib/sites-schema'
import type { SiteProbeScope } from '../types'

export function SiteFormProbeFields({
  form,
}: {
  form: UseFormReturn<SiteFormValues>
}) {
  const { t } = useTranslation()
  const probeEnabled = form.watch('postRefreshProbeEnabled')
  return (
    <div className='space-y-4'>
      <FormField
        control={form.control}
        name='postRefreshProbeEnabled'
        render={({ field }) => (
          <FormItem className='flex items-center justify-between gap-4'>
            <div className='space-y-0.5'>
              <FormLabel>{t('sites.form.postRefreshProbeEnabled')}</FormLabel>
              <FormDescription>
                {t('sites.form.postRefreshProbeEnabledHint')}
              </FormDescription>
            </div>
            <FormControl>
              <Switch
                checked={field.value}
                onCheckedChange={(checked) => field.onChange(checked)}
              />
            </FormControl>
          </FormItem>
        )}
      />
      {probeEnabled && (
        <div className='mt-3 grid items-start gap-x-5 gap-y-4 sm:grid-cols-2'>
          <FormField
            control={form.control}
            name='postRefreshProbeModel'
            render={({ field }) => (
              <FormItem className='sm:col-span-2'>
                <FormLabel>{t('sites.form.postRefreshProbeModel')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t(
                      'sites.form.postRefreshProbeModelPlaceholder'
                    )}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='postRefreshProbeScope'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('sites.form.postRefreshProbeScope')}</FormLabel>
                <Select
                  value={field.value}
                  onValueChange={(value) =>
                    field.onChange(value as SiteProbeScope)
                  }
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue
                        placeholder={t('sites.form.scopePlaceholder')}
                      >
                        {(value: unknown) =>
                          value === 'all'
                            ? t('sites.form.scopeAll')
                            : t('sites.form.scopeSingle')
                        }
                      </SelectValue>
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value='single'>
                      {t('sites.form.scopeSingle')}
                    </SelectItem>
                    <SelectItem value='all'>
                      {t('sites.form.scopeAll')}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='postRefreshProbeLatencyThresholdMs'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('sites.form.postRefreshProbeLatencyThresholdMsLabel')}
                </FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    step={100}
                    value={field.value}
                    onChange={(event) =>
                      field.onChange(
                        Number.isNaN(event.target.valueAsNumber)
                          ? 0
                          : event.target.valueAsNumber
                      )
                    }
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t('sites.form.postRefreshProbeLatencyThresholdMsHint')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>
      )}
    </div>
  )
}
