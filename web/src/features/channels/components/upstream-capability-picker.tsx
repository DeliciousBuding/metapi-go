import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

import {
  upstreamProtocolGroups,
  upstreamProtocols,
} from '../lib/upstream-config'

export function UpstreamCapabilityPicker(props: {
  value: number[]
  available: number[]
  onChange: (value: number[]) => void
  disabled?: boolean
  minSelected?: number
  unavailableLabel?: string
}) {
  const { t } = useTranslation()
  const visible = upstreamProtocols.filter(
    (p) => props.available.includes(p.bit) || props.value.includes(p.bit)
  )
  const firstGroup =
    visible.find((p) => props.value.includes(p.bit))?.group ?? visible[0]?.group
  return (
    <div className='space-y-2'>
      {upstreamProtocolGroups.map((group) => {
        const protocols = visible.filter((p) => p.group === group)
        if (!protocols.length) return null
        const count = protocols.filter((p) =>
          props.value.includes(p.bit)
        ).length
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
                      checked={props.value.includes(protocol.bit)}
                      disabled={
                        props.disabled ||
                        (props.value.includes(protocol.bit) &&
                          props.value.length <= (props.minSelected ?? 0))
                      }
                      onCheckedChange={(checked) =>
                        props.onChange(
                          checked
                            ? [...props.value, protocol.bit]
                            : props.value.filter((bit) => bit !== protocol.bit)
                        )
                      }
                    />
                    {t(`channels.capabilities.names.${protocol.key}`)}{' '}
                    {!props.available.includes(protocol.bit) && (
                      <span className='text-warning-soft-fg'>
                        {props.unavailableLabel ??
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
