import { useQuery } from '@tanstack/react-query'
import { Plus, X } from 'lucide-react'
import { useId, useState, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelPicker } from '@/components/common/model-picker'
import { ModelPill } from '@/components/common/model-pill'
import { StringMapEditor } from '@/components/common/string-map-editor'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formLabelIdFor } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import { api } from '@/lib/api'
import { parseStringMap, serializeStringMap } from '@/lib/helpers/string-map'

import type { DownstreamAccessPolicy } from '../types'
import { AccessPolicyChannels } from './access-policy-channels'
import { AccessPolicyQuota } from './access-policy-quota'

type Props = Omit<ComponentProps<'div'>, 'onChange'> & {
  value: DownstreamAccessPolicy | null | undefined
  onChange: (value: DownstreamAccessPolicy | null) => void
  candidateModels?: string[]
}

const blockMessages: Record<string, string> = {
  source_key_type_not_proxy: 'keyType',
  source_key_missing_write_requests: 'scope',
  source_project_unavailable: 'project',
  source_key_routing_override_unsupported: 'routing',
  'source_key_strategy_not_equivalent:adaptive': 'routing',
  'source_key_strategy_not_equivalent:failover': 'routing',
  'source_key_strategy_not_equivalent:circuit-breaker': 'routing',
  source_key_strategy_unknown: 'routing',
  source_key_trace_sticky_unknown: 'routing',
  source_key_model_mapping_unsupported: 'mapping',
}

/** One controlled policy object; UI drafts never replace imported constraints. */
export function AccessPolicyEditor({
  value,
  onChange,
  candidateModels = [],
  ...props
}: Props) {
  const { t } = useTranslation()
  const id = useId()
  const [pendingModel, setPendingModel] = useState('')
  const inventory = useQuery({
    queryKey: ['imported-upstreams'],
    queryFn: api.getImportedUpstreams,
  })
  const policy = value ?? {}
  const models = [
    ...new Set([
      ...candidateModels,
      ...(inventory.data?.members.map((member) => member.modelName) ?? []),
    ]),
  ]
  function update(patch: Partial<DownstreamAccessPolicy>) {
    onChange({ ...policy, ...patch })
  }
  function addModel(model: string) {
    const name = model.trim()
    if (!name) return
    update({ modelIds: [...new Set([...(policy.modelIds ?? []), name])] })
    setPendingModel('')
  }
  return (
    <div
      role='group'
      aria-labelledby={props.id ? formLabelIdFor(props.id) : undefined}
      className='space-y-5'
      tabIndex={-1}
      {...props}
    >
      {value === undefined ? (
        <Notice tone='warning'>
          {t('settings.downstream.keys.access.unreadable')}
        </Notice>
      ) : (
        <>
          {policy.blockReason && (
            <Notice tone='warning' role='status'>
              <p className='font-medium'>
                {t('settings.downstream.keys.access.blockedTitle')}
              </p>
              <p>
                {t(
                  `settings.downstream.keys.access.blocks.${blockMessages[policy.blockReason] ?? 'unknown'}`
                )}
              </p>
            </Notice>
          )}
          {(policy.routingStrategy || policy.traceStickyMode) && (
            <Notice tone='info'>
              {t('settings.downstream.keys.access.routingPolicySummary', {
                strategy: policy.routingStrategy || 'default',
                sticky: policy.traceStickyMode || 'default',
              })}
            </Notice>
          )}
        </>
      )}
      <AccessPolicyChannels
        visible={value !== undefined}
        id={id}
        selectedChannels={policy.allowedUpstreamChannelIds}
        inventory={inventory}
        onChange={(ids) => update({ allowedUpstreamChannelIds: ids })}
      />
      {value !== undefined && (
        <>
          <div className='space-y-2'>
            <Label htmlFor={`${id}-model`}>
              {t('settings.downstream.keys.access.models')}
            </Label>
            <p className='text-muted-foreground text-xs'>
              {t('settings.downstream.keys.access.modelsHint')}
            </p>
            <div className='flex flex-wrap gap-2'>
              {policy.modelIds?.map((model) => (
                <Badge key={model} variant='secondary' className='gap-1 pr-1'>
                  <ModelPill model={model} variant='inline' />
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-xs'
                    aria-label={t(
                      'settings.downstream.keys.access.removeModel',
                      { model }
                    )}
                    onClick={() =>
                      update({
                        modelIds: policy.modelIds?.filter(
                          (item) => item !== model
                        ),
                      })
                    }
                  >
                    <X className='size-3' />
                  </Button>
                </Badge>
              ))}
            </div>
            <div className='flex items-center gap-2'>
              <Input
                id={`${id}-model`}
                value={pendingModel}
                placeholder='gpt-6'
                onChange={(event) => setPendingModel(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    if (
                      event.nativeEvent.isComposing ||
                      event.nativeEvent.keyCode === 229
                    )
                      return
                    addModel(pendingModel)
                  }
                }}
              />
              <Button
                type='button'
                variant='outline'
                size='icon'
                aria-label={t('settings.downstream.keys.access.addModel')}
                disabled={!pendingModel.trim()}
                onClick={() => addModel(pendingModel)}
              >
                <Plus className='size-4' />
              </Button>
            </div>
            <ModelPicker
              models={models.map((name) => ({ name }))}
              value=''
              onValueChange={addModel}
              placeholder={t('settings.downstream.keys.access.pickModel')}
              aria-label={t('settings.downstream.keys.access.pickModel')}
            />
          </div>

          <div className='space-y-2'>
            <Label htmlFor={`${id}-mappings`}>
              {t('settings.downstream.keys.access.mappings')}
            </Label>
            <p className='text-muted-foreground text-xs'>
              {t('settings.downstream.keys.access.mappingsHint')}
            </p>
            <StringMapEditor
              id={`${id}-mappings`}
              structuredOnly
              ordered
              value={serializeStringMap(
                (policy.modelMappings ?? []).map((mapping) => ({
                  key: mapping.from,
                  value: mapping.to,
                }))
              )}
              onChange={(text) => {
                const entries = parseStringMap(text)
                if (entries) {
                  update({
                    modelMappings: entries.map((entry) => ({
                      from: entry.key,
                      to: entry.value,
                    })),
                  })
                }
              }}
              keyLabel={t('settings.downstream.keys.access.mappingFrom')}
              valueLabel={t('settings.downstream.keys.access.mappingTo')}
            />
          </div>

          <AccessPolicyQuota
            id={id}
            quota={policy.quota}
            onChange={(quota) => update({ quota })}
          />
        </>
      )}
    </div>
  )
}
