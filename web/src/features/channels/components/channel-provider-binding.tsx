import { useQuery } from '@tanstack/react-query'
import { useId, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import { formLabelIdFor } from '@/components/ui/form'
import {
  channelPluginQueryKey,
  channelPluginsApi,
  type ProviderBinding,
} from '@/lib/api/channel-plugins'
import type {
  ImportedCredential,
  ImportedEndpointConfig,
} from '@/lib/api/imported-upstreams'

import { ProviderCatalogFields } from './channel-provider-catalog-fields'
import { ProviderImageQueries } from './channel-provider-image-queries'
import { ProviderProtocolFields } from './channel-provider-protocol-fields'
import { ProviderSelector } from './channel-provider-selector'

type Props = Omit<ComponentProps<'div'>, 'onChange'> & {
  value: ProviderBinding | null
  onChange: (value: ProviderBinding | null) => void
  protocols: readonly (keyof ImportedEndpointConfig)[]
  credentialKind?: ImportedCredential['kind']
  newChannel?: boolean
  disabled?: boolean
}

// A provider is one independently bound protocol adapter, not another entry in
// the ordered wire-hook chain. Missing/stale bindings remain explicit drafts.
export function ChannelProviderBinding({
  value,
  onChange,
  protocols,
  credentialKind,
  newChannel,
  disabled,
  ...restProps
}: Props) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: channelPluginQueryKey,
    queryFn: channelPluginsApi.list,
  })
  const id = useId()
  const blocked = disabled || query.isPending || query.isError
  const items = (query.data?.items ?? []).filter(
    (item) => item.manifest?.apiVersion === 2
  )
  const current = items.find((item) => item.key === value?.key)
  const manifest =
    current?.manifest?.apiVersion === 2 ? current.manifest : undefined
  const missing = !!value && !current
  const incompatibleCredential =
    !!manifest &&
    credentialKind !== undefined &&
    !manifest.credentialKinds.some((kind) => kind === credentialKind)
  function update(patch: Partial<ProviderBinding>) {
    if (!value) return
    const next = { ...value, ...patch }
    if (patch.protocols && next.imageQueries) {
      const queries = { ...next.imageQueries }
      for (const protocol of [
        'imageGeneration',
        'imageEdit',
        'imageVariation',
        'modelscopeImageGeneration',
      ] as const) {
        if (!next.protocols.includes(protocol)) delete queries[protocol]
      }
      next.imageQueries = Object.keys(queries).length ? queries : undefined
    }
    onChange(next)
  }
  return (
    <div
      {...restProps}
      tabIndex={-1}
      role='group'
      aria-label={
        restProps.id || restProps['aria-labelledby']
          ? undefined
          : t('channels.plugins.providerBinding')
      }
      aria-labelledby={
        restProps['aria-labelledby'] ??
        (restProps.id ? formLabelIdFor(restProps.id) : undefined)
      }
      className='space-y-3'
    >
      <QueryErrorBanner
        error={query.error}
        messageKey='channels.plugins.loadError'
        onRetry={() => void query.refetch()}
        isRetrying={query.isFetching}
      />
      {query.isPending && <p role='status'>{t('common.loading')}</p>}
      <ProviderSelector
        id={id}
        value={value}
        current={current}
        items={items}
        protocols={protocols}
        credentialKind={credentialKind}
        blocked={blocked}
        onChange={onChange}
      />
      {!query.isError && !query.isPending && items.length === 0 && !value && (
        <p className='text-muted-foreground text-sm'>
          {t('channels.plugins.emptyProviders')}
        </p>
      )}
      {value && (
        <section className='space-y-3 border-t pt-3'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <span className='text-sm font-medium'>
              {manifest?.name ?? value.key}
            </span>
            <Button
              type='button'
              size='xs'
              variant='outline'
              disabled={blocked}
              onClick={() => onChange(null)}
            >
              {t('channels.plugins.clearProvider')}
            </Button>
          </div>
          {!query.isPending &&
            !query.isError &&
            (missing || current?.loadError || !current?.enabled) && (
              <p role='status' className='text-destructive text-sm'>
                {t('channels.plugins.providerUnavailable')}
              </p>
            )}
          {incompatibleCredential && (
            <p role='status' className='text-destructive text-sm'>
              {t('channels.plugins.providerCredentialMismatch')}
            </p>
          )}
          <ProviderProtocolFields
            value={value}
            manifest={manifest}
            protocols={protocols}
            blocked={blocked}
            update={update}
          />
          <ProviderImageQueries
            value={value}
            manifest={manifest}
            blocked={blocked}
            update={update}
            id={id}
            newChannel={newChannel}
            invalid={restProps['aria-invalid']}
          />
          <ProviderCatalogFields
            value={value}
            manifest={manifest}
            blocked={blocked}
            update={update}
            id={id}
            newChannel={newChannel}
            invalid={restProps['aria-invalid']}
            onRemove={() => {
              const { catalog: _, ...next } = value
              onChange(next)
            }}
          />
          <p className='text-muted-foreground text-xs'>
            {t('channels.plugins.providerSupport')}
          </p>
        </section>
      )}
    </div>
  )
}
