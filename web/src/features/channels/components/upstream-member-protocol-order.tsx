import { ArrowDown, ArrowUp } from 'lucide-react'
import type { ControllerRenderProps } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import { upstreamProtocols } from '../lib/upstream-config'
import { UpstreamCapabilityPicker } from './upstream-capability-picker'

export function MemberProtocolOrder({
  field,
  protocols,
  pending,
}: {
  field: Pick<
    ControllerRenderProps<{ protocolOrder: number[] }, 'protocolOrder'>,
    'value' | 'onChange' | 'ref'
  >
  protocols: number
  pending: boolean
}) {
  const { t } = useTranslation()
  const inherited = !field.value.length
  const allowed = upstreamProtocols.filter(
    (protocol) => protocols & protocol.bit
  )
  const orderedGroups = [
    {
      key: 'conversation',
      protocols: field.value.filter((bit) =>
        upstreamProtocols.some((p) => p.bit === bit && p.convertible)
      ),
    },
    {
      key: 'video',
      protocols: field.value.filter((bit) =>
        upstreamProtocols.some((p) => p.bit === bit && p.group === 'video')
      ),
    },
  ]
  function protocolName(bit: number) {
    const protocol = upstreamProtocols.find((p) => p.bit === bit)
    return protocol
      ? t(`channels.capabilities.names.${protocol.key}`)
      : String(bit)
  }
  function move(ordered: number[], index: number, offset: number) {
    const next = [...ordered]
    ;[next[index], next[index + offset]] = [next[index + offset], next[index]]
    field.onChange([
      ...next,
      ...field.value.filter((bit) => !ordered.includes(bit)),
    ])
  }
  return (
    <FormItem>
      <FormLabel>{t('channels.upstream.protocolOrder')}</FormLabel>
      <FormControl>
        <div
          role='group'
          aria-label={t('channels.upstream.protocolOrder')}
          tabIndex={-1}
          ref={field.ref}
          className='space-y-3'
        >
          <label className='flex items-center justify-between gap-3 text-sm'>
            <span>{t('channels.upstream.inheritProtocols')}</span>
            <Switch
              checked={inherited}
              disabled={pending}
              onCheckedChange={(checked) =>
                field.onChange(
                  checked ? [] : allowed.map((protocol) => protocol.bit)
                )
              }
            />
          </label>
          {!inherited && (
            <>
              <UpstreamCapabilityPicker
                value={field.value}
                available={allowed.map((protocol) => protocol.bit)}
                onChange={field.onChange}
                disabled={pending}
                minSelected={1}
              />
              {orderedGroups
                .filter((group) => group.protocols.length > 1)
                .map(({ key, protocols: ordered }) => (
                  <ol
                    key={key}
                    aria-label={t(`channels.capabilities.orderGroups.${key}`)}
                    className='divide-y rounded-lg border'
                  >
                    {ordered.map((bit, index) => (
                      <li
                        key={bit}
                        className='flex items-center gap-3 px-3 py-1.5'
                      >
                        <span className='text-muted-foreground text-xs tabular-nums'>
                          {index + 1}
                        </span>
                        <span className='flex-1 text-sm'>
                          {protocolName(bit)}
                        </span>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon-sm'
                          disabled={pending || index === 0}
                          aria-label={t('channels.upstream.moveUp', {
                            name: protocolName(bit),
                          })}
                          onClick={() => move(ordered, index, -1)}
                        >
                          <ArrowUp className='size-3.5' />
                        </Button>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon-sm'
                          disabled={pending || index === ordered.length - 1}
                          aria-label={t('channels.upstream.moveDown', {
                            name: protocolName(bit),
                          })}
                          onClick={() => move(ordered, index, 1)}
                        >
                          <ArrowDown className='size-3.5' />
                        </Button>
                      </li>
                    ))}
                  </ol>
                ))}
            </>
          )}
        </div>
      </FormControl>
      <FormMessage />
    </FormItem>
  )
}
