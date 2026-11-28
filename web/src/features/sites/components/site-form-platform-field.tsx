import type { Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import type { SiteFormValues } from '../lib/sites-schema'
import { SitePlatformPicker } from './site-platform-picker'

export function SiteFormPlatformField({
  control,
  platformMode,
  setPlatformMode,
}: {
  control: Control<SiteFormValues>
  platformMode: 'select' | 'custom'
  setPlatformMode: (mode: 'select' | 'custom') => void
}) {
  const { t } = useTranslation()
  return (
    <FormField
      control={control}
      name='platform'
      render={({ field }) => {
        return (
          <FormItem>
            <FormLabel>{t('sites.form.platform')}</FormLabel>
            {platformMode === 'select' ? (
              <>
                <FormControl>
                  <SitePlatformPicker
                    value={field.value}
                    onValueChange={field.onChange}
                  />
                </FormControl>
                <div className='flex items-center justify-between gap-2'>
                  <Button
                    type='button'
                    variant='link'
                    size='xs'
                    onClick={() => setPlatformMode('custom')}
                  >
                    {t('sites.form.platformCustomToggle')}
                  </Button>
                </div>
              </>
            ) : (
              <>
                <FormControl>
                  <Input
                    placeholder={t('sites.form.platformPlaceholder')}
                    {...field}
                  />
                </FormControl>
                <div className='flex items-center justify-between gap-2'>
                  <Button
                    type='button'
                    variant='link'
                    size='xs'
                    onClick={() => setPlatformMode('select')}
                  >
                    {t('sites.form.platformSelectToggle')}
                  </Button>
                </div>
              </>
            )}
            <FormMessage />
          </FormItem>
        )
      }}
    />
  )
}
