import { ChevronDown } from 'lucide-react'
import type { ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { formLabelIdFor } from '@/components/ui/form'
import { cn } from '@/lib/utils'

import {
  upstreamProtocolGroups,
  upstreamProtocols,
} from '../lib/upstream-config'

type CapabilityPickerProps = Omit<ComponentProps<'div'>, 'onChange'> & {
  value: number[]
  available: number[]
  onChange: (value: number[]) => void
  disabled?: boolean
  minSelected?: number
  unavailableLabel?: string
}

export function UpstreamCapabilityPicker({
  value,
  available,
  onChange,
  disabled,
  minSelected = 0,
  unavailableLabel,
  ...props
}: CapabilityPickerProps) {
  const { t } = useTranslation()
  const visible = upstreamProtocols.filter(
    (p) => available.includes(p.bit) || value.includes(p.bit)
  )
  const firstGroup =
    visible.find((p) => value.includes(p.bit))?.group ?? visible[0]?.group
  return (
    <div
      {...props}
      role='group'
      aria-labelledby={
        props['aria-labelledby'] ??
        (props.id ? formLabelIdFor(props.id) : undefined)
      }
      className={cn('space-y-2', props.className)}
    >
      {upstreamProtocolGroups.map((group) => {
        const protocols = visible.filter((p) => p.group === group)
        if (!protocols.length) return null
        const count = protocols.filter((p) => value.includes(p.bit)).length
        return (
          <Collapsible
            key={group}
            defaultOpen={group === firstGroup}
            className='rounded-lg border'
          >
            <CollapsibleTrigger className='group focus-visible:outline-ring flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm font-medium focus-visible:outline-2'>
              <span className='flex-1'>
                {t(`channels.capabilities.groups.${group}`)}
              </span>{' '}
              <Badge variant={count ? 'secondary' : 'outline'}>
                {t('channels.capabilities.selected', {
                  count,
                  total: protocols.length,
                })}
              </Badge>
              <ChevronDown
                className='size-4 transition-transform group-data-[panel-open]:rotate-180'
                aria-hidden='true'
              />
            </CollapsibleTrigger>
            <CollapsibleContent keepMounted className='border-t p-3'>
              <div className='flex flex-wrap gap-2'>
                {protocols.map((protocol) => (
                  <label
                    key={protocol.bit}
                    className='flex items-center gap-2 rounded-full border px-3 py-2 text-sm'
                  >
                    <Checkbox
                      checked={value.includes(protocol.bit)}
                      disabled={
                        disabled ||
                        (value.includes(protocol.bit) &&
                          value.length <= minSelected)
                      }
                      onCheckedChange={(checked) =>
                        onChange(
                          checked
                            ? [...value, protocol.bit]
                            : value.filter((bit) => bit !== protocol.bit)
                        )
                      }
                    />
                    {t(`channels.capabilities.names.${protocol.key}`)}{' '}
                    {!available.includes(protocol.bit) && (
                      <span className='text-warning-soft-fg'>
                        {unavailableLabel ??
                          t('channels.capabilities.notConfigured')}
                      </span>
                    )}
                  </label>
                ))}
              </div>
            </CollapsibleContent>
          </Collapsible>
        )
      })}
    </div>
  )
}
