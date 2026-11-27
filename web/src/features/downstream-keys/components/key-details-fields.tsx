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
import { Switch } from '@/components/ui/switch'

import {
  generateDownstreamSkSuffix,
  type CreateKeyFormValues,
} from './key-form-shared'

export function KeyIdentityFields({ isEdit }: { isEdit: boolean }) {
  const { t } = useTranslation()
  const form = useFormContext<CreateKeyFormValues>()
  function generateKey() {
    form.setValue('key', `sk-${generateDownstreamSkSuffix()}`, {
      shouldDirty: true,
    })
  }

  return (
    <>
      <FormField
        control={form.control}
        name='name'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('settings.downstream.keys.fields.name')}</FormLabel>
            <FormControl>
              <Input {...field} value={field.value ?? ''} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      {isEdit ? null : (
        <FormField
          control={form.control}
          name='key'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('settings.downstream.keys.fields.key')}</FormLabel>
              <div className='flex gap-2'>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    className='font-mono'
                    placeholder='sk-…'
                  />
                </FormControl>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={generateKey}
                >
                  {t('settings.downstream.keys.generate')}
                </Button>
              </div>
              <FormDescription>
                {t('settings.downstream.keys.fields.keyHint')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
    </>
  )
}

export function KeyLimitFields({ isEdit }: { isEdit: boolean }) {
  const { t } = useTranslation()
  const form = useFormContext<CreateKeyFormValues>()
  return (
    <>
      <FormField
        control={form.control}
        name='groupName'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('settings.downstream.keys.fields.groupName')}
            </FormLabel>
            <FormControl>
              <Input {...field} value={field.value ?? ''} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <div className='grid grid-cols-2 gap-4'>
        <FormField
          control={form.control}
          name='maxRequests'
          render={({ field }) => (
            <FormItem data-key-scope='maxRequests'>
              <FormLabel>
                {t('settings.downstream.keys.fields.maxRequests')}
              </FormLabel>
              <FormDescription>
                {t('settings.downstream.keys.fields.maxRequestsHint')}
              </FormDescription>
              <FormControl>
                <Input
                  {...field}
                  value={field.value ?? ''}
                  type='number'
                  min={0}
                  placeholder={t('settings.common.unlimited')}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='maxCost'
          render={({ field }) => (
            <FormItem data-key-scope='maxCost'>
              <FormLabel>
                {t('settings.downstream.keys.fields.maxCost')}
              </FormLabel>
              <FormDescription>
                {t('settings.downstream.keys.fields.maxCostHint')}
              </FormDescription>
              <FormControl>
                <Input
                  {...field}
                  value={field.value ?? ''}
                  type='number'
                  min={0}
                  step={0.01}
                  placeholder={t('settings.common.unlimited')}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
      <FormField
        control={form.control}
        name='expiresAt'
        render={({ field }) => (
          <FormItem data-key-scope='expiresAt'>
            <FormLabel>
              {t('settings.downstream.keys.fields.expiresAt')}
            </FormLabel>
            <FormDescription>
              {t('settings.downstream.keys.fields.expiresAtHint')}
            </FormDescription>
            <FormControl>
              <Input
                {...field}
                value={field.value ?? ''}
                type='datetime-local'
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name='enabled'
        render={({ field }) => (
          <FormItem
            data-key-scope='enabled'
            className='flex flex-row items-center gap-3'
          >
            <FormControl>
              <Switch
                checked={Boolean(field.value)}
                onCheckedChange={field.onChange}
              />
            </FormControl>
            <FormLabel className='cursor-pointer'>
              {t('settings.downstream.keys.fields.enabled')}
            </FormLabel>
          </FormItem>
        )}
      />
      {isEdit ? null : (
        <FormField
          control={form.control}
          name='description'
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t('settings.downstream.keys.fields.description')}
              </FormLabel>
              <FormControl>
                <Input {...field} value={field.value ?? ''} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
    </>
  )
}
