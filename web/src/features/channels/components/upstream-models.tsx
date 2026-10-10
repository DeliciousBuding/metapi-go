import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Plus, Search, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useUpstreamDeletion } from '@/components/common/upstream-deletion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'
import type {
  ImportedMember,
  ImportedUpstreamDetail,
} from '@/lib/api/imported-upstreams'
import type { UpstreamModel } from '@/lib/api/upstream-catalog'
import { useUndoableDelete } from '@/lib/undoable-delete'

import {
  upstreamKeys,
  upstreamProtocols,
  upstreamProtocolGroups,
} from '../lib/upstream-config'
import {
  UpstreamGrantForm,
  UpstreamModelCreateForm,
  UpstreamModelEditForm,
} from './upstream-model-forms'

type ModelsData = { items: UpstreamModel[] }

export function UpstreamModels(props: {
  detail: ImportedUpstreamDetail
  active: boolean
  members: ImportedMember[]
  onDirtyChange: (key: string, dirty: boolean) => void
  beforeDelete?: (action: () => void) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const deletion = useUpstreamDeletion()
  const undoDelete = useUndoableDelete()
  const query = useQuery({
    queryKey: upstreamKeys.models(props.detail.id),
    queryFn: () => api.getUpstreamModels(props.detail.id),
    enabled: props.active,
  })
  const credentials = useQuery({
    queryKey: upstreamKeys.credentials(props.detail.id),
    queryFn: () => api.getImportedCredentials(props.detail.id),
    enabled: props.active,
  })
  const [search, setSearch] = useState('')
  const [creating, setCreating] = useState(false)
  const [expanded, setExpanded] = useState<number[]>([])
  const [opened, setOpened] = useState<number[]>([])
  const [addingGrants, setAddingGrants] = useState<number[]>([])
  const endpoints = props.detail.endpointConfig ?? {}
  const configured = Object.values(endpoints).some(Boolean)
  const paths: Partial<Record<keyof typeof endpoints, string>> = {
    chat: props.detail.openaiChatCompletionPath,
    responses: props.detail.openaiResponsePath,
    messages: props.detail.anthropicMessagePath,
    gemini: '',
  }
  const availableProtocols = upstreamProtocols
    .filter((p) => (configured ? !!endpoints[p.key] : !!paths[p.key]))
    .map((p) => p.bit)
  const anonymousProtocols = upstreamProtocols
    .filter((p) => endpoints[p.key]?.auth === 'none')
    .map((p) => p.bit)
  async function remove(kind: 'model' | 'grant', id: number, name: string) {
    await deletion.requestDeletion(
      { kind, id, name },
      (_, commit) => {
        undoDelete<ModelsData, number>({
          item: id,
          queryKey: upstreamKeys.models(props.detail.id),
          removeFromCache: (data) => ({
            items:
              kind === 'model'
                ? data.items.filter((model) => model.id !== id)
                : data.items.map((model) => ({
                    ...model,
                    grants: model.grants.filter((grant) => grant.id !== id),
                  })),
          }),
          deleteFn: commit,
          title: t('channels.catalog.deleted', { name }),
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
  function requestRemove(kind: 'model' | 'grant', id: number, name: string) {
    const action = () => {
      void remove(kind, id, name)
    }
    if (props.beforeDelete) props.beforeDelete(action)
    else action()
  }
  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-3'>
        <div className='relative min-w-0 flex-1'>
          <Search className='text-muted-foreground absolute top-2.5 left-3 size-4' />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className='pl-9'
            aria-label={t('channels.catalog.searchModels')}
            placeholder={t('channels.catalog.searchModels')}
          />
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={() => setCreating((value) => !value)}
          aria-expanded={creating}
        >
          <Plus className='size-4' />
          {t('channels.catalog.addModels')}
        </Button>
      </div>
      <div hidden={!creating}>
        <UpstreamModelCreateForm
          channelId={props.detail.id}
          onDirtyChange={props.onDirtyChange}
          onCreated={() => setCreating(false)}
        />
      </div>
      {query.error && (
        <QueryErrorBanner
          error={query.error}
          messageKey='channels.imported.loadError'
          onRetry={() => query.refetch()}
          isRetrying={query.isFetching}
        />
      )}
      {credentials.error && (
        <QueryErrorBanner
          error={credentials.error}
          messageKey='channels.imported.loadError'
          onRetry={() => credentials.refetch()}
          isRetrying={credentials.isFetching}
        />
      )}
      {query.isLoading && (
        <p className='text-muted-foreground py-6 text-center'>
          {t('channels.upstream.loading')}
        </p>
      )}
      {query.data?.items.map((model) => {
        const availableCredentials = (credentials.data?.items ?? []).filter(
          (credential) =>
            !model.grants.some((grant) => grant.credentialId === credential.id)
        )
        const mask = model.grants
          .filter((grant) => grant.enabled)
          .reduce((bits, grant) => bits | grant.protocols, 0)
        return (
          <article
            key={model.id}
            className='overflow-hidden rounded-xl border'
            hidden={
              !model.name.toLowerCase().includes(search.trim().toLowerCase())
            }
          >
            <div className='flex items-center gap-2 p-3'>
              <button
                type='button'
                className='focus-visible:ring-ring flex min-w-0 flex-1 items-center gap-3 rounded-md text-left outline-none focus-visible:ring-2'
                aria-expanded={expanded.includes(model.id)}
                aria-label={t('channels.catalog.editModel', {
                  name: model.name,
                })}
                onClick={() => {
                  setOpened((ids) =>
                    ids.includes(model.id) ? ids : [...ids, model.id]
                  )
                  setExpanded((ids) =>
                    ids.includes(model.id)
                      ? ids.filter((id) => id !== model.id)
                      : [...ids, model.id]
                  )
                }}
              >
                <ChevronDown
                  className={`size-4 shrink-0 transition-transform ${expanded.includes(model.id) ? 'rotate-180' : ''}`}
                />
                <div className='min-w-0 space-y-1.5'>
                  <ModelPill model={model.name} />
                  <p className='text-muted-foreground text-xs'>
                    {t('channels.catalog.grantCount', {
                      count: model.grants.length,
                    })}
                  </p>
                  <div
                    className='flex flex-wrap gap-1.5'
                    aria-label={t('channels.capabilities.authorized')}
                  >
                    {upstreamProtocolGroups.map((group) => {
                      const count = upstreamProtocols.filter(
                        (p) => p.group === group && mask & p.bit
                      ).length
                      return count ? (
                        <Badge key={group} variant='outline'>
                          {t(`channels.capabilities.groups.${group}`)} · {count}
                        </Badge>
                      ) : null
                    })}
                  </div>
                </div>
              </button>
              <Badge variant={model.enabled ? 'success' : 'secondary'}>
                {t(
                  model.enabled
                    ? 'channels.imported.enabled'
                    : 'channels.imported.disabled'
                )}
              </Badge>
              <Button
                type='button'
                size='icon-sm'
                variant='ghost'
                disabled={deletion.isPending}
                aria-label={t('channels.catalog.deleteModel', {
                  name: model.name,
                })}
                onClick={() => requestRemove('model', model.id, model.name)}
              >
                <Trash2 className='size-4' />
              </Button>
            </div>
            {opened.includes(model.id) && (
              <div
                hidden={!expanded.includes(model.id)}
                className='space-y-4 border-t p-4'
              >
                <UpstreamModelEditForm
                  model={model}
                  onDirtyChange={props.onDirtyChange}
                />
                <div className='flex items-center justify-between gap-3 border-t pt-4'>
                  <h3 className='text-sm font-semibold'>
                    {t('channels.catalog.grants')}
                  </h3>
                  <Button
                    size='sm'
                    variant='ghost'
                    disabled={
                      !availableCredentials.length || !availableProtocols.length
                    }
                    aria-expanded={addingGrants.includes(model.id)}
                    onClick={() =>
                      setAddingGrants((ids) =>
                        ids.includes(model.id)
                          ? ids.filter((id) => id !== model.id)
                          : [...ids, model.id]
                      )
                    }
                  >
                    <Plus className='size-4' />
                    {t('channels.catalog.addGrant')}
                  </Button>
                </div>
                <div
                  hidden={
                    !addingGrants.includes(model.id) ||
                    !availableCredentials.length
                  }
                >
                  <UpstreamGrantForm
                    modelId={model.id}
                    credentials={availableCredentials}
                    availableProtocols={availableProtocols}
                    anonymousProtocols={anonymousProtocols}
                    members={props.members}
                    onDirtyChange={props.onDirtyChange}
                    onCreated={() =>
                      setAddingGrants((ids) =>
                        ids.filter((id) => id !== model.id)
                      )
                    }
                  />
                </div>
                {model.grants.map((grant) => (
                  <div key={grant.id} className='space-y-2'>
                    <div className='text-muted-foreground flex items-center justify-between gap-3 text-xs'>
                      <span>
                        {t('channels.catalog.routeCount', {
                          count: grant.memberCount,
                        })}
                      </span>
                      <Button
                        variant='ghost'
                        size='icon-sm'
                        disabled={deletion.isPending}
                        aria-label={t('channels.catalog.deleteGrant', {
                          name: grant.credentialName,
                        })}
                        onClick={() =>
                          requestRemove(
                            'grant',
                            grant.id,
                            `${model.name} · ${grant.credentialName}`
                          )
                        }
                      >
                        <Trash2 className='size-3.5' />
                      </Button>
                    </div>
                    <UpstreamGrantForm
                      modelId={model.id}
                      grant={grant}
                      credentials={credentials.data?.items ?? []}
                      availableProtocols={availableProtocols}
                      anonymousProtocols={anonymousProtocols}
                      members={props.members}
                      onDirtyChange={props.onDirtyChange}
                    />
                  </div>
                ))}
                {!!credentials.data?.items.length &&
                  !availableCredentials.length && (
                    <p className='text-muted-foreground text-sm'>
                      {t('channels.catalog.allCredentialsGranted')}
                    </p>
                  )}
                {!credentials.data?.items.length && !credentials.isFetching && (
                  <p className='text-muted-foreground text-xs'>
                    {t('channels.catalog.credentialsFirst')}
                  </p>
                )}
                {!availableProtocols.length && (
                  <p className='text-muted-foreground text-xs'>
                    {t('channels.catalog.endpointsFirst')}
                  </p>
                )}
              </div>
            )}
          </article>
        )
      })}
      {query.data?.items.length === 0 && (
        <p className='text-muted-foreground py-8 text-center'>
          {t('channels.catalog.noModels')}
        </p>
      )}
      {deletion.dialog}
    </div>
  )
}
