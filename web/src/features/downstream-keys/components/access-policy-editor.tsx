import { useQuery } from '@tanstack/react-query'
import { Plus, X } from 'lucide-react'
import { useId, useState, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelPicker } from '@/components/common/model-picker'
import { ModelPill } from '@/components/common/model-pill'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { StringMapEditor } from '@/components/common/string-map-editor'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { formLabelIdFor } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import { parseStringMap, serializeStringMap } from '@/lib/helpers/string-map'

import type { DownstreamAccessPolicy } from '../types'

type Quota = NonNullable<DownstreamAccessPolicy['quota']>
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
  const selectedChannels = policy.allowedUpstreamChannelIds
  const channelOptions = inventory.data?.items ?? []
  const unresolvedChannels =
    selectedChannels?.filter(
      (channelId) => !channelOptions.some((channel) => channel.id === channelId)
    ) ?? []
  const models = [
    ...new Set([
      ...candidateModels,
      ...(inventory.data?.members.map((member) => member.modelName) ?? []),
    ]),
  ]
  const quota = policy.quota
  const historyMissingBefore = quota?.historyMissingBefore
  const historyMissing = historyMissingBefore !== undefined

  function update(patch: Partial<DownstreamAccessPolicy>) {
    onChange({ ...policy, ...patch })
  }
  function updateQuota(patch: Partial<Quota>) {
    update({ quota: { period: { type: 'all_time' }, ...quota, ...patch } })
  }
  function addModel(model: string) {
    const name = model.trim()
    if (!name) return
    update({ modelIds: [...new Set([...(policy.modelIds ?? []), name])] })
    setPendingModel('')
  }
  function selectPeriod(mode: string) {
    if (mode === 'past_duration') {
      updateQuota({
        period: { type: mode, pastDuration: { value: 1, unit: 'hour' } },
      })
    } else if (mode === 'day' || mode === 'month') {
      updateQuota({
        period: { type: 'calendar_duration', calendarDuration: { unit: mode } },
      })
    } else {
      updateQuota({ period: { type: 'all_time' } })
    }
  }
  const periodMode =
    quota?.period.type === 'calendar_duration'
      ? quota.period.calendarDuration?.unit
      : quota?.period.type

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
          <div className='space-y-2'>
            <Label htmlFor={`${id}-channels`}>
              {t('settings.downstream.keys.access.channelScope')}
            </Label>
            <Select
              value={selectedChannels === undefined ? 'all' : 'selected'}
              onValueChange={(mode) =>
                update({
                  allowedUpstreamChannelIds:
                    mode === 'selected' ? [] : undefined,
                })
              }
            >
              <SelectTrigger id={`${id}-channels`} className='w-full'>
                <SelectValue>
                  {t(
                    `settings.downstream.keys.access.${selectedChannels === undefined ? 'allChannels' : 'selectedChannels'}`
                  )}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>
                  {t('settings.downstream.keys.access.allChannels')}
                </SelectItem>
                <SelectItem value='selected'>
                  {t('settings.downstream.keys.access.selectedChannels')}
                </SelectItem>
              </SelectContent>
            </Select>
            <p className='text-muted-foreground text-xs'>
              {t('settings.downstream.keys.access.channelHint')}
            </p>
            {selectedChannels !== undefined && (
              <div className='space-y-2'>
                {selectedChannels.length === 0 && (
                  <Notice tone='warning'>
                    {t('settings.downstream.keys.access.noChannels')}
                  </Notice>
                )}
                {inventory.isPending && (
                  <p className='text-muted-foreground text-sm'>
                    {t('settings.downstream.keys.access.loadingChannels')}
                  </p>
                )}
                <QueryErrorBanner
                  error={inventory.error}
                  messageKey='settings.downstream.keys.access.loadError'
                  onRetry={() => inventory.refetch()}
                  isRetrying={inventory.isFetching}
                />
                <div className='max-h-48 space-y-1 overflow-y-auto rounded-lg border p-2'>
                  {channelOptions.map((channel) => (
                    <Label
                      key={channel.id}
                      className='flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 font-normal'
                    >
                      <Checkbox
                        checked={selectedChannels.includes(channel.id)}
                        onCheckedChange={(checked) =>
                          update({
                            allowedUpstreamChannelIds: checked
                              ? [...selectedChannels, channel.id]
                              : selectedChannels.filter(
                                  (item) => item !== channel.id
                                ),
                          })
                        }
                      />
                      <span className='min-w-0 flex-1 break-words'>
                        {channel.name}
                      </span>
                      {!channel.enabled && (
                        <Badge variant='outline'>
                          {t('settings.downstream.keys.access.disabledChannel')}
                        </Badge>
                      )}
                    </Label>
                  ))}
                  {!inventory.isPending &&
                    !inventory.error &&
                    channelOptions.length === 0 && (
                      <p className='text-muted-foreground p-2 text-sm'>
                        {t('settings.downstream.keys.access.emptyInventory')}
                      </p>
                    )}
                  {!inventory.isPending &&
                    unresolvedChannels.map((channelId) => (
                      <Label
                        key={channelId}
                        className='flex items-center gap-2 px-2 py-1.5 font-normal'
                      >
                        <Checkbox
                          checked
                          onCheckedChange={() =>
                            update({
                              allowedUpstreamChannelIds:
                                selectedChannels.filter(
                                  (item) => item !== channelId
                                ),
                            })
                          }
                        />
                        {t(
                          'settings.downstream.keys.access.unavailableChannel',
                          { id: channelId }
                        )}
                      </Label>
                    ))}
                </div>
              </div>
            )}
          </div>

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

          <div className='space-y-3 border-t pt-4'>
            <div className='flex items-center justify-between gap-3'>
              <Label htmlFor={`${id}-quota`}>
                {t('settings.downstream.keys.access.quota')}
              </Label>
              <Switch
                id={`${id}-quota`}
                checked={quota !== undefined}
                disabled={historyMissing}
                onCheckedChange={(enabled) =>
                  update({
                    quota: enabled
                      ? { period: { type: 'all_time' } }
                      : undefined,
                  })
                }
              />
            </div>
            {historyMissing && (
              <Notice tone='warning' role='status'>
                <p className='font-medium'>
                  {t('settings.downstream.keys.access.historyTitle')}
                </p>
                <p>
                  {t('settings.downstream.keys.access.historyHint', {
                    date: new Date(historyMissingBefore).toLocaleString(),
                  })}
                </p>
              </Notice>
            )}
            {quota && (
              <>
                <p className='text-muted-foreground text-xs'>
                  {t('settings.downstream.keys.access.quotaHint')}
                </p>
                <div className='grid gap-3 sm:grid-cols-3'>
                  {(['requests', 'totalTokens', 'cost'] as const).map(
                    (field) => (
                      <div key={field} className='space-y-1.5'>
                        <Label htmlFor={`${id}-${field}`}>
                          {t(`settings.downstream.keys.access.${field}`)}
                        </Label>
                        <Input
                          id={`${id}-${field}`}
                          type='number'
                          min={0}
                          step={field === 'cost' ? 'any' : 1}
                          value={quota[field] ?? ''}
                          placeholder={t('settings.common.unlimited')}
                          onChange={(event) =>
                            updateQuota({
                              [field]:
                                event.target.value === ''
                                  ? undefined
                                  : Number(event.target.value),
                            })
                          }
                        />
                      </div>
                    )
                  )}
                </div>
                <div className='space-y-1.5'>
                  <Label htmlFor={`${id}-period`}>
                    {t('settings.downstream.keys.access.period')}
                  </Label>
                  <Select
                    value={periodMode}
                    onValueChange={(mode) => {
                      if (mode) selectPeriod(mode)
                    }}
                  >
                    <SelectTrigger id={`${id}-period`} className='w-full'>
                      <SelectValue>
                        {t(
                          `settings.downstream.keys.access.periods.${periodMode ?? 'all_time'}`
                        )}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {(
                        ['all_time', 'past_duration', 'day', 'month'] as const
                      ).map((mode) => (
                        <SelectItem key={mode} value={mode}>
                          {t(`settings.downstream.keys.access.periods.${mode}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                {quota.period.type === 'past_duration' && (
                  <div className='grid grid-cols-2 gap-3'>
                    <div className='space-y-1.5'>
                      <Label htmlFor={`${id}-duration`}>
                        {t('settings.downstream.keys.access.duration')}
                      </Label>
                      <Input
                        id={`${id}-duration`}
                        type='number'
                        min={1}
                        step={1}
                        value={quota.period.pastDuration?.value ?? ''}
                        onChange={(event) =>
                          updateQuota({
                            period: {
                              type: 'past_duration',
                              pastDuration: {
                                value: Number(event.target.value),
                                unit: quota.period.pastDuration?.unit ?? 'hour',
                              },
                            },
                          })
                        }
                      />
                    </div>
                    <div className='space-y-1.5'>
                      <Label htmlFor={`${id}-unit`}>
                        {t('settings.downstream.keys.access.unit')}
                      </Label>
                      <Select
                        value={quota.period.pastDuration?.unit}
                        onValueChange={(unit) => {
                          if (
                            unit === 'minute' ||
                            unit === 'hour' ||
                            unit === 'day'
                          ) {
                            updateQuota({
                              period: {
                                type: 'past_duration',
                                pastDuration: {
                                  value: quota.period.pastDuration?.value ?? 1,
                                  unit,
                                },
                              },
                            })
                          }
                        }}
                      >
                        <SelectTrigger id={`${id}-unit`} className='w-full'>
                          <SelectValue>
                            {t(
                              `settings.downstream.keys.access.units.${quota.period.pastDuration?.unit ?? 'hour'}`
                            )}
                          </SelectValue>
                        </SelectTrigger>
                        <SelectContent>
                          {(['minute', 'hour', 'day'] as const).map((unit) => (
                            <SelectItem key={unit} value={unit}>
                              {t(
                                `settings.downstream.keys.access.units.${unit}`
                              )}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                )}
                <div className='space-y-1.5'>
                  <Label htmlFor={`${id}-timezone`}>
                    {t('settings.downstream.keys.access.timezone')}
                  </Label>
                  <Input
                    id={`${id}-timezone`}
                    value={quota.timezone ?? ''}
                    placeholder='UTC'
                    onChange={(event) =>
                      updateQuota({ timezone: event.target.value || undefined })
                    }
                  />
                  <p className='text-muted-foreground text-xs'>
                    {t('settings.downstream.keys.access.timezoneHint')}
                  </p>
                </div>
              </>
            )}
          </div>
        </>
      )}
    </div>
  )
}
