import { useTranslation } from 'react-i18next'

import { useReauthReplay } from '@/components/common/reauth-provider'
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
import { toast } from '@/lib/toast'

import { useCreateSite, useUpdateSite } from '../api'
import { analyzePrimarySiteUrl } from '../lib/site-primary-url'
import type { SiteFormValues } from '../lib/sites-schema'
import type { Site } from '../types'
import { SiteFormAdvancedFields } from './site-form-advanced-fields'
import { SiteFormConnectionFields } from './site-form-connection-fields'
import { matchesTemplate, buildPayload } from './site-form-values'
import { useSiteConnectionForm } from './use-site-connection-form'

type SiteFormSheetProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  editingSite: Site | null
  onCreated?: (site: Site) => void
}

export function SiteFormSheet({
  open,
  onOpenChange,
  editingSite,
  onCreated,
}: SiteFormSheetProps) {
  const { t } = useTranslation()
  const isEditing = editingSite !== null

  const connection = useSiteConnectionForm(open, editingSite)
  const { form, selectedTemplate } = connection
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onDiscard: () => form.reset(),
    onOpenChange,
  })

  const createSite = useCreateSite()
  const updateSite = useUpdateSite()
  const handleReauth = useReauthReplay()
  const isSubmitting = createSite.isPending || updateSite.isPending

  async function onSubmit(values: SiteFormValues) {
    const endpointsTouched = form.getFieldState('apiEndpointsText').isDirty
    // Primary URL normalization guidance: a known API request suffix is
    // stripped to the site root on save (mirrors the TS original). Other
    // extra paths are preserved verbatim — the live alerts told the operator.
    const analysis = analyzePrimarySiteUrl(values.url)
    const persistedUrl =
      analysis.action === 'auto_strip_known_api_suffix' && analysis.persistedUrl
        ? analysis.persistedUrl
        : values.url
    const urlWasNormalized = persistedUrl !== values.url.trim()
    const payload = buildPayload(
      values,
      editingSite,
      endpointsTouched,
      persistedUrl
    )
    if (
      !isEditing &&
      selectedTemplate?.group === 'api' &&
      matchesTemplate(selectedTemplate, persistedUrl, values.platform)
    ) {
      payload.initializationPresetId = selectedTemplate.id
    }
    const finishSave = (created?: Site) => {
      toast.success(
        t(
          isEditing
            ? 'sites.form.updateSucceeded'
            : 'sites.form.createSucceeded',
          { name: values.name }
        )
      )
      if (urlWasNormalized) {
        toast.info(t('sites.form.urlNormalizedToast', { url: persistedUrl }))
      }
      form.reset()
      onOpenChange(false)
      if (!isEditing && created) {
        onCreated?.(created)
      }
    }
    const save = async (confirmToken?: string) => {
      if (isEditing && editingSite) {
        await updateSite.mutateAsync({
          id: editingSite.id,
          payload,
          confirmToken,
        })
        finishSave()
        return
      }
      const created = await createSite.mutateAsync({ payload, confirmToken })
      finishSave(created)
    }
    try {
      await save()
    } catch (error) {
      // Setting a per-site proxy redirects stored upstream credentials; the
      // backend rejects it with reauthRequired until the master token is
      // re-entered. Replay the same form values once through the dialog.
      handleReauth(error, t('reauth.operations.siteProxy'), (confirmToken) =>
        save(confirmToken)
      )
    }
  }

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent
        className='gap-0 overflow-hidden sm:max-w-2xl'
        showMobileCloseBar={false}
      >
        <SheetHeader className='shrink-0 border-b px-4 py-5 pr-12 sm:px-6'>
          <SheetTitle>
            {isEditing ? t('sites.form.editTitle') : t('sites.form.addTitle')}
          </SheetTitle>
          <SheetDescription className='sr-only'>
            {isEditing
              ? t('sites.form.editDescription')
              : t('sites.form.addDescription')}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit, () =>
              toast.error(t('sites.form.invalid'))
            )}
            className='flex min-h-0 flex-1 flex-col'
          >
            <div
              data-slot='site-form-body'
              className='min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-6 sm:px-6'
            >
              <SiteFormConnectionFields
                connection={connection}
                editingSite={editingSite}
              />
              <SiteFormAdvancedFields form={form} />
            </div>
            <SheetFooter className='shrink-0 flex-row justify-end gap-2 border-t px-4 py-4 sm:px-6'>
              <Button
                type='button'
                variant='outline'
                onClick={() => handleOpenChange(false)}
                disabled={isSubmitting}
              >
                {t('sites.form.cancel')}
              </Button>
              <Button type='submit' disabled={isSubmitting}>
                {isSubmitting && <Spinner />}
                {isEditing ? t('sites.form.save') : t('sites.form.create')}
              </Button>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
      {guard}
    </Sheet>
  )
}
