// metapi-go/components/form — dirty-close guard for modal form dialogs/sheets.
//
// Modal Dialog/Sheet overlays block sidebar navigation while open, so the
// router-level FormNavigationGuard (useBlocker) does not apply here. The only
// leave paths are the dialog's own close affordances (X / Escape / Cancel),
// which all funnel through `onOpenChange(false)`. This hook intercepts that
// call when the form is dirty and shows the shared ConfirmDialog instead of
// silently discarding the user's edits.
//
// Usage in a form dialog:
//   const form = useForm(...)
//   const { handleOpenChange, guard } = useDirtyDialogClose({
//     enabled: form.formState.isDirty,
//     onDiscard: () => form.reset(),
//     onOpenChange,
//   })
//   <Dialog open={open} onOpenChange={handleOpenChange}> ... {guard}
//
// Successful submits must reset the form before closing (form.reset() then
// onOpenChange(false)) so the post-save close is not treated as a discard.

import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'

type DirtyDialogCloseOptions = {
  /** True while the form holds unsaved changes (RHF `formState.isDirty`). */
  enabled: boolean
  /** Called after the user confirms discarding — typically `form.reset()`. */
  onDiscard?: () => void
  /** The dialog/sheet's own `onOpenChange` (parent owns `open`). */
  onOpenChange: (open: boolean) => void
}

export function useDirtyDialogClose({
  enabled,
  onDiscard,
  onOpenChange,
}: DirtyDialogCloseOptions) {
  const { t } = useTranslation()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (next) {
        onOpenChange(true)
        return
      }
      if (!enabled) {
        onOpenChange(false)
        return
      }
      setConfirmOpen(true)
    },
    [enabled, onOpenChange],
  )

  const guard = (
    <ConfirmDialog
      open={confirmOpen}
      title={t('settings.common.unsavedTitle')}
      description={t('settings.common.unsavedDescription')}
      confirmLabel={t('settings.common.discardChanges')}
      cancelLabel={t('settings.common.keepEditing')}
      destructive
      onConfirm={() => {
        setConfirmOpen(false)
        onDiscard?.()
        onOpenChange(false)
      }}
      onCancel={() => setConfirmOpen(false)}
    />
  )

  return { handleOpenChange, guard }
}