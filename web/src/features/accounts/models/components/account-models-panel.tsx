// metapi-go features/accounts/models/components — account Models panel
// (#998, Wave 12). Embedded in the account detail sheet, it shows the
// persisted model availability for one account with honest state:
//   - manual upstream refresh (POST /api/models/check/{id} — the existing
//     refresh owner; no periodic scheduler in this wave);
//   - manual add (creates pinned manual rows);
//   - explicit removal, offered only on manual rows — auto rows belong to
//     the refresh owner;
//   - source (Manual/Auto) + availability (Unavailable) + site-disabled
//     badges, last-checked time, and real loading/error/empty states.
// All copy uses t(key, { defaultValue }) so missing locale keys (integration
// owns the locale JSON) still render readable text.

import { RefreshCw, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { toBcp47 } from '@/i18n/languages'
import { formatAbsoluteDateTime, formatRelativeTime } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  useAccountModels,
  useRefreshAccountModels,
  useUpdateManualModels,
} from '../api'

interface AccountModelsPanelProps {
  accountId: number
}

function describeError(error: unknown): string {
  if (error instanceof Error && error.message.trim() !== '') {
    return error.message
  }
  return 'Unknown error'
}

export function AccountModelsPanel({ accountId }: AccountModelsPanelProps) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const { data, isLoading, isError, isFetching, refetch } =
    useAccountModels(accountId)
  const refreshMutation = useRefreshAccountModels(accountId)
  const manualMutation = useUpdateManualModels(accountId)

  const [newModel, setNewModel] = useState('')

  const models = data?.models ?? []

  const handleRefresh = () => {
    refreshMutation.mutate()
  }

  const handleAdd = (event: FormEvent) => {
    event.preventDefault()
    const name = newModel.trim()
    if (name === '' || manualMutation.isPending) return
    manualMutation.mutate(
      { models: [name] },
      {
        onSuccess: () => setNewModel(''),
      }
    )
  }

  const handleRemove = (name: string) => {
    manualMutation.mutate({ remove: [name] })
  }

  return (
    <section className='flex flex-col gap-2'>
      <div className='flex items-center justify-between gap-2'>
        <h3 className='text-sm font-medium'>
          {t('accounts.models.title', { defaultValue: 'Models' })}
          {data ? ` (${data.totalCount ?? models.length})` : ''}
        </h3>
        <Button
          variant='outline'
          size='xs'
          onClick={handleRefresh}
          disabled={refreshMutation.isPending}
        >
          {refreshMutation.isPending ? (
            <Spinner className='size-3' />
          ) : (
            <RefreshCw />
          )}
          {t('accounts.models.refresh', {
            defaultValue: 'Refresh from upstream',
          })}
        </Button>
      </div>

      {isLoading && (
        <div className='text-muted-foreground flex items-center gap-2 text-xs'>
          <Spinner className='size-3' />
          {t('accounts.models.loading', { defaultValue: 'Loading models…' })}
        </div>
      )}

      {!isLoading && isError && (
        <div className='flex flex-col gap-1 text-xs'>
          <p className='text-destructive' role='alert'>
            {t('accounts.models.loadFailed', {
              defaultValue: 'Failed to load models',
            })}
          </p>
          <div>
            <Button
              variant='outline'
              size='xs'
              onClick={() => void refetch()}
              disabled={isFetching}
            >
              {t('accounts.models.retry', { defaultValue: 'Retry' })}
            </Button>
          </div>
        </div>
      )}

      {!isLoading && !isError && models.length === 0 && (
        <p className='text-muted-foreground text-xs'>
          {t('accounts.models.empty', {
            defaultValue:
              'No models yet — refresh from upstream or add one manually.',
          })}
        </p>
      )}

      {!isLoading && !isError && models.length > 0 && (
        <ul className='flex flex-col gap-1'>
          {models.map((model) => {
            const checkedTitle = model.checkedAt
              ? formatAbsoluteDateTime(model.checkedAt, locale) || undefined
              : undefined
            return (
              <li
                key={model.name}
                className={cn(
                  'flex items-center gap-2 rounded-md border px-2 py-1 text-xs',
                  !model.available && 'opacity-60'
                )}
              >
                <span className='min-w-0 flex-1 truncate' title={model.name}>
                  {model.name}
                </span>
                {model.isManual && (
                  <Badge variant='secondary'>
                    {t('accounts.models.manualBadge', {
                      defaultValue: 'Manual',
                    })}
                  </Badge>
                )}
                {!model.available && (
                  <Badge variant='outline'>
                    {t('accounts.models.unavailableBadge', {
                      defaultValue: 'Unavailable',
                    })}
                  </Badge>
                )}
                {model.disabled && (
                  <Badge variant='outline'>
                    {t('accounts.models.disabledBadge', {
                      defaultValue: 'Disabled',
                    })}
                  </Badge>
                )}
                {model.checkedAt && (
                  <span
                    className='text-muted-foreground shrink-0'
                    title={checkedTitle}
                  >
                    {formatRelativeTime(model.checkedAt, locale)}
                  </span>
                )}
                {model.isManual && (
                  <Button
                    variant='ghost'
                    size='xs'
                    aria-label={t('accounts.models.removeAria', {
                      name: model.name,
                      defaultValue: 'Remove {{name}}',
                    })}
                    onClick={() => handleRemove(model.name)}
                    disabled={manualMutation.isPending}
                  >
                    <Trash2 />
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      )}

      {refreshMutation.isError && (
        <p className='text-destructive text-xs' role='alert'>
          {t('accounts.models.refreshFailedInline', {
            message: describeError(refreshMutation.error),
            defaultValue: 'Refresh failed: {{message}}',
          })}
        </p>
      )}
      {manualMutation.isError && (
        <p className='text-destructive text-xs' role='alert'>
          {t('accounts.models.manualFailedInline', {
            message: describeError(manualMutation.error),
            defaultValue: 'Manual models update failed: {{message}}',
          })}
        </p>
      )}

      <form onSubmit={handleAdd} className='flex gap-1'>
        <Input
          value={newModel}
          onChange={(event) => setNewModel(event.target.value)}
          placeholder={t('accounts.models.placeholder', {
            defaultValue: 'Model name, e.g. gpt-4o',
          })}
          aria-label={t('accounts.models.inputAria', {
            defaultValue: 'New manual model name',
          })}
        />
        <Button
          type='submit'
          variant='outline'
          size='sm'
          disabled={newModel.trim() === '' || manualMutation.isPending}
        >
          {manualMutation.isPending ? <Spinner className='size-3' /> : null}
          {t('accounts.models.add', { defaultValue: 'Add' })}
        </Button>
      </form>
    </section>
  )
}
