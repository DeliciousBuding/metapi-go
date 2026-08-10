// metapi-go features/checkin/components — manual checkin trigger dialog.
//
// A modal Dialog (not a Sheet) for picking a single account and triggering
// its checkin immediately. The account list is sourced from the accounts
// feature's snapshot, filtered to accounts whose site supports checkin
// (capabilities.canCheckin). The trigger-one response carries a per-account
// `status` (`success` / `failed` / `skipped`), so the dialog inspects it to
// choose the right toast and closes on completion.

import { Loader2, Zap } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import { useAccounts } from '@/features/accounts'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { useCheckinAccount } from '../api'

interface ManualCheckinDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const NO_SELECTION = '__none'

export function ManualCheckinDialog({
  open,
  onOpenChange,
}: ManualCheckinDialogProps) {
  const { data: snapshot } = useAccounts()
  const accounts = (snapshot?.accounts ?? []).filter(
    (account) => account.capabilities?.canCheckin === true,
  )

  const triggerMutation = useCheckinAccount()
  const [selectedId, setSelectedId] = useState<string>(NO_SELECTION)

  const handleTrigger = async () => {
    if (selectedId === NO_SELECTION) return
    const accountId = Number(selectedId)
    try {
      const result = await triggerMutation.mutateAsync(accountId)
      if (result.status === 'success') {
        toast.success('签到成功', {
          description: result.reward ? `奖励：${result.reward}` : undefined,
        })
      } else if (result.status === 'skipped' || result.skipped) {
        toast.info('签到已跳过', {
          description: result.message || undefined,
        })
      } else {
        toast.error('签到失败', {
          description: result.message || undefined,
        })
      }
      onOpenChange(false)
      setSelectedId(NO_SELECTION)
    } catch {
      // http-client already toasted the network/business error.
    }
  }

  const isSubmitting = triggerMutation.isPending

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setSelectedId(NO_SELECTION)
        onOpenChange(next)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>手动签到</DialogTitle>
          <DialogDescription>
            选择一个支持签到的账号，立即触发一次签到。
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-3 py-2'>
          <Select
            value={selectedId}
            onValueChange={(value) => setSelectedId(value ?? NO_SELECTION)}
          >
            <SelectTrigger>
              <SelectValue placeholder='选择账号' />
            </SelectTrigger>
            <SelectContent>
              {accounts.length === 0 && (
                <SelectItem value={NO_SELECTION} disabled>
                  暂无支持签到的账号
                </SelectItem>
              )}
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
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isSubmitting}
          >
            取消
          </Button>
          <Button
            onClick={handleTrigger}
            disabled={isSubmitting || selectedId === NO_SELECTION}
          >
            {isSubmitting && <Loader2 className='animate-spin' />}
            <Zap />
            立即签到
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
