import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Spinner } from '@/components/ui/spinner'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { useUndoableDelete } from '@/lib/undoable-delete'

import { accountQueryKeys } from '../../api'
import type { AccountToken } from '../../types'
import {
  accountTokenQueryKeys,
  useAccountTokens,
  useSetDefaultAccountToken,
  useToggleAccountTokenEnabled,
} from '../api'
import { AccountTokenRow } from './account-token-row'

export function AccountTokenList({
  accountId,
  onEdit,
  onConfigureRoutes,
}: {
  accountId: number
  onEdit: (token: AccountToken) => void
  onConfigureRoutes?: (tokenId: number) => void
}) {
  const { t } = useTranslation()
  const {
    data: tokens = [],
    isLoading,
    error,
    refetch,
    isFetching,
  } = useAccountTokens(accountId)
  const setDefaultMutation = useSetDefaultAccountToken()
  const toggleEnabledMutation = useToggleAccountTokenEnabled()

  // Delete-with-undo tier: token delete is leaf — no confirm dialog; the row
  // leaves immediately and a 6s undo toast gates the real DELETE.
  const undoableDelete = useUndoableDelete()
  const deleteToken = (token: AccountToken) =>
    undoableDelete<AccountToken[], AccountToken>({
      item: token,
      queryKey: accountTokenQueryKeys.list(accountId),
      removeFromCache: (data, target) =>
        data.filter((entry) => entry.id !== target.id),
      deleteFn: async (target) => {
        const result = await api.deleteAccountToken(target.id)
        assertBusinessOk(result, 'accounts.tokens.toast.deleteFailed')
      },
      title: t('accounts.tokens.toast.deleted'),
      undoLabel: t('common.undo'),
      errorTitle: t('accounts.tokens.toast.deleteFailed'),
      alsoInvalidate: [accountTokenQueryKeys.all, accountQueryKeys.all],
    })

  return (
    <>
      <QueryErrorBanner
        error={error}
        messageKey='accounts.tokens.loadError'
        onRetry={() => void refetch()}
        isRetrying={isFetching}
      />

      {isLoading && (
        <div className='text-muted-foreground flex items-center justify-center py-6 text-sm'>
          <Spinner />
          {t('accounts.tokens.loading')}
        </div>
      )}
      {!isLoading && !error && tokens.length === 0 && (
        <p className='text-muted-foreground py-6 text-center text-sm'>
          {t('accounts.tokens.empty')}
        </p>
      )}
      {!isLoading && tokens.length > 0 && (
        <ul className='flex flex-col divide-y rounded-lg border'>
          {tokens.map((token) => (
            <AccountTokenRow
              key={token.id}
              token={token}
              onConfigureRoutes={
                onConfigureRoutes && !error
                  ? () => onConfigureRoutes(token.id)
                  : undefined
              }
              onEdit={() => onEdit(token)}
              onDelete={() => deleteToken(token)}
              onSetDefault={() => setDefaultMutation.mutate(token.id)}
              onToggleEnabled={(enabled) =>
                toggleEnabledMutation.mutate({ id: token.id, enabled })
              }
              isToggling={toggleEnabledMutation.isPending}
            />
          ))}
        </ul>
      )}
    </>
  )
}
