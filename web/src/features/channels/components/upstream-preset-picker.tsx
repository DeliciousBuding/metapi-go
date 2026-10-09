import { Check, Settings2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'
import {
  getConnectionPresetIcon,
  getConnectionPresetNameKey,
  getPlatformDefinition,
} from '@/lib/platform-catalog'
import { cn } from '@/lib/utils'

import { upstreamProtocols } from '../lib/upstream-config'

function PresetIdentity(props: { preset: UpstreamPreset; compact?: boolean }) {
  const { t } = useTranslation()
  const nameKey = getConnectionPresetNameKey(props.preset.id)
  const name = nameKey ? t(nameKey) : props.preset.name
  const icon =
    getPlatformDefinition(props.preset.provider)?.icon ??
    getConnectionPresetIcon(props.preset.id)
  return (
    <>
      <span
        aria-hidden='true'
        className='flex size-9 shrink-0 items-center justify-center'
      >
        <BrandGlyph icon={icon} size={26} fallbackText={name} />
      </span>
      <span
        className={cn(
          'min-w-0 flex-1 text-left',
          props.compact
            ? 'flex flex-wrap items-center gap-x-3 gap-y-1'
            : 'space-y-1'
        )}
      >
        <span className='block text-sm font-medium'>{name}</span>
        <span className='flex flex-wrap gap-1'>
          {upstreamProtocols
            .filter((protocol) => props.preset.protocols.includes(protocol.key))
            .map((protocol) => (
              <Badge
                key={protocol.key}
                variant='secondary'
                className='px-1.5 py-0 text-xs font-normal'
              >
                {protocol.name}
              </Badge>
            ))}
        </span>
      </span>
    </>
  )
}

export function UpstreamPresetPicker(props: {
  presets: UpstreamPreset[]
  selectedId: string | null
  manual: boolean
  disabled: boolean
  onSelect: (preset: UpstreamPreset, name: string) => void
  onCustom: () => void
}) {
  const { t } = useTranslation()
  const [group, setGroup] = useState('common')
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState(true)
  const selected = props.presets.find(
    (preset) => preset.id === props.selectedId
  )
  const query = search.trim().toLowerCase()
  const labels = {
    common: t('channels.create.groups.common'),
    coding: t('channels.create.groups.coding'),
    gateway: t('channels.create.groups.gateway'),
    other: t('channels.create.groups.other'),
  }
  const visible = props.presets.filter((preset) => {
    const nameKey = getConnectionPresetNameKey(preset.id)
    const name = nameKey ? t(nameKey) : preset.name
    if (query) {
      return `${name} ${preset.name} ${preset.label} ${preset.provider} ${preset.defaultUrl}`
        .toLowerCase()
        .includes(query)
    }
    return group === 'common'
      ? preset.group !== 'other'
      : preset.group === group
  })

  if (!expanded && (selected || props.manual)) {
    return (
      <section
        className='flex items-center gap-3 rounded-xl border px-3 py-2.5'
        aria-label={t('channels.create.preset')}
      >
        {selected ? (
          <PresetIdentity preset={selected} compact />
        ) : (
          <span className='flex min-w-0 flex-1 items-center gap-3 text-sm font-medium'>
            <Settings2 className='size-4' aria-hidden='true' />
            {t('channels.create.custom')}
          </span>
        )}
        <Button
          type='button'
          variant='ghost'
          size='sm'
          disabled={props.disabled}
          onClick={() => setExpanded(true)}
        >
          {t('channels.create.change')}
        </Button>
      </section>
    )
  }

  return (
    <section className='space-y-3' aria-label={t('channels.create.preset')}>
      <Tabs value={group} onValueChange={(value) => setGroup(String(value))}>
        <TabsList className='w-full justify-start overflow-x-auto'>
          {Object.entries(labels).map(([value, label]) => (
            <TabsTrigger key={value} value={value} disabled={props.disabled}>
              {label}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      <Input
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        placeholder={t('channels.create.search')}
        aria-label={t('channels.create.search')}
        disabled={props.disabled}
      />
      <div className='grid max-h-[50vh] gap-2 overflow-y-auto pr-1 sm:grid-cols-2'>
        {visible.map((preset) => {
          const nameKey = getConnectionPresetNameKey(preset.id)
          const name = nameKey ? t(nameKey) : preset.name
          const selected = preset.id === props.selectedId
          return (
            <Button
              key={preset.id}
              type='button'
              variant='outline'
              disabled={props.disabled}
              aria-pressed={selected}
              aria-label={t('channels.create.usePreset', {
                name: preset.label,
              })}
              className={cn(
                'h-auto min-h-18 justify-start gap-3 rounded-xl px-3 py-2.5 whitespace-normal',
                selected && 'border-primary bg-primary/5'
              )}
              onClick={() => {
                setExpanded(false)
                props.onSelect(preset, name)
              }}
            >
              <PresetIdentity preset={preset} />
              {selected && (
                <Check
                  className='text-primary size-4 shrink-0'
                  aria-hidden='true'
                />
              )}
            </Button>
          )
        })}
      </div>
      {!visible.length && (query || props.presets.length > 0) && (
        <p className='text-muted-foreground text-sm'>
          {t('channels.create.noPresets')}
        </p>
      )}
      <Button
        type='button'
        variant='ghost'
        size='sm'
        disabled={props.disabled}
        onClick={() => {
          setExpanded(false)
          props.onCustom()
        }}
      >
        <Settings2 className='size-4' aria-hidden='true' />
        {t('channels.create.custom')}
      </Button>
    </section>
  )
}
