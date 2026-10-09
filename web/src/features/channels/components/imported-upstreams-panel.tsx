import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

const queryKey = ['imported-upstreams'] as const
const protocolNames: Record<string, string> = {
  chat: 'Chat',
  responses: 'Responses',
  messages: 'Messages',
  gemini: 'Gemini',
}

export function ImportedUpstreamsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({ queryKey, queryFn: api.getImportedUpstreams })
  const mutation = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      api.setImportedUpstreamEnabled(id, enabled),
    onSuccess: async (result) => {
      if (!result.success) {
        throw new Error('Imported upstream update failed')
      }
      await queryClient.invalidateQueries({ queryKey })
      toast.success(t('channels.imported.updated'))
    },
    onError: () => toast.error(t('channels.imported.updateError')),
  })
  if (query.error) {
    return (
      <QueryErrorBanner
        error={query.error}
        messageKey='channels.imported.loadError'
        onRetry={() => query.refetch()}
        isRetrying={query.isFetching}
      />
    )
  }
  if (!query.data?.items.length) {
    return null
  }
  return (
    <details className='border-border shrink-0 rounded-lg border' open>
      <summary className='cursor-pointer px-4 py-3 text-sm font-medium'>
        {t('channels.imported.title', { count: query.data.items.length })}
      </summary>
      <div
        className='max-h-96 space-y-3 overflow-y-auto px-4 pb-4'
        tabIndex={0}
        role='region'
        aria-label={t('channels.imported.region')}
      >
        <div className='flex items-start justify-between gap-3'>
          <p className='text-muted-foreground text-xs'>
            {t('channels.imported.description')}
          </p>
          <Button
            size='sm'
            variant='ghost'
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {t('channels.imported.refresh')}
          </Button>
        </div>
        <div className='grid gap-3 lg:grid-cols-2'>
          {query.data.items.map((item) => (
            <article
              key={item.id}
              className='border-border rounded-md border p-3'
            >
              <div className='flex items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <h3 className='text-sm font-medium'>{item.name}</h3>
                  <p className='text-muted-foreground mt-1 text-xs break-all'>
                    {item.baseUrl}
                  </p>
                </div>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={mutation.isPending}
                  aria-label={t(
                    item.enabled
                      ? 'channels.imported.disableChannel'
                      : 'channels.imported.enableChannel',
                    { name: item.name }
                  )}
                  onClick={() =>
                    mutation.mutate({ id: item.id, enabled: !item.enabled })
                  }
                >
                  {t(
                    item.enabled
                      ? 'channels.imported.disable'
                      : 'channels.imported.enable'
                  )}
                </Button>
              </div>
              <div className='mt-2 flex flex-wrap items-center gap-2 text-xs'>
                <Badge variant={item.enabled ? 'success' : 'secondary'}>
                  {t(
                    item.enabled
                      ? 'channels.imported.enabled'
                      : 'channels.imported.disabled'
                  )}
                </Badge>
                <span className='text-muted-foreground'>
                  {item.originKey} · {item.dialect} ·{' '}
                  {t('channels.imported.counts', {
                    models: item.modelCount,
                    credentials: item.credentialCount,
                  })}
                </span>
              </div>
              {Object.keys(item.endpointConfig ?? {}).length > 0 && (
                <details className='mt-3 text-xs'>
                  <summary className='text-muted-foreground cursor-pointer'>
                    {t('channels.imported.endpoints')}
                  </summary>
                  <dl className='mt-2 space-y-2'>
                    {Object.entries(item.endpointConfig ?? {}).map(
                      ([protocol, endpoint]) => (
                        <div key={protocol}>
                          <dt className='font-medium'>
                            {protocolNames[protocol] ?? protocol}
                            <span className='text-muted-foreground ml-2 font-normal'>
                              {endpoint.auth === 'bearer'
                                ? 'Bearer'
                                : endpoint.auth}
                            </span>
                          </dt>
                          <dd className='text-muted-foreground mt-0.5 break-all'>
                            {endpoint.url}
                          </dd>
                        </div>
                      )
                    )}
                  </dl>
                </details>
              )}
              <ul className='mt-3 space-y-2'>
                {query.data.members
                  .filter((member) => member.channelId === item.id)
                  .map((member) => (
                    <li
                      key={member.id}
                      className='border-border flex flex-wrap items-center gap-2 border-t pt-2 text-xs'
                    >
                      <Link
                        to='/token-routes'
                        search={{ routeId: member.routeId }}
                        className='text-primary hover:underline'
                      >
                        {member.groupName}
                      </Link>
                      <ModelPill model={member.modelName} />
                      <span className='text-muted-foreground'>
                        {member.credentialName}
                      </span>
                      {member.cooldownUntil &&
                      Date.parse(member.cooldownUntil) > Date.now() ? (
                        <Badge
                          variant='warning'
                          title={member.cooldownReasonCode || undefined}
                        >
                          {t('channels.imported.coolingDown')}
                        </Badge>
                      ) : null}
                      <span className='text-muted-foreground'>
                        {[
                          member.protocols & 2 ? 'Chat' : '',
                          member.protocols & 4 ? 'Responses' : '',
                          member.protocols & 8 ? 'Messages' : '',
                          member.protocols & 16 ? 'Gemini' : '',
                        ]
                          .filter(Boolean)
                          .join(' / ')}
                      </span>
                    </li>
                  ))}
              </ul>
            </article>
          ))}
        </div>
      </div>
    </details>
  )
}
