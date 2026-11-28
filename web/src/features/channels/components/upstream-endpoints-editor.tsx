import {
  AudioLines,
  Braces,
  ChevronDown,
  ImageIcon,
  MessageSquare,
  Search,
  Video,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

import {
  upstreamProtocolGroups,
  upstreamProtocols,
} from '../lib/upstream-config'
import {
  endpointValidation,
  type EndpointEditorProps,
} from './upstream-endpoint-config'
import { EndpointRow } from './upstream-endpoint-row'

const groupIcons = {
  conversation: MessageSquare,
  retrieval: Search,
  image: ImageIcon,
  audio: AudioLines,
  video: Video,
  native: Braces,
}

export function UpstreamEndpointsEditor(props: EndpointEditorProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<
    Partial<Record<(typeof upstreamProtocolGroups)[number], boolean>>
  >({})
  const form = useFormContext()
  const showValidation = !!form?.formState.errors.endpointConfig
  const validationAttempt = form?.formState.submitCount ?? 0
  const { invalidKeys, invalidGroups } = endpointValidation(
    props.value,
    showValidation
  )
  useEffect(() => {
    if (!invalidGroups) return
    setExpanded((previous) => ({
      ...previous,
      ...Object.fromEntries(
        invalidGroups.split(',').map((group) => [group, true])
      ),
    }))
  }, [invalidGroups, validationAttempt])
  const firstGroup =
    upstreamProtocols.find((p) => props.value[p.key])?.group ?? 'conversation'
  return (
    <div className='space-y-3'>
      {upstreamProtocolGroups.map((group) => {
        const protocols = upstreamProtocols.filter((p) => p.group === group)
        const count = protocols.filter((p) => props.value[p.key]).length
        const Icon = groupIcons[group]
        return (
          <Collapsible
            key={group}
            open={expanded[group] ?? group === firstGroup}
            onOpenChange={(open) =>
              setExpanded((previous) => ({ ...previous, [group]: open }))
            }
            className='rounded-xl border'
          >
            <CollapsibleTrigger className='group focus-visible:outline-ring flex w-full items-center gap-3 rounded-xl px-4 py-3 text-left text-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-2'>
              <Icon
                className='text-muted-foreground size-4 shrink-0'
                aria-hidden='true'
              />
              <span className='flex-1'>
                {t(`channels.capabilities.groups.${group}`)}
              </span>{' '}
              <Badge variant={count ? 'secondary' : 'outline'}>
                {t('channels.capabilities.configured', {
                  count,
                  total: protocols.length,
                })}
              </Badge>
              <ChevronDown
                className='size-4 transition-transform group-data-[panel-open]:rotate-180'
                aria-hidden='true'
              />
            </CollapsibleTrigger>
            <CollapsibleContent keepMounted className='divide-y border-t'>
              {protocols.map((protocol) => (
                <EndpointRow
                  key={protocol.key}
                  {...props}
                  protocol={protocol}
                  invalid={invalidKeys.has(protocol.key)}
                  validationAttempt={validationAttempt}
                />
              ))}
            </CollapsibleContent>
          </Collapsible>
        )
      })}
    </div>
  )
}
