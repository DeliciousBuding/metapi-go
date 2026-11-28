import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronUp, Plus, Search } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useUpstreamDeletion } from '@/components/common/upstream-deletion'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import type { ImportedCredential } from '@/lib/api/imported-upstreams'
import { useUndoableDelete } from '@/lib/undoable-delete'

import { credentialKindLabel, upstreamKeys } from '../lib/upstream-config'
import { CredentialForm } from './upstream-credential-form'
import { CredentialRow } from './upstream-credential-row'

export function UpstreamCredentials(props: {
  id: number
  active: boolean
  allowAnonymous?: boolean
  onDirtyChange: (key: string, dirty: boolean) => void
  beforeDelete?: (action: () => void) => void
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const deletion = useUpstreamDeletion()
  const undoDelete = useUndoableDelete()
  const query = useQuery({
    queryKey: upstreamKeys.credentials(props.id),
    queryFn: () => api.getImportedCredentials(props.id),
    enabled: props.active,
  })
  const [editors, setEditors] = useState<Partial<Record<number, boolean>>>({})
  const [search, setSearch] = useState('')
  const [limit, setLimit] = useState(5)
  const searchRef = useRef<HTMLInputElement>(null)
  const items = query.data?.items ?? []
  const locale = toBcp47(i18n.language)
  const searchTerm = search.trim().toLocaleLowerCase(locale)
  const matching = items.filter((item) =>
    `${item.name} ${item.id} ${credentialKindLabel(item.kind)}`
      .toLocaleLowerCase(locale)
      .includes(searchTerm)
  )
  const visibleIds = new Set(matching.slice(0, limit).map((item) => item.id))
  const rows = items.filter(
    (item) => visibleIds.has(item.id) || editors[item.id] !== undefined
  )
  const [creating, setCreating] = useState(false)
  const [creationStarted, setCreationStarted] = useState(false)
  async function remove(credential: ImportedCredential) {
    await deletion.requestDeletion(
      { kind: 'credential', id: credential.id, name: credential.name },
      (_, commit) => {
        undoDelete<{ items: ImportedCredential[] }, ImportedCredential>({
          item: credential,
          queryKey: upstreamKeys.credentials(props.id),
          removeFromCache: (data, item) => ({
            items: data.items.filter((row) => row.id !== item.id),
          }),
          deleteFn: commit,
          title: t('channels.catalog.deleted', { name: credential.name }),
          undoLabel: t('common.undo'),
          errorTitle: t('channels.catalog.deleteError'),
          alsoInvalidate: [upstreamKeys.all, ['routes']],
        })
      },
      () => {
        void client.invalidateQueries({ queryKey: upstreamKeys.all })
      }
    )
  }
  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <span
          role='status'
          className='text-muted-foreground text-xs tabular-nums'
        >
          {query.data &&
            t('channels.upstream.credentialCount', {
              shown: Math.min(limit, matching.length),
              total: items.length,
            })}
        </span>
        <Button
          variant='outline'
          size='sm'
          onClick={() => {
            setCreationStarted(true)
            setCreating((value) => !value)
          }}
          aria-expanded={creating}
        >
          <Plus className='size-4' />
          {t('channels.catalog.addCredential')}
        </Button>
      </div>
      <div hidden={!creating} className='overflow-hidden rounded-xl border'>
        {creationStarted && (
          <CredentialForm
            key={props.id}
            channelId={props.id}
            allowAnonymous={props.allowAnonymous}
            onDirtyChange={props.onDirtyChange}
            onCreated={() => {
              setCreating(false)
              setCreationStarted(false)
            }}
          />
        )}
      </div>
      {query.error && (
        <QueryErrorBanner
          error={query.error}
          messageKey='channels.imported.loadError'
          onRetry={() => query.refetch()}
          isRetrying={query.isFetching}
        />
      )}
      {query.isLoading && (
        <p className='text-muted-foreground py-6 text-center'>
          {t('channels.upstream.loading')}
        </p>
      )}
      {items.length > 5 && limit > 5 && (
        <div className='relative'>
          <Search
            aria-hidden='true'
            className='text-muted-foreground pointer-events-none absolute top-2 left-3 size-4'
          />
          <Input
            ref={searchRef}
            className='h-8 pl-9'
            aria-label={t('channels.upstream.searchCredentials')}
            placeholder={t('channels.upstream.searchCredentials')}
            value={search}
            onChange={(event) => {
              setSearch(event.target.value)
              setLimit(20)
            }}
          />
        </div>
      )}
      <div
        hidden={visibleIds.size === 0}
        className='overflow-hidden rounded-xl border'
      >
        {rows.map((credential) => (
          <CredentialRow
            key={credential.id}
            credential={credential}
            hidden={!visibleIds.has(credential.id)}
            expanded={editors[credential.id]}
            deleting={deletion.isPending}
            onToggle={() =>
              setEditors((current) => ({
                ...current,
                [credential.id]: !current[credential.id],
              }))
            }
            onDelete={() => {
              const action = () => {
                void remove(credential)
              }
              if (props.beforeDelete) props.beforeDelete(action)
              else action()
            }}
            onDirtyChange={props.onDirtyChange}
          />
        ))}
      </div>
      {items.length > 0 && matching.length === 0 && (
        <p className='text-muted-foreground py-6 text-center text-sm'>
          {t('common.noResults')}
        </p>
      )}
      {matching.length > limit && (
        <Button
          variant='ghost'
          size='sm'
          className='w-full'
          onClick={() => {
            setLimit((value) => (value === 5 ? 20 : value + 20))
            if (limit === 5)
              requestAnimationFrame(() =>
                searchRef.current?.focus({ preventScroll: true })
              )
          }}
        >
          <ChevronDown aria-hidden='true' />
          {t('channels.upstream.showMoreCredentials', {
            count: matching.length - limit,
          })}
        </Button>
      )}
      {limit > 5 && items.length > 5 && (
        <Button
          variant='ghost'
          size='sm'
          className='w-full'
          onClick={() => {
            setSearch('')
            setLimit(5)
          }}
        >
          <ChevronUp aria-hidden='true' />
          {t('channels.upstream.collapseCredentials')}
        </Button>
      )}
      {query.data?.items.length === 0 && (
        <p className='text-muted-foreground py-8 text-center'>
          {t('channels.upstream.noCredentials')}
        </p>
      )}
      {deletion.dialog}
    </div>
  )
}
