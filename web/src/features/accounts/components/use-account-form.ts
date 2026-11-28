import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'

import { useSiteInitializationPresets } from '@/features/sites'
import { getPlatformDefinition } from '@/lib/platform-catalog'

import {
  getAccountFormDefaultValues,
  getAccountFormSchema,
  transformAccountToFormValues,
  type AccountFormValues,
} from '../lib/accounts-schema'
import { findAccountInitializationPreset } from '../lib/initialization-preset'
import type { Account, CredentialMode, Site } from '../types'

export interface AccountFormOptions {
  open: boolean
  mode: 'create' | 'edit'
  account?: Account | null
  sites: Site[]
  initialSiteId?: number
  /** Credential-mode hint from the sites guided-flow deep link
   *  (`segment=apikey`) — applied as the create default only; the operator
   *  can switch tabs freely afterwards. Ignored in edit mode. */
  initialCredentialMode?: CredentialMode
}

export function useAccountForm({
  open,
  mode,
  account,
  sites,
  initialSiteId,
  initialCredentialMode,
}: AccountFormOptions) {
  const presets = useSiteInitializationPresets()
  const isEdit = mode === 'edit' && !!account

  const schema = useMemo(() => getAccountFormSchema(!isEdit), [isEdit])
  const form = useForm<AccountFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getAccountFormDefaultValues(),
  })

  const credentialMode = form.watch('credentialMode') as CredentialMode
  const [initializedFor, setInitializedFor] = useState<string | null>(null)
  const isInitialized = initializedFor !== null

  useEffect(() => {
    if (!open) {
      setInitializedFor(null)
      return
    }
    const targetKey = isEdit && account ? `edit:${account.id}` : 'create'
    if (initializedFor === targetKey) return
    setInitializedFor(targetKey)
    // An explicit deep link wins; direct API sites start in API-key mode.
    const initialSite = sites.find((site) => site.id === initialSiteId)
    const defaultMode =
      getPlatformDefinition(initialSite?.platform)?.group === 'api'
        ? 'apikey'
        : 'session'
    const baseDefaults = getAccountFormDefaultValues(
      isEdit
        ? (account?.credentialMode ?? 'session')
        : (initialCredentialMode ?? defaultMode)
    )
    if (isEdit && account) {
      form.reset({ ...baseDefaults, ...transformAccountToFormValues(account) })
    } else {
      form.reset({
        ...baseDefaults,
        siteId: initialSiteId ?? baseDefaults.siteId,
      })
    }
  }, [
    open,
    isEdit,
    account,
    initializedFor,
    initialSiteId,
    initialCredentialMode,
    sites,
    form,
  ])

  const watchedSiteId = form.watch('siteId')
  const watchedPassword = form.watch('password')
  const watchedUsername = form.watch('username')
  const selectedSite = sites.find((site) => site.id === watchedSiteId)
  const preset = findAccountInitializationPreset(selectedSite, presets.data)
  const apiSite = getPlatformDefinition(selectedSite?.platform)?.group === 'api'
  const passwordDraft =
    !!watchedPassword || (credentialMode === 'password' && !!watchedUsername)
  const showPasswordMode = isEdit || !apiSite || passwordDraft

  useEffect(() => {
    if (
      !open ||
      !isInitialized ||
      isEdit ||
      credentialMode !== 'apikey' ||
      !presets.isSuccess ||
      form.getFieldState('skipModelFetch').isTouched
    ) {
      return
    }
    form.setValue(
      'skipModelFetch',
      preset?.recommendedSkipModelFetch ?? false,
      { shouldDirty: false }
    )
  }, [
    open,
    isInitialized,
    isEdit,
    credentialMode,
    watchedSiteId,
    preset?.recommendedSkipModelFetch,
    presets.isSuccess,
    form,
  ])

  useEffect(() => {
    if (
      open &&
      isInitialized &&
      !showPasswordMode &&
      credentialMode === 'password'
    ) {
      form.setValue('credentialMode', 'apikey')
    }
  }, [open, isInitialized, showPasswordMode, credentialMode, form])

  return { form, isEdit, isInitialized, credentialMode, showPasswordMode }
}
