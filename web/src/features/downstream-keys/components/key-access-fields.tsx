import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { useSites } from '@/features/sites'

import { AccessPolicyEditor } from './access-policy-editor'
import { CredentialRefPicker } from './credential-ref-picker'
import type { CreateKeyFormValues } from './key-form-shared'
import { ModelPolicyEditor } from './model-policy-editor'
import { SiteScopePicker } from './site-scope-picker'

export function KeyScopeFields({
  candidateModels,
  routeGrantCount,
}: {
  candidateModels: string[]
  routeGrantCount: number
}) {
  const { t } = useTranslation()
  const form = useFormContext<CreateKeyFormValues>()
  const { data: sites } = useSites()
  return (
    <>
      <FormField
        control={form.control}
        name='supportedModels'
        render={({ field }) => (
          <FormItem data-key-scope='supportedModels'>
            <FormLabel>
              {t('settings.downstream.keys.models.fieldLabel', {
                defaultValue: 'Model access',
              })}
            </FormLabel>
            <FormDescription>
              {t('settings.downstream.keys.models.hint', {
                defaultValue:
                  'Empty denies all. Add exact names, glob patterns (*), or re: regex rules. All models requires an explicit *.',
              })}
            </FormDescription>
            <FormControl>
              <ModelPolicyEditor
                value={field.value ?? []}
                onChange={field.onChange}
                candidateModels={candidateModels}
                routeGrantCount={routeGrantCount}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name='allowedSiteIds'
        render={({ field }) => (
          <FormItem data-key-scope='allowedSiteIds'>
            <FormLabel>
              {t('settings.downstream.keys.sites.fieldLabel', {
                defaultValue: 'Upstream site restriction',
              })}
            </FormLabel>
            <FormDescription>
              {t('settings.downstream.keys.sites.hint', {
                defaultValue:
                  'Leave empty for no site restriction. Selecting sites limits this key to channels of the chosen upstream sites.',
              })}
            </FormDescription>
            <FormControl>
              <SiteScopePicker
                value={field.value ?? []}
                onChange={field.onChange}
                sites={sites ?? []}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name='allowedCredentialRefs'
        render={({ field }) => (
          <FormItem data-key-scope='allowedCredentialRefs'>
            <FormLabel>
              {t('settings.downstream.keys.credentials.fieldLabelAllow')}
            </FormLabel>
            <FormDescription>
              {t('settings.downstream.keys.credentials.hintAllow')}
            </FormDescription>
            <FormControl>
              <CredentialRefPicker
                value={field.value ?? []}
                onChange={field.onChange}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name='excludedCredentialRefs'
        render={({ field }) => (
          <FormItem data-key-scope='excludedCredentialRefs'>
            <FormLabel>
              {t('settings.downstream.keys.credentials.fieldLabelExclude')}
            </FormLabel>
            <FormDescription>
              {t('settings.downstream.keys.credentials.hintExclude')}
            </FormDescription>
            <FormControl>
              <CredentialRefPicker
                value={field.value ?? []}
                onChange={field.onChange}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}

export function KeyAccessPolicyField({
  candidateModels,
}: {
  candidateModels: string[]
}) {
  const { t } = useTranslation()
  const form = useFormContext<CreateKeyFormValues>()
  return (
    <FormField
      control={form.control}
      name='accessPolicy'
      render={({ field }) => (
        <FormItem data-key-scope='accessPolicy' className='border-t pt-4'>
          <FormLabel>
            {t('settings.downstream.keys.fields.accessPolicy')}
          </FormLabel>
          <FormDescription>
            {t('settings.downstream.keys.access.hint')}
          </FormDescription>
          <FormControl>
            <AccessPolicyEditor {...field} candidateModels={candidateModels} />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
