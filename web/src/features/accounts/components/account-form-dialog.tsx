import { useTranslation } from 'react-i18next'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'

import { AccountCredentialFields } from './account-credential-fields'
import { AccountParameterFields } from './account-parameter-fields'
import { AccountSiteField } from './account-site-field'
import { useAccountForm, type AccountFormOptions } from './use-account-form'
import { useAccountFormSubmit } from './use-account-form-submit'

interface AccountFormDialogProps extends AccountFormOptions {
  onOpenChange: (open: boolean) => void
}

export function AccountFormDialog(props: AccountFormDialogProps) {
  const { open, onOpenChange, account, sites, initialCredentialMode } = props
  const { t } = useTranslation()
  const { form, isEdit, isInitialized, credentialMode, showPasswordMode } =
    useAccountForm(props)
  const { onSubmit, onInvalid, isSubmitting } = useAccountFormSubmit(
    form,
    isEdit,
    account,
    onOpenChange
  )
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onDiscard: () => form.reset(),
    onOpenChange,
  })

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 overflow-hidden sm:max-w-lg'
        showMobileCloseBar={false}
      >
        <SheetHeader className='shrink-0 border-b'>
          <SheetTitle>
            {isEdit
              ? t('accounts.form.editTitle')
              : t('accounts.form.addTitle')}
          </SheetTitle>
          <SheetDescription>
            {isEdit
              ? t('accounts.form.editDescription')
              : t('accounts.form.addDescription')}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='account-form'
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            inert={!isInitialized ? true : undefined}
            aria-busy={!isInitialized}
            className='min-h-0 flex-1 space-y-6 overflow-y-auto p-4 text-sm leading-5 [&_[data-slot=form-description]]:leading-5 [&_[data-slot=form-label]]:font-semibold'
          >
            <AccountSiteField
              form={form}
              sites={sites}
              open={open}
              inferCredentialMode={!isEdit && !initialCredentialMode}
            />
            <AccountCredentialFields
              form={form}
              open={open}
              credentialMode={credentialMode}
              showPasswordMode={showPasswordMode}
            />
            {/* Account-parameter fields (status / check-in / unit cost / proxy
                / tags) only apply to session or api-key credentials: the
                password login-bind endpoint force-sets status + check-in and
                ignores the rest, so editing them in password mode would be a
                silent no-op. Hide them instead of showing
                controls that do nothing. */}
            {credentialMode !== 'password' && (
              <AccountParameterFields form={form} />
            )}
          </form>
        </Form>

        <SheetFooter>
          <Button
            variant='outline'
            onClick={() => handleOpenChange(false)}
            disabled={isSubmitting}
          >
            {t('common.cancel')}
          </Button>
          <Button
            type='submit'
            form='account-form'
            disabled={isSubmitting || !isInitialized}
          >
            {isSubmitting && <Spinner />}
            {isEdit ? t('accounts.form.save') : t('accounts.form.create')}
          </Button>
        </SheetFooter>
      </SheetContent>
      {guard}
    </Sheet>
  )
}
