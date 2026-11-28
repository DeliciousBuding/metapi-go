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

import type { AccountFormValues } from '../lib/accounts-schema'

// Password-mode fields — username+password sign-in, bound via the separate
// POST /api/accounts/login endpoint (the backend fetches the session token
// and stores autoRelogin config on the account).
export function PasswordFields({
  form,
}: {
  form: UseFormReturn<AccountFormValues>
}) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='username'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formPassword.username')}</FormLabel>
            <FormControl>
              <Input
                autoComplete='username'
                placeholder={t('accounts.formPassword.usernamePlaceholder')}
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
        name='password'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.formPassword.password')}</FormLabel>
            <FormControl>
              <Input
                type='password'
                autoComplete='new-password'
                placeholder={t('accounts.formPassword.passwordPlaceholder')}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>{t('accounts.formPassword.hint')}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}
