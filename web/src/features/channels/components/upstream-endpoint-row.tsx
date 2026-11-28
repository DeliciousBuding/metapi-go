import { ChevronDown } from 'lucide-react'
import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import type { ImportedEndpoint } from '@/lib/api/imported-upstreams'

import type {
  EndpointEditorProps,
  EndpointProtocol,
} from './upstream-endpoint-config'
import { EndpointProtocolFields } from './upstream-endpoint-protocol-fields'

export function EndpointRow(
  props: EndpointEditorProps & {
    protocol: EndpointProtocol
    invalid: boolean
    validationAttempt: number
  }
) {
  const { t } = useTranslation()
  const id = useId()
  const protocol = props.protocol
  const endpoint = props.value[protocol.key]
  const [editing, setEditing] = useState(false)
  useEffect(() => {
    if (props.invalid) setEditing(true)
  }, [props.invalid, props.validationAttempt])
  function update(patch: Partial<ImportedEndpoint>) {
    if (endpoint) {
      props.onChange({
        ...props.value,
        [protocol.key]: { ...endpoint, ...patch },
      })
    }
  }
  return (
    <div
      role='group'
      aria-label={t(`channels.capabilities.names.${protocol.key}`)}
      className='min-w-0'
    >
      <div className='flex min-w-0 items-center gap-3 px-4 py-2.5'>
        <label className='flex shrink-0 cursor-pointer items-center gap-3 text-sm font-medium'>
          <Checkbox
            checked={!!endpoint}
            disabled={props.disabled}
            onCheckedChange={(checked) => {
              const next = { ...props.value }
              setEditing(checked === true)
              if (checked) {
                let auth: ImportedEndpoint['auth'] = 'bearer'
                if (protocol.key === 'messages') {
                  auth = 'x-api-key'
                }
                if (
                  protocol.key === 'gemini' ||
                  protocol.key === 'geminiEmbeddings'
                ) {
                  auth = 'x-goog-api-key'
                }
                next[protocol.key] = {
                  url: '',
                  auth,
                  ...(protocol.requiredProfile
                    ? { profile: protocol.requiredProfile }
                    : {}),
                }
              } else delete next[protocol.key]
              props.onChange(next)
            }}
          />
          {t(`channels.capabilities.names.${protocol.key}`)}
        </label>
        {endpoint && (
          <Button
            type='button'
            variant='ghost'
            size='sm'
            className='text-muted-foreground min-w-0 flex-1 justify-end gap-2 px-1 text-sm font-normal'
            aria-label={t('channels.capabilities.configureEndpoint', {
              name: t(`channels.capabilities.names.${protocol.key}`),
            })}
            aria-expanded={editing}
            aria-controls={editing ? `${id}-${protocol.key}-editor` : undefined}
            aria-invalid={props.invalid || undefined}
            disabled={props.disabled}
            onClick={() => setEditing((current) => !current)}
          >
            <span className='truncate' title={endpoint.url}>
              {endpoint.url || t('channels.upstream.endpointUrl')}
            </span>
            <ChevronDown
              className={`size-4 shrink-0 transition-transform ${editing ? 'rotate-180' : ''}`}
              aria-hidden='true'
            />
          </Button>
        )}
      </div>
      {endpoint && editing && (
        <EndpointProtocolFields
          id={id}
          endpoint={endpoint}
          protocol={protocol}
          provider={props.provider}
          disabled={props.disabled}
          invalid={props.invalid}
          validationAttempt={props.validationAttempt}
          update={update}
        />
      )}
    </div>
  )
}
