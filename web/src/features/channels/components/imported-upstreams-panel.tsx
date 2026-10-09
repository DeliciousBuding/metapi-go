import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowUpRight,
  Cable,
  KeyRound,
  RefreshCw,
  Search,
  Users,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { memberProtocols, upstreamKeys } from '../lib/upstream-config'
import { UpstreamDetailSheet } from './upstream-detail-sheet'
import { UpstreamIdentity } from './upstream-identity'

export function ImportedUpstreamsPanel(props: {
  children?: ReactNode
  accountCount?: number
  accountsLoading?: boolean
  forceAccounts?: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: upstreamKeys.all,
    queryFn: api.getImportedUpstreams,
  })
  const [selectedTab, setSelectedTab] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [detailId, setDetailId] = useState<number | null>(null)
  const mutation = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      api.setImportedUpstreamEnabled(id, enabled),
    onSuccess: async (result) => {
      if (!result.success) return
      await queryClient.invalidateQueries({ queryKey: upstreamKeys.all })
      toast.success(t('channels.imported.updated'))
    },
  })
  const items = query.data?.items ?? []
  const members = query.data?.members ?? []
  const detail = items.find((item) => item.id === detailId)
  if (!items.length && !query.error) return props.children ?? null
  const tab =
    selectedTab ??
    (!props.forceAccounts &&
    !props.accountsLoading &&
    (props.accountCount ?? 0) === 0
      ? 'upstreams'
      : 'accounts')
  const needle = search.trim().toLowerCase()
  const filtered = items.filter((item) =>
    [
      item.name,
      item.provider,
      item.baseUrl,
      ...members
        .filter((member) => member.channelId === item.id)
        .map((member) => member.modelName),
    ].some((value) => value?.toLowerCase().includes(needle))
  )
  return (
    <>
      <Tabs
        value={tab}
        onValueChange={(value) => setSelectedTab(String(value))}
        className='min-h-0 flex-1 gap-3'
      >
        <TabsList variant='line' className='shrink-0 gap-4 border-b pb-2'>
          <TabsTrigger value='accounts'>
            <Users />
            {t('channels.upstream.accountsTab')}
            <span className='text-muted-foreground text-xs tabular-nums'>
              {props.accountCount ?? 0}
            </span>
          </TabsTrigger>
          <TabsTrigger value='upstreams'>
            <Cable />
            {t('channels.upstream.upstreamsTab')}
            <span className='text-muted-foreground text-xs tabular-nums'>
              {items.length}
            </span>
          </TabsTrigger>
        </TabsList>
        <TabsContent value='accounts' className='flex min-h-0 flex-col gap-3'>
          {props.children}
        </TabsContent>
        <TabsContent value='upstreams' className='flex min-h-0 flex-col gap-3'>
          <div className='flex shrink-0 items-center gap-3'>
            <div className='relative max-w-sm flex-1'>
              <Search
                className='text-muted-foreground absolute top-2.5 left-3 size-4'
                aria-hidden='true'
              />
              <Input
                aria-label={t('channels.upstream.search')}
                placeholder={t('channels.upstream.search')}
                className='pl-9'
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </div>
            <Button
              variant='outline'
              size='icon'
              aria-label={t('channels.imported.refresh')}
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              <RefreshCw className='size-4' />
            </Button>
          </div>
          {query.error && (
            <QueryErrorBanner
              error={query.error}
              messageKey='channels.imported.loadError'
              onRetry={() => query.refetch()}
              isRetrying={query.isFetching}
            />
          )}
          <div
            className='min-h-0 flex-1 overflow-y-auto'
            role='region'
            aria-label={t('channels.imported.region')}
            tabIndex={0}
          >
            <div className='grid gap-3 xl:grid-cols-2'>
              {filtered.map((item) => {
                const bindings = members.filter(
                  (member) => member.channelId === item.id
                )
                const models = [
                  ...new Set(bindings.map((member) => member.modelName)),
                ]
                return (
                  <article
                    key={item.id}
                    className='bg-card hover:border-primary/30 flex flex-col gap-4 rounded-xl border p-4 transition-colors'
                  >
                    <div className='flex items-start justify-between gap-3'>
                      <div className='min-w-0 space-y-2'>
                        <UpstreamIdentity
                          provider={item.provider}
                          dialect={item.dialect}
                        />
                        <h2 className='text-base font-semibold'>
                          <button
                            type='button'
                            className='hover:text-primary focus-visible:outline-ring max-w-full truncate text-left'
                            onClick={() => setDetailId(item.id)}
                          >
                            {item.name}
                          </button>
                        </h2>
                        <p
                          className='text-muted-foreground truncate text-xs'
                          title={item.baseUrl}
                        >
                          {item.baseUrl}
                        </p>
                      </div>
                      <Badge variant={item.enabled ? 'success' : 'secondary'}>
                        {t(
                          item.enabled
                            ? 'channels.imported.enabled'
                            : 'channels.imported.disabled'
                        )}
                      </Badge>
                    </div>
                    <div className='flex flex-wrap items-center gap-2'>
                      {models.slice(0, 3).map((model) => (
                        <ModelPill key={model} model={model} />
                      ))}
                      {models.length > 3 && (
                        <span className='text-muted-foreground text-xs'>
                          +{models.length - 3}
                        </span>
                      )}
                    </div>
                    <div className='text-muted-foreground space-y-1 text-xs'>
                      {[...new Set(bindings.map(memberProtocols))]
                        .slice(0, 2)
                        .map((protocols) => (
                          <p key={protocols}>{protocols}</p>
                        ))}
                    </div>
                    <div className='mt-auto flex flex-wrap items-center justify-between gap-3 border-t pt-3'>
                      <span className='text-muted-foreground flex items-center gap-1.5 text-xs'>
                        <KeyRound className='size-3.5' />
                        {t('channels.imported.counts', {
                          models: item.modelCount,
                          credentials: item.credentialCount,
                        })}
                      </span>
                      <div className='flex items-center gap-1'>
                        <Button
                          size='sm'
                          variant='ghost'
                          disabled={mutation.isPending}
                          aria-label={t(
                            item.enabled
                              ? 'channels.imported.disableChannel'
                              : 'channels.imported.enableChannel',
                            { name: item.name }
                          )}
                          onClick={() =>
                            mutation.mutate({
                              id: item.id,
                              enabled: !item.enabled,
                            })
                          }
                        >
                          {t(
                            item.enabled
                              ? 'channels.imported.disable'
                              : 'channels.imported.enable'
                          )}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setDetailId(item.id)}
                        >
                          {t('channels.upstream.manage')}
                          <ArrowUpRight className='size-3.5' />
                        </Button>
                      </div>
                    </div>
                  </article>
                )
              })}
            </div>
            {!filtered.length && (
              <p className='text-muted-foreground py-12 text-center text-sm'>
                {t('channels.upstream.noMatches')}
              </p>
            )}
          </div>
        </TabsContent>
      </Tabs>
      {detail && (
        <UpstreamDetailSheet
          key={detail.id}
          item={detail}
          members={members.filter((member) => member.channelId === detail.id)}
          onClose={() => setDetailId(null)}
        />
      )}
    </>
  )
}
