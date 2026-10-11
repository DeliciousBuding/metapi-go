import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import { supplyKindLabels } from '../lib/supply-kind-labels'
import type { ModelSupplySource } from '../types'

export function ModelSupplySources(props: {
  sources: ModelSupplySource[] | undefined
}) {
  const { t } = useTranslation()

  return (
    <section aria-label={t('models.supply.title')}>
      <h3 className='text-sm font-medium'>{t('models.supply.title')}</h3>
      <p className='text-muted-foreground mt-1 text-xs'>
        {t('models.supply.description')}
      </p>
      {props.sources === undefined ? (
        <p className='text-muted-foreground mt-3 text-sm'>
          {t('models.supply.unknown')}
        </p>
      ) : (
        <ul className='mt-3 space-y-3'>
          {props.sources.map((source) => (
            <li
              key={`${source.kind}-${source.id}`}
              className='space-y-2 rounded-md border p-3 text-sm'
            >
              <div className='flex flex-wrap items-center gap-2'>
                <span className='min-w-0 font-medium break-all'>
                  {source.connectionName}
                </span>
                <Badge variant='outline'>
                  {t(supplyKindLabels[source.kind])}
                </Badge>
                {!source.enabled && (
                  <Badge variant='secondary'>
                    {t('models.supply.disabled')}
                  </Badge>
                )}
              </div>
              <dl className='grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs'>
                <dt className='text-muted-foreground'>
                  {t('models.supply.identity')}
                </dt>
                <dd className='break-all'>
                  {source.name || `#${source.id}`}
                  {source.credentialKind && ` · ${source.credentialKind}`}
                </dd>
                <dt className='text-muted-foreground'>
                  {source.kind === 'direct'
                    ? t('models.supply.upstreamModel')
                    : t('models.supply.recordedModel')}
                </dt>
                <dd className='font-mono break-all'>{source.upstreamModel}</dd>
              </dl>
              <p className='text-muted-foreground text-xs'>
                {source.evidence === 'configured'
                  ? t('models.supply.configured')
                  : t('models.supply.availability')}
              </p>
              {source.successCount !== undefined &&
                source.failCount !== undefined && (
                  <p className='text-muted-foreground text-xs tabular-nums'>
                    {t('models.supply.observed', {
                      success: source.successCount,
                      failure: source.failCount,
                    })}
                  </p>
                )}
              {source.bindings.length > 0 && (
                <ul className='space-y-1 border-t pt-2'>
                  {source.bindings.map((binding) => (
                    <li key={binding.itemId} className='space-y-1 text-xs'>
                      <span className='font-mono break-all'>
                        {binding.publicModel} → {source.upstreamModel}
                      </span>
                      <div className='text-muted-foreground flex flex-wrap gap-2'>
                        <span>{t('models.supply.routeBinding')}</span>
                        {!binding.routeEnabled && (
                          <span>{t('models.supply.routeDisabled')}</span>
                        )}
                        {!binding.groupEnabled && (
                          <span>{t('models.supply.groupDisabled')}</span>
                        )}
                        {!binding.selected && (
                          <span>{t('models.supply.notSelected')}</span>
                        )}
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </li>
          ))}
          {props.sources.length === 0 && (
            <li className='text-muted-foreground text-sm'>
              {t('models.supply.empty')}
            </li>
          )}
        </ul>
      )}
    </section>
  )
}
