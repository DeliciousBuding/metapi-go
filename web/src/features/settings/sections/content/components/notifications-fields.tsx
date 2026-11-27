import { useFormContext, type FieldPathByValue } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormDescription,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import type {
  NotifyFormValues,
  SecretFieldName,
} from './notifications-form-values'

export function NotificationToggle({
  name,
}: {
  name: FieldPathByValue<NotifyFormValues, boolean>
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<NotifyFormValues>()
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem className='flex flex-row items-center gap-3'>
          <FormControl>
            <Checkbox
              checked={Boolean(field.value)}
              onCheckedChange={field.onChange}
            />
          </FormControl>
          <FormLabel className='cursor-pointer'>
            {t(`settings.content.notifications.fields.${name}`)}
          </FormLabel>
        </FormItem>
      )}
    />
  )
}

export function NotificationText({
  name,
  url,
  placeholder,
}: {
  name: Exclude<
    FieldPathByValue<NotifyFormValues, string | undefined>,
    SecretFieldName
  >
  url?: boolean
  placeholder?: string
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<NotifyFormValues>()
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>
            {t(`settings.content.notifications.fields.${name}`)}
          </FormLabel>
          <FormControl>
            <Input
              {...field}
              value={field.value ?? ''}
              placeholder={placeholder}
              className={url ? 'font-mono' : undefined}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function NotificationSecret({
  name,
  masked,
}: {
  name: SecretFieldName
  masked: string
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<NotifyFormValues>()
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>
            {t(`settings.content.notifications.fields.${name}`)}
          </FormLabel>
          <FormControl>
            <Input
              {...field}
              value={field.value ?? ''}
              type='password'
              placeholder={t(
                'settings.content.notifications.secretPlaceholder'
              )}
            />
          </FormControl>
          {masked ? (
            <FormDescription>
              {t('settings.content.notifications.masked', { masked })}
            </FormDescription>
          ) : null}
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
