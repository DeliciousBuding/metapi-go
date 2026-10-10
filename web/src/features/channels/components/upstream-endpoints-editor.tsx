import {
  AudioLines,
  ChevronDown,
  ImageIcon,
  MessageSquare,
  Search,
  Video,
} from 'lucide-react'
import { useEffect, useId, useState } from 'react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type {
  ImportedEndpoint,
  ImportedEndpointConfig,
} from '@/lib/api/imported-upstreams'

import {
  availableProfiles,
  connectionSchema,
  upstreamProtocolGroups,
  upstreamProtocols,
} from '../lib/upstream-config'

type EndpointEditorProps = {
  value: ImportedEndpointConfig
  onChange: (value: ImportedEndpointConfig) => void
  provider: string
  disabled?: boolean
}
const groupIcons = {
  conversation: MessageSquare,
  retrieval: Search,
  image: ImageIcon,
  audio: AudioLines,
  video: Video,
}

export function UpstreamEndpointsEditor(props: EndpointEditorProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<
    Partial<Record<(typeof upstreamProtocolGroups)[number], boolean>>
  >({})
  const form = useFormContext()
  const showValidation = !!form?.formState.errors.endpointConfig
  const validationAttempt = form?.formState.submitCount ?? 0
  const invalidKeys = new Set(
    upstreamProtocols
      .filter((protocol) => {
        const endpoint = props.value[protocol.key]
        return (
          showValidation &&
          endpoint &&
          !connectionSchema.shape.endpointConfig.safeParse({
            [protocol.key]: endpoint,
          }).success
        )
      })
      .map((protocol) => protocol.key)
  )
  const invalidGroups = upstreamProtocolGroups
    .filter((group) =>
      upstreamProtocols.some(
        (protocol) => protocol.group === group && invalidKeys.has(protocol.key)
      )
    )
    .join(',')
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

function EndpointRow(
  props: EndpointEditorProps & {
    protocol: (typeof upstreamProtocols)[number]
    invalid: boolean
    validationAttempt: number
  }
) {
  const { t } = useTranslation()
  const id = useId()
  const protocol = props.protocol
  const profiles = availableProfiles(protocol.key, props.provider)
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
        <div
          id={`${id}-${protocol.key}-editor`}
          className='space-y-3 border-t px-4 py-3'
        >
          <label
            className='block space-y-1.5 text-sm font-medium'
            htmlFor={`${id}-${protocol.key}-url`}
          >
            <span>{t('channels.upstream.endpointUrl')}</span>
            <Input
              id={`${id}-${protocol.key}-url`}
              value={endpoint.url}
              aria-invalid={
                (props.invalid &&
                  !connectionSchema.shape.baseUrl.safeParse(endpoint.url)
                    .success) ||
                undefined
              }
              disabled={props.disabled}
              onChange={(event) => update({ url: event.target.value })}
              placeholder='https://…'
            />
          </label>
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='space-y-1.5'>
              <label
                className='text-sm font-medium'
                id={`${id}-${protocol.key}-auth`}
              >
                {t('channels.upstream.auth')}
              </label>
              <Select
                value={endpoint.auth}
                disabled={props.disabled || !!endpoint.profile}
                onValueChange={(value) =>
                  value && update({ auth: value as ImportedEndpoint['auth'] })
                }
              >
                <SelectTrigger
                  className='w-full'
                  aria-labelledby={`${id}-${protocol.key}-auth`}
                >
                  <SelectValue>
                    {(value) =>
                      value === 'bearer' ? 'Bearer' : String(value ?? '')
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {['bearer', 'x-api-key', 'x-goog-api-key'].map((auth) => (
                    <SelectItem key={auth} value={auth}>
                      {auth === 'bearer' ? 'Bearer' : auth}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {(['chat', 'responses', 'messages'].includes(protocol.key) ||
              profiles.length > 0 ||
              endpoint.profile) && (
              <div className='space-y-1.5'>
                <label
                  className='text-sm font-medium'
                  id={`${id}-${protocol.key}-profile`}
                >
                  {t('channels.upstream.profile')}
                </label>
                <Select
                  value={
                    endpoint.profile ?? protocol.requiredProfile ?? 'generic'
                  }
                  disabled={props.disabled || !!protocol.requiredProfile}
                  onValueChange={(value) => {
                    if (value) {
                      update({
                        profile:
                          value === 'generic'
                            ? undefined
                            : (value as ImportedEndpoint['profile']),
                        ...(value !== 'generic' ? { auth: 'bearer' } : {}),
                        ...(value !== 'codex-image'
                          ? { requestModel: undefined }
                          : {}),
                      })
                    }
                  }}
                >
                  <SelectTrigger
                    className='w-full'
                    aria-labelledby={`${id}-${protocol.key}-profile`}
                  >
                    <SelectValue>
                      {(value) =>
                        value === 'generic'
                          ? t('channels.upstream.generic')
                          : t(`channels.upstream.profiles.${value}`)
                      }
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {!protocol.requiredProfile && (
                      <SelectItem value='generic'>
                        {t('channels.upstream.generic')}
                      </SelectItem>
                    )}
                    {profiles.map((profile) => (
                      <SelectItem key={profile} value={profile}>
                        {t(`channels.upstream.profiles.${profile}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
          </div>
          {endpoint.profile === 'codex-image' && (
            <div className='space-y-1.5 text-sm'>
              <label
                className='font-medium'
                htmlFor={`${id}-${protocol.key}-request-model`}
              >
                {t('channels.capabilities.requestModel')}
              </label>
              <Input
                id={`${id}-${protocol.key}-request-model`}
                aria-describedby={`${id}-${protocol.key}-request-model-hint`}
                value={endpoint.requestModel ?? ''}
                aria-invalid={
                  (props.invalid &&
                    (!endpoint.requestModel?.trim() ||
                      endpoint.requestModel.length > 255)) ||
                  undefined
                }
                disabled={props.disabled}
                onChange={(event) =>
                  update({ requestModel: event.target.value })
                }
              />
              <p
                id={`${id}-${protocol.key}-request-model-hint`}
                className='text-muted-foreground'
              >
                {t('channels.capabilities.requestModelHint')}
              </p>
            </div>
          )}
          {(protocol.key === 'gemini' ||
            protocol.key === 'geminiEmbeddings') && (
            <label className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={endpoint.modelPath ?? false}
                disabled={props.disabled}
                onCheckedChange={(checked) =>
                  update({ modelPath: checked === true })
                }
              />
              {t('channels.upstream.modelPath')}
            </label>
          )}
        </div>
      )}
    </div>
  )
}
