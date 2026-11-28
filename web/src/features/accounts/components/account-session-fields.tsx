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
import { Textarea } from '@/components/ui/textarea'

import {
  VerifyCredentialFeedback,
  type VerifiedCredentialFieldsProps,
} from './account-credential-verification'

export function SessionFields({
  form,
  verification,
  onVerify,
}: VerifiedCredentialFieldsProps) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='accessToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.accessToken')}</FormLabel>
            <FormControl>
              <Textarea
                rows={4}
                placeholder={t('accounts.formSession.accessTokenPlaceholder')}
                className='font-mono text-xs'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>
              {t('accounts.formSession.accessTokenHint')}
            </FormDescription>
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
        name='platformUserId'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.platformUserId')}</FormLabel>
            <FormControl>
              <Input
                type='number'
                placeholder={t(
                  'accounts.formSession.platformUserIdPlaceholder'
                )}
                value={field.value ?? ''}
                onChange={(event) =>
                  field.onChange(
                    event.target.value === ''
                      ? undefined
                      : Number(event.target.value)
                  )
                }
                onBlur={field.onBlur}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name='refreshToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.refreshToken')}</FormLabel>
            <FormControl>
              <Input
                className='font-mono text-xs'
                placeholder={t('accounts.formSession.refreshTokenPlaceholder')}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name='tokenExpiresAt'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formSession.tokenExpiresAt')}</FormLabel>
            <FormControl>
              <Input
                type='number'
                placeholder={t(
                  'accounts.formSession.tokenExpiresAtPlaceholder'
                )}
                value={field.value ?? ''}
                onChange={(event) =>
                  field.onChange(
                    event.target.value === ''
                      ? undefined
                      : Number(event.target.value)
                  )
                }
                onBlur={field.onBlur}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}
