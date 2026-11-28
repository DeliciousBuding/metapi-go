import { useQuery } from '@tanstack/react-query'
import { Eye, EyeOff } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

import { type PersonalKey, userPortalApi } from '../api'
import type { ModelConnectionContext } from '../connection-context'
import { connectionExample } from '../connection-example'
import { CopyValue } from './portal-copy-value'

function CreatedKeyConnection({
  userId,
  context,
  item,
}: {
  userId: number
  context: ModelConnectionContext
  item: PersonalKey
}) {
  const { t } = useTranslation()
  // Reuse the catalog from the model page when available. A fresh read is
  // explicit, and remains separate from a charged protocol request.
  const query = useQuery({
    queryKey: ['userPortal', userId, 'models'],
    queryFn: userPortalApi.models,
    enabled: false,
    retry: false,
  })
  const model =
    !query.isError && !query.isFetching
      ? query.data?.items.find((value) => value.id === context.model)
      : undefined
  const protocol = model?.protocols.find(
    (value) => context.protocol === undefined || context.protocol === value
  )
  function renderDetails() {
    if (query.isFetching)
      return (
        <p role='status' className='text-muted-foreground text-sm'>
          {t('common.loading')}
        </p>
      )
    if (query.isError)
      return (
        <Notice tone='destructive' role='alert'>
          {t('userPortal.loadFailed')}
        </Notice>
      )
    if (!query.data)
      return (
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.connection.loadDetails')}
        </p>
      )
    if (!model)
      return (
        <Notice tone='warning'>
          {t('userPortal.catalog.selectedUnavailable', {
            model: context.model,
          })}
        </Notice>
      )
    if (!protocol)
      return (
        <Notice tone='warning'>
          {context.protocol
            ? t('userPortal.connection.protocolUnavailable', {
                model: model.id,
                protocol: context.protocol,
              })
            : t('userPortal.catalog.noExample')}
        </Notice>
      )
    const example = connectionExample(
      window.location.origin,
      model.id,
      protocol
    )
    return (
      <>
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.catalog.protocol')}: {protocol}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.catalog.environmentKey')}
        </p>
        {item.supportedModels.length > 0 &&
          !item.supportedModels.includes('*') && (
            <Notice tone='info'>
              {t('userPortal.connection.keyLimits', {
                models: item.supportedModels.join(', '),
              })}
            </Notice>
          )}
        <pre
          className='bg-muted max-h-72 overflow-auto rounded-lg border p-3 text-xs'
          tabIndex={0}
          aria-label={t('userPortal.catalog.example')}
        >
          <code>{example}</code>
        </pre>
        <CopyValue
          value={example}
          label={t('userPortal.catalog.copyExample')}
        />
        <p className='text-muted-foreground text-xs'>
          {t('userPortal.catalog.verifyHint')}
        </p>
      </>
    )
  }
  return (
    <div className='min-w-0 space-y-3 border-t pt-4'>
      <h3 className='font-medium'>{t('userPortal.catalog.firstCall')}</h3>
      {context.invalid ? (
        <Notice tone='warning'>
          {t('userPortal.connection.invalidContext')}
        </Notice>
      ) : (
        <>
          <p className='text-sm wrap-anywhere'>
            {t('userPortal.selectedConnectionModel', { model: context.model })}
          </p>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {query.isFetching && <Spinner />}
            {t('userPortal.connection.refreshDetails')}
          </Button>
          {renderDetails()}
        </>
      )}
    </div>
  )
}

export function CreatedKey({
  secret,
  onDismiss,
  connection,
}: {
  secret: string
  onDismiss: () => void
  connection?: {
    userId: number
    context: ModelConnectionContext
    item: PersonalKey
  }
}) {
  const { t } = useTranslation()
  const [visible, setVisible] = useState(false)
  const endpoint = window.location.origin + '/v1'
  return (
    <div className='grid gap-4'>
      <Notice tone='warning' size='compact'>
        {t('userPortal.oneTimeKey')}
      </Notice>
      <div className='bg-muted/40 space-y-3 rounded-lg border p-3'>
        <div className='space-y-2'>
          <Label htmlFor='created-personal-key' className='text-xs'>
            {t('userPortal.secretKey')}
          </Label>
          <div className='flex gap-2'>
            <Input
              id='created-personal-key'
              type={visible ? 'text' : 'password'}
              readOnly
              value={secret}
              className='bg-background font-mono text-xs'
            />
            <Button
              type='button'
              variant='outline'
              size='icon'
              aria-label={t(
                visible ? 'userPortal.hideKey' : 'userPortal.showKey'
              )}
              onClick={() => setVisible(!visible)}
            >
              {visible ? <EyeOff /> : <Eye />}
            </Button>
          </div>
          <CopyValue value={secret} label={t('userPortal.copyKey')} />
        </div>
        <div className='space-y-2 border-t pt-3'>
          <Label htmlFor='personal-endpoint' className='text-xs'>
            {t('userPortal.endpoint')}
          </Label>
          <Input
            id='personal-endpoint'
            readOnly
            value={endpoint}
            className='bg-background font-mono text-xs'
          />
          <CopyValue value={endpoint} label={t('userPortal.copyEndpoint')} />
        </div>
      </div>
      <p className='text-muted-foreground text-xs leading-relaxed'>
        {t('userPortal.connectHint')}
      </p>
      {connection && <CreatedKeyConnection {...connection} />}
      <Button onClick={onDismiss}>{t('userPortal.savedKey')}</Button>
    </div>
  )
}
