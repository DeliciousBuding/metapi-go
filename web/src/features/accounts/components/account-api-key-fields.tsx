import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  VerifyCredentialFeedback,
  type VerifiedCredentialFieldsProps,
} from './account-credential-verification'

export function ApiKeyFields({
  form,
  verification,
  onVerify,
}: VerifiedCredentialFieldsProps) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='apiToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formApiKey.apiKey')}</FormLabel>
            <FormControl>
              <Textarea
                rows={3}
                placeholder={t('accounts.formApiKey.apiKeyPlaceholder')}
                className='font-mono text-xs'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <VerifyCredentialFeedback
        verification={verification}
        onVerify={onVerify}
      />

      <FormField
        control={form.control}
        name='skipModelFetch'
        render={({ field }) => (
          <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
            <div className='space-y-0.5'>
              <FormLabel>{t('accounts.formApiKey.skipModelFetch')}</FormLabel>
              <FormDescription>
                {t('accounts.formApiKey.skipModelFetchHint')}
              </FormDescription>
            </div>
            <FormControl>
              <Switch
                checked={field.value}
                onCheckedChange={(checked) => {
                  field.onChange(checked)
                  field.onBlur() // Remember explicit choices even when toggled back to the default.
                }}
              />
            </FormControl>
          </FormItem>
        )}
      />
    </>
  )
}
