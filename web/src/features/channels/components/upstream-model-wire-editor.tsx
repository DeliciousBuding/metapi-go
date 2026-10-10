import { ChevronDown } from 'lucide-react'
import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import type { ImportedEndpoint } from '@/lib/api/imported-upstreams'

import { modelWireUrlSchema } from '../lib/upstream-config'

export function UpstreamModelWireEditor(props: {
  value: ImportedEndpoint['modelWireUrls']
  onChange: (value: NonNullable<ImportedEndpoint['modelWireUrls']>) => void
  disabled?: boolean
  invalid: boolean
  validationAttempt: number
}) {
  const { t } = useTranslation()
  const id = useId()
  const [open, setOpen] = useState(
    !props.value?.responses || !props.value?.messages
  )
  useEffect(() => {
    if (props.invalid) setOpen(true)
  }, [props.invalid, props.validationAttempt])
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='rounded-lg border'
    >
      <CollapsibleTrigger className='group focus-visible:outline-ring flex w-full items-center justify-between gap-3 rounded-lg px-3 py-2 text-left text-sm font-medium focus-visible:outline-2'>
        {t('channels.upstream.modelWireUrls.title')}
        <ChevronDown
          className='size-4 shrink-0 transition-transform group-data-[panel-open]:rotate-180'
          aria-hidden='true'
        />
      </CollapsibleTrigger>
      <CollapsibleContent className='space-y-3 border-t p-3'>
        {(['responses', 'messages'] as const).map((wire) => (
          <label
            key={wire}
            htmlFor={`${id}-${wire}`}
            className='block min-w-0 space-y-1.5 text-sm font-medium'
          >
            <span>{t(`channels.upstream.modelWireUrls.${wire}`)}</span>
            <Input
              id={`${id}-${wire}`}
              value={props.value?.[wire] ?? ''}
              disabled={props.disabled}
              placeholder='https://…'
              aria-invalid={
                (props.invalid &&
                  !modelWireUrlSchema.safeParse(props.value?.[wire]).success) ||
                undefined
              }
              onChange={(event) =>
                props.onChange({
                  responses: props.value?.responses ?? '',
                  messages: props.value?.messages ?? '',
                  [wire]: event.target.value,
                })
              }
            />
          </label>
        ))}
      </CollapsibleContent>
    </Collapsible>
  )
}
