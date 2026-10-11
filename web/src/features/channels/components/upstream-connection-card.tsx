import { ArrowUpRight, Boxes, KeyRound, Route } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type {
  ImportedMember,
  ImportedUpstream,
} from '@/lib/api/imported-upstreams'

import { memberProtocols } from '../lib/upstream-config'
import {
  connectionReadiness,
  type UpstreamDetailTab,
} from '../lib/upstream-view'
import { UpstreamIdentity } from './upstream-identity'

export function UpstreamConnectionCard(props: {
  item: ImportedUpstream
  members: ImportedMember[]
  now: number
  updating: boolean
  onManage: (tab: UpstreamDetailTab) => void
  onToggle: () => void
}) {
  const { t } = useTranslation()
  const item = props.item
  const models = [...new Set(props.members.map((member) => member.modelName))]
  const routes = new Set(
    props.members
      .filter((member) => member.routeId > 0)
      .map((member) => member.routeId)
  ).size
  const readiness = connectionReadiness(item, props.members, props.now)
  const steps = [
    {
      tab: 'credentials' as const,
      icon: KeyRound,
      count: item.credentialCount,
    },
    { tab: 'models' as const, icon: Boxes, count: item.modelCount },
    { tab: 'routes' as const, icon: Route, count: routes },
  ]
  return (
    <article
      aria-label={item.name}
      className='bg-card flex min-w-0 flex-col gap-4 rounded-xl border p-4'
    >
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0 space-y-2'>
          <UpstreamIdentity provider={item.provider} dialect={item.dialect} />
          <h2 className='text-base font-semibold'>
            <button
              type='button'
              className='hover:text-primary focus-visible:outline-ring max-w-full truncate text-left'
              onClick={() => props.onManage('connection')}
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
      <div
        className='grid grid-cols-3 gap-2'
        aria-label={t('channels.visual.connectionSteps')}
      >
        {steps.map((step) => (
          <button
            key={step.tab}
            type='button'
            className='bg-muted/40 hover:bg-accent focus-visible:ring-ring flex min-w-0 flex-col items-start gap-1 rounded-lg border p-3 text-left focus-visible:ring-2 focus-visible:outline-none'
            aria-label={t('channels.visual.openStep', {
              name: item.name,
              step: t(`channels.visual.steps.${step.tab}`),
              count: step.count,
            })}
            onClick={() => props.onManage(step.tab)}
          >
            <span className='flex w-full items-center justify-between gap-2'>
              <span className='text-lg font-semibold tabular-nums'>
                {step.count}
              </span>
              <step.icon
                aria-hidden='true'
                className='text-muted-foreground size-3.5 shrink-0'
              />
            </span>
            <span className='text-muted-foreground text-xs break-words'>
              {t(`channels.visual.steps.${step.tab}`)}
            </span>
          </button>
        ))}
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
        {[...new Set(props.members.map(memberProtocols))]
          .slice(0, 2)
          .map((protocols) => (
            <p key={protocols}>{protocols}</p>
          ))}
      </div>
      <div className='mt-auto flex flex-wrap items-center justify-between gap-3 border-t pt-3'>
        <p className='text-muted-foreground text-xs' role='status'>
          {t(`channels.visual.readiness.${readiness}`)}
        </p>
        <div className='flex items-center gap-1'>
          <Button
            size='sm'
            variant='ghost'
            disabled={props.updating}
            aria-label={t(
              item.enabled
                ? 'channels.imported.disableChannel'
                : 'channels.imported.enableChannel',
              { name: item.name }
            )}
            onClick={props.onToggle}
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
            onClick={() => props.onManage('connection')}
          >
            {t('channels.upstream.manage')}
            <ArrowUpRight aria-hidden='true' className='size-3.5' />
          </Button>
        </div>
      </div>
    </article>
  )
}
