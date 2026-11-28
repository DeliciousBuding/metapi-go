import type { UseQueryResult } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { ImportedUpstreamInventory } from '@/lib/api/imported-upstreams'

import { useScopeSelectionFocus } from '../hooks/use-scope-selection-focus'
import type { DownstreamAccessPolicy } from '../types'
import { ScopePickerToolbar } from './scope-picker-toolbar'

export function AccessPolicyChannels({
  id,
  selectedChannels,
  inventory,
  onChange,
  visible,
}: {
  id: string
  selectedChannels: DownstreamAccessPolicy['allowedUpstreamChannelIds']
  inventory: UseQueryResult<ImportedUpstreamInventory>
  onChange: (ids: DownstreamAccessPolicy['allowedUpstreamChannelIds']) => void
  visible: boolean
}) {
  const { t } = useTranslation()
  const [channelSearch, setChannelSearch] = useState('')
  const [onlySelectedChannels, setOnlySelectedChannels] = useState(false)
  const focus = useScopeSelectionFocus()
  const channelOptions = inventory.data?.items ?? []
  const unresolvedChannels =
    selectedChannels?.filter(
      (channelId) => !channelOptions.some((channel) => channel.id === channelId)
    ) ?? []
  const filteredChannels = channelOptions.filter(
    (channel) =>
      (!onlySelectedChannels || selectedChannels?.includes(channel.id)) &&
      `${channel.name} ${channel.id}`
        .toLocaleLowerCase()
        .includes(channelSearch.trim().toLocaleLowerCase())
  )
  if (!visible) return null
  return (
    <div className='space-y-2'>
      <Label htmlFor={`${id}-channels`}>
        {t('settings.downstream.keys.access.channelScope')}
      </Label>
      <Select
        value={selectedChannels === undefined ? 'all' : 'selected'}
        onValueChange={(mode) => onChange(mode === 'selected' ? [] : undefined)}
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
          <ScopePickerToolbar
            selectedCount={new Set(selectedChannels).size}
            search={channelSearch}
            onSearchChange={setChannelSearch}
            onlySelected={onlySelectedChannels}
            onOnlySelectedChange={setOnlySelectedChannels}
            onLocateSelected={() => {
              setChannelSearch('')
              setOnlySelectedChannels(true)
              focus.focusSelected()
            }}
          />
          <div
            ref={focus.listRef}
            className='max-h-48 space-y-1 overflow-y-auto rounded-lg border p-2'
          >
            {filteredChannels.map((channel) => (
              <Label
                key={channel.id}
                className='flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 font-normal'
              >
                <Checkbox
                  checked={selectedChannels.includes(channel.id)}
                  onCheckedChange={(checked) =>
                    onChange(
                      checked
                        ? [...selectedChannels, channel.id]
                        : selectedChannels.filter((item) => item !== channel.id)
                    )
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
              filteredChannels.length === 0 && (
                <p className='text-muted-foreground p-2 text-sm'>
                  {t('settings.downstream.keys.picker.noMatches')}
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
                      onChange(
                        selectedChannels.filter((item) => item !== channelId)
                      )
                    }
                  />
                  {t('settings.downstream.keys.access.unavailableChannel', {
                    id: channelId,
                  })}
                </Label>
              ))}
          </div>
        </div>
      )}
    </div>
  )
}
