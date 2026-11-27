import { useQuery } from '@tanstack/react-query'
import { KeyRound } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'
import { useUndoableDelete } from '@/lib/undoable-delete'

import { type PersonalKey, userPortalApi } from '../api'
import type { ModelConnectionContext } from '../connection-context'
import { CreateKeyDialog } from './portal-create-key-dialog'
import { PortalKeyRow } from './portal-key-row'
import { RotateKeyDialog } from './portal-rotate-key-dialog'

export { CopyValue } from './portal-copy-value'

type PortalKeysProps = {
  userId: number
  connectionContext?: ModelConnectionContext
}

export function PortalKeys(props: PortalKeysProps) {
  return (
    <OpenPortalKeys
      key={JSON.stringify([props.userId, props.connectionContext])}
      {...props}
    />
  )
}

function OpenPortalKeys({ userId, connectionContext }: PortalKeysProps) {
  const { t } = useTranslation()
  const queryKey = ['userPortal', userId, 'keys'] as const
  const query = useQuery({ queryKey, queryFn: userPortalApi.keys })
  const remove = useUndoableDelete()
  const [rotating, setRotating] = useState<PersonalKey | null>(null)

  function revokeKey(key: PersonalKey) {
    remove<{ items: PersonalKey[] }, PersonalKey>({
      item: key,
      queryKey,
      removeFromCache: (data, item) => ({
        items: data.items.filter((v) => v.id !== item.id),
      }),
      deleteFn: (item) => userPortalApi.deleteKey(item.id),
      title: t('userPortal.keyRemoved'),
      undoLabel: t('common.undo'),
      errorTitle: t('userPortal.actionFailed'),
      alsoInvalidate: [['userPortal', userId, 'usage']],
    })
  }

  function renderKeysList() {
    if (query.isPending) {
      return <Spinner aria-label={t('common.loading')} />
    }
    if (query.isError) {
      return (
        <Notice tone='destructive' role='alert'>
          <span>{t('userPortal.loadFailed')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void query.refetch()}
          >
            {t('common.retry')}
          </Button>
        </Notice>
      )
    }
    if (query.data.items.length === 0) {
      return (
        <Card>
          <CardContent className='text-muted-foreground flex items-center gap-3 py-4'>
            <KeyRound aria-hidden='true' className='size-4 shrink-0' />
            <span>{t('userPortal.noKeys')}</span>
          </CardContent>
        </Card>
      )
    }
    return (
      <div className='bg-card divide-border divide-y rounded-xl border'>
        {query.data.items.map((item) => (
          <PortalKeyRow
            key={item.id}
            item={item}
            onRotate={setRotating}
            onRevoke={revokeKey}
          />
        ))}
      </div>
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.keysHint')}
        </p>
        <div className='flex flex-wrap items-center gap-2'>
          <QuietRefreshStatus
            updatedAt={query.dataUpdatedAt}
            hasError={query.isError}
          />
          <CreateKeyDialog
            userId={userId}
            connectionContext={connectionContext}
          />
        </div>
      </div>
      {renderKeysList()}
      {rotating && (
        <RotateKeyDialog
          item={rotating}
          userId={userId}
          onClose={() => setRotating(null)}
        />
      )}
    </div>
  )
}
