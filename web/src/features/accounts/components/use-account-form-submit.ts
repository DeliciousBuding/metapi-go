import type { SubmitErrorHandler, UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useReauthReplay } from '@/components/common/reauth-provider'
import { toast } from '@/lib/toast'

import {
  resolveCreatedAccountId,
  useCreateAccount,
  useLoginAccount,
  useUpdateAccount,
} from '../api'
import {
  transformFormToPayload,
  type AccountFormValues,
} from '../lib/accounts-schema'
import type { Account } from '../types'
import {
  showAccountCreatedToast,
  showAccountLoginToast,
} from './account-created-toast'

export function useAccountFormSubmit(
  form: UseFormReturn<AccountFormValues>,
  isEdit: boolean,
  account: Account | null | undefined,
  onOpenChange: (open: boolean) => void
) {
  const { t } = useTranslation()
  const createMutation = useCreateAccount()
  const updateMutation = useUpdateAccount()
  const handleReauth = useReauthReplay()
  const loginMutation = useLoginAccount()
  const onSubmit = async (values: AccountFormValues) => {
    if (values.credentialMode === 'password') {
      try {
        const result = await loginMutation.mutateAsync({
          siteId: values.siteId,
          username: values.username?.trim() ?? '',
          password: values.password ?? '',
        })
        showAccountLoginToast(
          result?.account?.id,
          values.siteId,
          result?.reusedAccount,
          {
            tokenCount: result?.tokenCount,
            tokenSyncStatus: result?.tokenSyncStatus,
            tokenSyncMessage: result?.tokenSyncMessage,
          }
        )
        form.reset()
        onOpenChange(false)
      } catch {
        // http-client already toasted the business/network error.
      }
      return
    }

    const payload = transformFormToPayload(values, isEdit ? 'update' : 'create')
    if (!payload) {
      toast.error(t('accounts.toast.loginFailed'))
      return
    }
    const save = async (confirmToken?: string) => {
      if (isEdit && account) {
        await updateMutation.mutateAsync({
          id: account.id,
          payload,
          confirmToken,
        })
      } else {
        const result = await createMutation.mutateAsync({
          payload,
          confirmToken,
        })
        showAccountCreatedToast(
          resolveCreatedAccountId(result),
          values.siteId,
          {
            tokenCount: result?.tokenCount,
            tokenSyncStatus: result?.tokenSyncStatus,
            tokenSyncMessage: result?.tokenSyncMessage,
          }
        )
      }
      form.reset()
      onOpenChange(false)
    }
    try {
      await save()
    } catch (error) {
      // A per-account proxy redirects the stored upstream credential; the
      // backend answers reauthRequired until the master token is re-entered.
      handleReauth(error, t('reauth.operations.accountProxy'), (confirmToken) =>
        save(confirmToken)
      )
    }
  }

  const onInvalid: SubmitErrorHandler<AccountFormValues> = () => {
    toast.error(t('accounts.form.invalid'))
  }

  const isSubmitting =
    createMutation.isPending ||
    updateMutation.isPending ||
    loginMutation.isPending

  return { onSubmit, onInvalid, isSubmitting }
}
