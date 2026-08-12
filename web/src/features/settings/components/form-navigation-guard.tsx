// metapi-go/features/settings/components — unsaved-changes navigation guard.
// TanStack Router `useBlocker` (resolver form) intercepts in-app navigation
// when a section form is dirty; `enableBeforeUnload` covers tab close/refresh
// with the browser's native prompt. Confirmation is rendered through the
// shared ConfirmDialog so copy stays bilingual.

import { useBlocker } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'

type FormNavigationGuardProps = {
  enabled: boolean
  /** Optional callback run after the user confirms discarding changes. */
  onDiscard?: () => void
}

export function FormNavigationGuard({
  enabled,
  onDiscard,
}: FormNavigationGuardProps) {
  const { t } = useTranslation()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const blocker = useBlocker({
    shouldBlockFn: () => enabled,
    withResolver: true,
    enableBeforeUnload: () => enabled,
    disabled: !enabled,
  })

  useEffect(() => {
    if (blocker.status === 'blocked') {
      setConfirmOpen(true)
    }
  }, [blocker.status])

  function handleStay() {
    setConfirmOpen(false)
    blocker.reset?.()
  }

  function handleDiscard() {
    setConfirmOpen(false)
    blocker.proceed?.()
    onDiscard?.()
  }

  return (
    <ConfirmDialog
      open={confirmOpen}
      title={t('settings.common.unsavedTitle')}
      description={t('settings.common.unsavedDescription')}
      confirmLabel={t('settings.common.discardChanges')}
      cancelLabel={t('settings.common.keepEditing')}
      destructive
      onConfirm={handleDiscard}
      onCancel={handleStay}
    />
  )
}
