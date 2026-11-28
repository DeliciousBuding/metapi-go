import { Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'

import type { AccountToken } from '../../types'
import { useSyncAccountTokens } from '../api'
import { AccountTokenForm } from './account-token-form'
import { AccountTokenList } from './account-token-list'

interface TokensPanelProps {
  accountId: number
  onConfigureRoutes?: (tokenId: number) => void
  /**
   * Reports the inline token form's dirty state so the hosting account
   * detail sheet can confirm before it closes over unsaved edits.
   */
  onFormDirtyChange?: (dirty: boolean) => void
}

export function TokensPanel({
  accountId,
  onConfigureRoutes,
  onFormDirtyChange,
}: TokensPanelProps) {
  const { t } = useTranslation()
  const syncMutation = useSyncAccountTokens()
  const [editor, setEditor] = useState<'create' | AccountToken | null>(null)

  const handleSync = async () => {
    try {
      await syncMutation.mutateAsync(accountId)
    } catch {
      // http-client toasted
    }
  }

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex items-center justify-between'>
        <h3 className='text-sm font-medium'>{t('accounts.tokens.title')}</h3>
        <div className='flex items-center gap-1'>
          <Button
            variant='outline'
            size='xs'
            onClick={handleSync}
            disabled={syncMutation.isPending}
          >
            {syncMutation.isPending ? (
              <Spinner className='size-3' />
            ) : (
              <RefreshCw />
            )}
            {t('accounts.tokens.syncFromSite')}
          </Button>
          <Button size='xs' onClick={() => setEditor('create')}>
            <Plus />
            {t('accounts.tokens.add')}
          </Button>
        </div>
      </div>

      {editor !== null && (
        <AccountTokenForm
          accountId={accountId}
          token={editor === 'create' ? null : editor}
          onClose={() => setEditor(null)}
          onDirtyChange={onFormDirtyChange}
        />
      )}

      <Separator />

      <AccountTokenList
        accountId={accountId}
        onEdit={setEditor}
        onConfigureRoutes={onConfigureRoutes}
      />
    </div>
  )
}
