import {
  AlertTriangle,
  CheckCircle2,
  Globe,
  KeyRound,
  ShieldCheck,
  Webhook,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import {
  extractStaticPluginMetaPreview,
  type PluginMetaPreviewResult,
} from '../lib/plugin-meta-preview'

export function ChannelPluginMetaPreview({
  source,
  pluginKey,
}: {
  source: string
  pluginKey: string
}) {
  const { t } = useTranslation()
  const preview = useMemo(
    () => extractStaticPluginMetaPreview(source, pluginKey),
    [source, pluginKey]
  )

  return (
    <aside
      aria-label={t('channels.plugins.preview.title')}
      className='bg-muted/20 flex h-fit flex-col gap-4 rounded-xl border p-4 text-xs'
    >
      <div className='space-y-1 border-b pb-3'>
        <div className='flex items-center gap-2'>
          <ShieldCheck
            className='text-primary size-4 shrink-0'
            aria-hidden='true'
          />
          <h3 className='text-sm font-semibold'>
            {t('channels.plugins.preview.title')}
          </h3>
        </div>
        <p className='text-muted-foreground leading-5'>
          {t('channels.plugins.preview.subtitle')}
        </p>
      </div>

      <PreviewBody preview={preview} pluginKey={pluginKey} />
    </aside>
  )
}

function PreviewBody({
  preview,
  pluginKey,
}: {
  preview: PluginMetaPreviewResult
  pluginKey: string
}) {
  const { t } = useTranslation()

  if (preview.status === 'empty' || preview.status === 'not_found') {
    return (
      <p className='text-muted-foreground rounded-lg border border-dashed p-3 leading-5'>
        {t('channels.plugins.preview.notFound')}
      </p>
    )
  }

  if (preview.status === 'dynamic') {
    return (
      <div
        role='status'
        className='border-warning/40 bg-warning/10 space-y-1 rounded-lg border p-3'
      >
        <p className='text-warning-soft-fg flex items-center gap-1.5 font-medium'>
          <AlertTriangle className='size-3.5 shrink-0' aria-hidden='true' />
          {t('channels.plugins.preview.dynamicMeta')}
        </p>
        <p className='text-muted-foreground font-mono break-words'>
          {preview.reason}
        </p>
      </div>
    )
  }

  if (preview.status === 'invalid_schema') {
    const maybeVersion = preview.rawMeta.apiVersion
    const maybeProtocols = Array.isArray(preview.rawMeta.protocols)
      ? preview.rawMeta.protocols.filter(
          (p): p is string => typeof p === 'string'
        )
      : []
    const maybeHooks = Array.isArray(preview.rawMeta.hooks)
      ? preview.rawMeta.hooks.filter((h): h is string => typeof h === 'string')
      : []
    return (
      <div className='space-y-3'>
        <div
          role='status'
          className='border-destructive/40 bg-destructive/10 space-y-1.5 rounded-lg border p-3'
        >
          <p className='text-destructive-soft-fg flex items-center gap-1.5 font-medium'>
            <AlertTriangle className='size-3.5 shrink-0' aria-hidden='true' />
            {t('channels.plugins.preview.schemaInvalid')}
          </p>
          <ul className='text-muted-foreground list-disc space-y-0.5 pl-4'>
            {preview.issues.map((issue) => (
              <li key={issue} className='break-words'>
                {issue}
              </li>
            ))}
          </ul>
        </div>
        {(maybeVersion !== undefined ||
          maybeProtocols.length > 0 ||
          maybeHooks.length > 0) && (
          <div className='space-y-2 border-t pt-3'>
            {maybeVersion !== undefined && (
              <p>
                <span className='text-muted-foreground'>
                  {t('channels.plugins.preview.apiVersion')}:{' '}
                </span>
                <span className='font-mono'>{String(maybeVersion)}</span>
              </p>
            )}
            {maybeProtocols.length > 0 && (
              <div className='flex flex-wrap gap-1'>
                {maybeProtocols.map((p) => (
                  <Badge key={p} variant='outline'>
                    {p}
                  </Badge>
                ))}
              </div>
            )}
            {maybeHooks.length > 0 && (
              <div className='flex flex-wrap gap-1'>
                {maybeHooks.map((h) => (
                  <Badge key={h} variant='secondary'>
                    {h}
                  </Badge>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    )
  }

  const { manifest, keyMismatch } = preview
  const isProvider = manifest.apiVersion === 2
  const endpointEntries = isProvider
    ? Object.entries(manifest.defaults?.endpoints ?? {})
    : []
  const imageTaskEntries = isProvider
    ? Object.entries(manifest.imageTasks ?? {})
    : []

  return (
    <div className='space-y-4'>
      <div className='space-y-2'>
        <div className='flex flex-wrap items-center gap-2'>
          <Badge variant='success' className='gap-1'>
            <CheckCircle2 className='size-3' aria-hidden='true' />
            {t('channels.plugins.preview.valid')}
          </Badge>
          <Badge variant={isProvider ? 'default' : 'secondary'}>
            {t(
              isProvider
                ? 'channels.plugins.preview.apiProvider'
                : 'channels.plugins.preview.apiWire'
            )}
          </Badge>
        </div>

        {keyMismatch && (
          <p
            role='status'
            className='border-warning/40 bg-warning/10 text-warning-soft-fg rounded-md border px-2.5 py-1.5'
          >
            {t('channels.plugins.preview.keyMismatch', {
              metaKey: manifest.key,
              pluginKey,
            })}
          </p>
        )}

        <dl className='bg-background grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 rounded-lg border p-2.5'>
          <dt className='text-muted-foreground'>name</dt>
          <dd className='font-medium break-words'>{manifest.name}</dd>
          <dt className='text-muted-foreground'>key</dt>
          <dd className='font-mono break-all'>{manifest.key}</dd>
          <dt className='text-muted-foreground'>version</dt>
          <dd className='font-mono'>{manifest.version}</dd>
        </dl>
      </div>

      <section className='space-y-1.5'>
        <h4 className='text-muted-foreground font-medium'>
          {t('channels.plugins.preview.protocols')}
        </h4>
        <div className='flex flex-wrap gap-1'>
          {manifest.protocols.map((protocol) => (
            <Badge key={protocol} variant='outline'>
              {protocol}
            </Badge>
          ))}
        </div>
      </section>

      <section className='space-y-1.5'>
        <h4 className='text-muted-foreground flex items-center gap-1 font-medium'>
          <Webhook className='size-3.5' aria-hidden='true' />
          {t('channels.plugins.preview.hooks')}
        </h4>
        <div className='flex flex-wrap gap-1'>
          {manifest.hooks.map((hook) => (
            <Badge key={hook} variant='secondary'>
              {hook}
            </Badge>
          ))}
        </div>
      </section>

      {isProvider && (
        <section className='space-y-1.5'>
          <h4 className='text-muted-foreground flex items-center gap-1 font-medium'>
            <KeyRound className='size-3.5' aria-hidden='true' />
            {t('channels.plugins.preview.credentialKinds')}
          </h4>
          <div className='flex flex-wrap gap-1'>
            {manifest.credentialKinds.map((kind) => (
              <Badge key={kind} variant='outline'>
                {kind}
              </Badge>
            ))}
          </div>
        </section>
      )}

      {isProvider && manifest.authentication && (
        <section className='space-y-1.5'>
          <h4 className='text-muted-foreground font-medium'>
            {t('channels.plugins.preview.authentication')}
          </h4>
          <div className='bg-background space-y-1 rounded-lg border p-2.5'>
            <div className='flex flex-wrap items-center gap-1.5'>
              <Badge variant='warning'>{manifest.authentication.type}</Badge>
              <code className='font-mono font-medium'>
                {manifest.authentication.header}
              </code>
              {manifest.authentication.prefix ? (
                <span className='text-muted-foreground font-mono'>
                  ({manifest.authentication.prefix})
                </span>
              ) : null}
            </div>
            {manifest.authentication.jwt && (
              <p className='text-muted-foreground font-mono'>
                issuer: {manifest.authentication.jwt.issuerClaim} · ttl:{' '}
                {manifest.authentication.jwt.ttlSeconds}s
              </p>
            )}
          </div>
        </section>
      )}

      {isProvider && (
        <section className='space-y-1.5'>
          <h4 className='text-muted-foreground flex items-center gap-1 font-medium'>
            <Globe className='size-3.5' aria-hidden='true' />
            {t('channels.plugins.preview.endpoints')}
          </h4>
          {endpointEntries.length > 0 ? (
            <ul className='bg-background divide-y rounded-lg border'>
              {endpointEntries.map(([protocol, tmpl]) =>
                tmpl ? (
                  <li
                    key={protocol}
                    className='flex flex-wrap items-center justify-between gap-2 px-2.5 py-1.5'
                  >
                    <span className='font-medium'>{protocol}</span>
                    <span className='flex items-center gap-1.5'>
                      <code className='text-muted-foreground font-mono break-all'>
                        {tmpl.path}
                      </code>
                      <Badge variant='outline'>{tmpl.auth}</Badge>
                    </span>
                  </li>
                ) : null
              )}
            </ul>
          ) : (
            <p className='text-muted-foreground'>
              {t('channels.plugins.preview.noneDeclared')}
            </p>
          )}

          {manifest.defaults?.catalog && (
            <div className='bg-background flex flex-wrap items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5'>
              <span className='font-medium'>
                {t('channels.plugins.preview.catalog')}
              </span>
              <span className='flex items-center gap-1.5'>
                <code className='text-muted-foreground font-mono break-all'>
                  {manifest.defaults.catalog.path}
                </code>
                <Badge variant='outline'>
                  {manifest.defaults.catalog.auth}
                </Badge>
              </span>
            </div>
          )}

          {imageTaskEntries.length > 0 && (
            <div className='space-y-1 pt-1'>
              <p className='text-muted-foreground font-medium'>
                {t('channels.plugins.preview.imageQueries')}
              </p>
              <ul className='bg-background divide-y rounded-lg border'>
                {imageTaskEntries.map(([protocol, task]) =>
                  task?.query ? (
                    <li
                      key={protocol}
                      className='flex flex-wrap items-center justify-between gap-2 px-2.5 py-1.5'
                    >
                      <span className='font-medium'>{protocol}</span>
                      <code className='text-muted-foreground font-mono break-all'>
                        {task.query.path}
                      </code>
                    </li>
                  ) : null
                )}
              </ul>
            </div>
          )}
        </section>
      )}

      {manifest.options && manifest.options.length > 0 && (
        <section className='space-y-1.5'>
          <h4 className='text-muted-foreground font-medium'>
            {t('channels.plugins.preview.options', {
              count: manifest.options.length,
            })}
          </h4>
          <div className='flex flex-wrap gap-1'>
            {manifest.options.map((opt) => (
              <Badge key={opt.key} variant='outline' className='font-mono'>
                {opt.key}: {opt.type}
              </Badge>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
