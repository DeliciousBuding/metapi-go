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

import type { SiteFormValues } from '../lib/sites-schema'
import { SiteFormProbeFields } from './site-form-probe-fields'

export function SiteFormRoutingFields({
  form,
}: {
  form: UseFormReturn<SiteFormValues>
}) {
  const { t } = useTranslation()
  return (
    <fieldset className='min-w-0 space-y-4'>
      <legend className='mb-4 text-sm font-semibold'>
        {t('sites.form.sections.routing')}
      </legend>
      <div className='grid items-start gap-x-5 gap-y-4 sm:grid-cols-2'>
        <FormField
          control={form.control}
          name='globalWeight'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('sites.form.globalWeight')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={0}
                  step={1}
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
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='maxConcurrency'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('sites.form.maxConcurrency')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={0}
                  step={1}
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
                {t('sites.form.maxConcurrencyHint')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
      <SiteFormProbeFields form={form} />
    </fieldset>
  )
}
