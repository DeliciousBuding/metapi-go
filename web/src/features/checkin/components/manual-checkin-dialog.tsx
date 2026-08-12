// metapi-go features/checkin/components — manual checkin trigger dialog.
// i18n: all user-visible strings migrated to t() calls.

import { Loader2, Zap } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useAccounts } from '@/features/accounts'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

import { useCheckinAccount } from '../api'

interface ManualCheckinDialogProps { open: boolean; onOpenChange: (open: boolean) => void }
const NO_SELECTION = '__none'

export function ManualCheckinDialog({ open, onOpenChange }: ManualCheckinDialogProps) {
  const { t } = useTranslation()
  const { data: snapshot } = useAccounts()
  const accounts = (snapshot?.accounts ?? []).filter((account) => account.capabilities?.canCheckin === true)
  const triggerMutation = useCheckinAccount()
  const [selectedId, setSelectedId] = useState<string>(NO_SELECTION)

  const handleTrigger = async () => {
    if (selectedId === NO_SELECTION) return
    const accountId = Number(selectedId)
    try {
      const result = await triggerMutation.mutateAsync(accountId)
      if (result.status === 'success') {
        toast.success(t('checkin.toast.success'), { description: result.reward ? t('checkin.toast.successReward', { reward: result.reward }) : undefined })
      } else if (result.status === 'skipped' || result.skipped) {
        toast.info(t('checkin.toast.skipped'), { description: result.message || undefined })
      } else {
        toast.error(t('checkin.toast.failed'), { description: result.message || undefined })
      }
      onOpenChange(false)
      setSelectedId(NO_SELECTION)
    } catch { }
  }

  const isSubmitting = triggerMutation.isPending

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) setSelectedId(NO_SELECTION); onOpenChange(next) }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('checkin.manual.title')}</DialogTitle>
          <DialogDescription>{t('checkin.manual.description')}</DialogDescription>
        </DialogHeader>
        <div className='space-y-3 py-2'>
          <Select value={selectedId} onValueChange={(value) => setSelectedId(value ?? NO_SELECTION)}>
            <SelectTrigger>
              <SelectValue>
                {(selected) => {
                  if (!selected || selected === NO_SELECTION) return t('checkin.manual.accountPlaceholder')
                  const account = accounts.find((item) => String(item.id) === selected)
                  if (!account) return String(selected)
                  const suffix = account.site?.name ? ` · ${account.site.name}` : ''
                  return `${account.username || `#${account.id}`}${suffix}`
                }}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {accounts.length === 0 && <SelectItem value={NO_SELECTION} disabled>{t('checkin.manual.accountEmpty')}</SelectItem>}
              {accounts.map((account) => (
                <SelectItem key={account.id} value={String(account.id)}>
                  {account.username || `#${account.id}`}
                  {account.site?.name ? ` · ${account.site.name}` : ''}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)} disabled={isSubmitting}>{t('common.cancel')}</Button>
          <Button onClick={handleTrigger} disabled={isSubmitting || selectedId === NO_SELECTION}>
            {isSubmitting && <Loader2 className='animate-spin' />}
            <Zap />
            {t('checkin.manual.trigger')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
