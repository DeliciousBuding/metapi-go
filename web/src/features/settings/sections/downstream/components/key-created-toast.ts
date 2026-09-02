// metapi-go/features/settings/sections/downstream/components — the guided
// "connect now" toast shown after a downstream API key is created. This is the
// LAST step of the site → account → route → key guided configuration chain:
// the key exists, and the operator needs the credential surface to actually
// hand it to a client.
//
// Sibling of accounts/account-created-toast.tsx and
// token-routes/route-completion-toast.tsx — same shape (module-level helper,
// i18n.t resolved at call time so the copy follows the active locale, toast
// infra from lib/toast).
//
// The Connect dialog already auto-opens on create. The toast action is the way
// BACK in: the dialog is dismissible, and #1034 keeps it locked behind a
// master-token re-confirm, so an operator who closes it (or who came back to
// this tab later) needs a one-click reopen instead of hunting the row's
// Connect button in the table.

import type { CredentialExportTarget } from '@/components/common/credential-export-dialog'
import i18n from '@/i18n/config'
import { toast } from '@/lib/toast'

/**
 * Fire the post-create guided toast. Call from the key form's success handler
 * with the created row and the host's dialog-open callback.
 *
 * `onConnect` receives the same target back, so the host reuses its existing
 * open-the-export-dialog path instead of this module reaching into UI state.
 */
export function showKeyCreatedToast(
  target: CredentialExportTarget,
  onConnect: (target: CredentialExportTarget) => void
): void {
  toast.success(i18n.t('settings.downstream.keys.toast.created'), {
    description: i18n.t('settings.downstream.keys.toast.createdConnectHint'),
    // Same 8s tier as the other two guided-chain toasts: long enough to read
    // the next step and reach the action, short enough not to outstay it.
    duration: 8000,
    action: {
      label: i18n.t('settings.downstream.keys.toast.connectAction'),
      onClick: () => onConnect(target),
    },
  })
}
