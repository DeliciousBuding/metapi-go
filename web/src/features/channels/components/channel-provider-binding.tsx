import { useQuery } from '@tanstack/react-query'
import { useId, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { formLabelIdFor } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  channelPluginQueryKey,
  channelPluginsApi,
  type ProviderBinding,
} from '@/lib/api/channel-plugins'
import type {
  ImportedCredential,
  ImportedEndpointConfig,
} from '@/lib/api/imported-upstreams'

import { PluginOptionField } from './channel-plugin-bindings'

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
  const catalog = value?.catalog
  const blocked = disabled || query.isPending || query.isError
  const items = (query.data?.items ?? []).filter(
    (item) => item.manifest?.apiVersion === 2
  )
  const current = items.find((item) => item.key === value?.key)
  const manifest =
    current?.manifest?.apiVersion === 2 ? current.manifest : undefined
  const available =
    manifest?.protocols.filter((protocol) => protocols.includes(protocol)) ?? []
  const missing = !!value && !current
  const incompatibleCredential =
    !!manifest &&
    credentialKind !== undefined &&
    !manifest.credentialKinds.some((kind) => kind === credentialKind)
  function update(patch: Partial<ProviderBinding>) {
    if (value) onChange({ ...value, ...patch })
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
      <label id={`${id}-label`} htmlFor={`${id}-selector`} className='text-sm'>
        {t('channels.plugins.providerSelect')}
      </label>
      <Select
        value={value?.key ?? '__native__'}
        disabled={blocked}
        onValueChange={(key) => {
          if (key === '__native__') {
            onChange(null)
            return
          }
          const item = items.find((item) => item.key === key)
          if (item?.manifest?.apiVersion !== 2 || !item.enabled) return
          onChange({
            key: item.key,
            protocols: item.manifest.protocols.filter((protocol) =>
              protocols.includes(protocol)
            ),
          })
        }}
      >
        <SelectTrigger id={`${id}-selector`} className='w-full min-w-0'>
          <SelectValue>
            {value
              ? (manifest?.name ?? value.key)
              : t('channels.plugins.nativeProvider')}
          </SelectValue>
        </SelectTrigger>
        <SelectContent aria-labelledby={`${id}-label`}>
          <SelectItem value='__native__'>
            {t('channels.plugins.nativeProvider')}
          </SelectItem>
          {missing && (
            <SelectItem value={value.key} disabled>
              {value.key} · {t('channels.plugins.unavailable')}
            </SelectItem>
          )}
          {items.map((item) => {
            const meta = item.manifest
            if (meta?.apiVersion !== 2) return null
            const eligible =
              item.enabled &&
              !item.loadError &&
              meta.protocols.some((protocol) => protocols.includes(protocol)) &&
              (credentialKind === undefined ||
                meta.credentialKinds.some((kind) => kind === credentialKind))
            return (
              <SelectItem key={item.key} value={item.key} disabled={!eligible}>
                {meta.name}
                {!eligible && ` · ${t('channels.plugins.unavailable')}`}
              </SelectItem>
            )
          })}
        </SelectContent>
      </Select>
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
          <div className='flex flex-wrap gap-3'>
            {[...new Set([...available, ...value.protocols])].map(
              (protocol) => (
                <label
                  key={protocol}
                  className='flex items-center gap-2 text-sm'
                >
                  <Checkbox
                    checked={value.protocols.includes(protocol)}
                    disabled={
                      blocked ||
                      (!value.protocols.includes(protocol) &&
                        !available.includes(protocol)) ||
                      (value.protocols.length === 1 &&
                        value.protocols.includes(protocol))
                    }
                    onCheckedChange={(checked) =>
                      update({
                        protocols: checked
                          ? [...value.protocols, protocol]
                          : value.protocols.filter((item) => item !== protocol),
                      })
                    }
                  />
                  {t(`channels.capabilities.names.${protocol}`)}
                  {!protocols.includes(protocol) && (
                    <span className='text-destructive'>
                      {t('channels.plugins.endpointMissing')}
                    </span>
                  )}
                </label>
              )
            )}
          </div>
          {manifest?.options?.map((option) => (
            <PluginOptionField
              key={option.key}
              option={option}
              value={value.options?.[option.key] ?? option.default}
              disabled={blocked}
              onChange={(next) =>
                update({ options: { ...value.options, [option.key]: next } })
              }
            />
          ))}
          {catalog ? (
            <div className='space-y-2 border-t pt-3'>
              <label htmlFor={`${id}-catalog`} className='text-sm'>
                {t('channels.plugins.catalogUrl')}
              </label>
              <Input
                id={`${id}-catalog`}
                value={catalog.url}
                aria-invalid={restProps['aria-invalid']}
                disabled={blocked}
                onChange={(event) =>
                  update({
                    catalog: { ...catalog, url: event.target.value },
                  })
                }
              />
              <label
                id={`${id}-auth-label`}
                htmlFor={`${id}-auth`}
                className='text-sm'
              >
                {t('channels.plugins.catalogAuth')}
              </label>
              <Select
                value={catalog.auth}
                disabled={blocked}
                onValueChange={(auth) => {
                  if (
                    auth === 'bearer' ||
                    auth === 'x-api-key' ||
                    auth === 'x-goog-api-key' ||
                    auth === 'none'
                  )
                    update({ catalog: { ...catalog, auth } })
                }}
              >
                <SelectTrigger id={`${id}-auth`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent aria-labelledby={`${id}-auth-label`}>
                  {(
                    ['bearer', 'x-api-key', 'x-goog-api-key', 'none'] as const
                  ).map((auth) => (
                    <SelectItem key={auth} value={auth}>
                      {auth}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <label htmlFor={`${id}-cursor`} className='text-sm'>
                {t('channels.plugins.catalogCursor')}
              </label>
              <Input
                id={`${id}-cursor`}
                value={catalog.cursorParameter ?? ''}
                disabled={blocked}
                onChange={(event) =>
                  update({
                    catalog: {
                      ...catalog,
                      cursorParameter: event.target.value,
                    },
                  })
                }
              />
              <Button
                type='button'
                size='xs'
                variant='outline'
                disabled={blocked}
                onClick={() => {
                  const { catalog: _, ...next } = value
                  onChange(next)
                }}
              >
                {t(
                  newChannel && manifest?.defaults?.catalog
                    ? 'channels.plugins.catalogUseDefault'
                    : 'channels.plugins.catalogRemove'
                )}
              </Button>
            </div>
          ) : (
            manifest?.hooks.includes('modelsRequest') && (
              <div className='space-y-2'>
                {newChannel && manifest.defaults?.catalog && (
                  <p className='text-muted-foreground text-sm'>
                    {t('channels.plugins.catalogDefault', {
                      path: manifest.defaults.catalog.path,
                    })}
                  </p>
                )}
                <Button
                  type='button'
                  size='xs'
                  variant='outline'
                  disabled={blocked}
                  onClick={() =>
                    update({ catalog: { url: '', auth: 'bearer' } })
                  }
                >
                  {t('channels.plugins.catalogConfigure')}
                </Button>
              </div>
            )
          )}
          <p className='text-muted-foreground text-xs'>
            {t('channels.plugins.catalogHint')}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t('channels.plugins.providerSupport')}
          </p>
        </section>
      )}
    </div>
  )
}
