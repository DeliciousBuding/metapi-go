import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { AccountFormValues } from '../lib/accounts-schema'
import type { CredentialMode } from '../types'
import { ApiKeyFields } from './account-api-key-fields'
import { useCredentialVerification } from './account-credential-verification'
import { PasswordFields } from './account-password-fields'
import { SessionFields } from './account-session-fields'

export function AccountCredentialFields({
  form,
  open,
  credentialMode,
  showPasswordMode,
}: {
  form: UseFormReturn<AccountFormValues>
  open: boolean
  credentialMode: CredentialMode
  showPasswordMode: boolean
}) {
  const { t } = useTranslation()
  const feedback = useCredentialVerification(form, open, credentialMode)
  return (
    <>
      {/* Credential mode toggle */}
      <FormItem>
        <FormLabel>{t('accounts.form.credentialMode')}</FormLabel>
        <Tabs
          value={credentialMode}
          onValueChange={(value) =>
            form.setValue('credentialMode', value as CredentialMode, {
              shouldDirty: true,
            })
          }
        >
          <TabsList>
            <TabsTrigger value='session'>
              {t('accounts.form.modeSession')}
            </TabsTrigger>
            <TabsTrigger value='apikey'>
              {t('accounts.form.modeApiKey')}
            </TabsTrigger>
            {showPasswordMode && (
              <TabsTrigger value='password'>
                {t('accounts.form.modePassword')}
              </TabsTrigger>
            )}
          </TabsList>
        </Tabs>
      </FormItem>

      {/* Connection name (optional) — hidden in password mode, where
                username is the site login name collected by PasswordFields */}
      {credentialMode !== 'password' && (
        <FormField
          control={form.control}
          name='username'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('accounts.form.connectionName')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('accounts.form.connectionNamePlaceholder')}
                  {...field}
                  value={field.value ?? ''}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}

      {credentialMode === 'session' && (
        <SessionFields form={form} {...feedback} />
      )}
      {credentialMode === 'apikey' && (
        <ApiKeyFields form={form} {...feedback} />
      )}
      {credentialMode === 'password' && <PasswordFields form={form} />}
    </>
  )
}
