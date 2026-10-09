import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
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

import { availableProfiles, upstreamProtocols } from '../lib/upstream-config'

export function UpstreamEndpointsEditor(props: {
  value: ImportedEndpointConfig
  onChange: (value: ImportedEndpointConfig) => void
  provider: string
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <div className='space-y-3'>
      {upstreamProtocols.map((protocol) => {
        const endpoint = props.value[protocol.key]
        function update(patch: Partial<ImportedEndpoint>) {
          if (endpoint) {
            props.onChange({
              ...props.value,
              [protocol.key]: { ...endpoint, ...patch },
            })
          }
        }
        return (
          <div key={protocol.key} className='rounded-xl border'>
            <label className='flex cursor-pointer items-center gap-3 px-4 py-3 text-sm font-medium'>
              <Checkbox
                checked={!!endpoint}
                disabled={props.disabled}
                onCheckedChange={(checked) => {
                  const next = { ...props.value }
                  if (checked) {
                    next[protocol.key] = {
                      url: '',
                      auth:
                        protocol.key === 'messages' ? 'x-api-key' : 'bearer',
                    }
                  } else delete next[protocol.key]
                  props.onChange(next)
                }}
              />
              {protocol.name}
            </label>
            {endpoint && (
              <div className='space-y-3 border-t px-4 py-3'>
                <label
                  className='block space-y-1.5 text-xs font-medium'
                  htmlFor={`${id}-${protocol.key}-url`}
                >
                  <span>{t('channels.upstream.endpointUrl')}</span>
                  <Input
                    id={`${id}-${protocol.key}-url`}
                    value={endpoint.url}
                    disabled={props.disabled}
                    onChange={(event) => update({ url: event.target.value })}
                    placeholder='https://…'
                  />
                </label>
                <div className='grid gap-3 sm:grid-cols-2'>
                  <div className='space-y-1.5'>
                    <label
                      className='text-xs font-medium'
                      id={`${id}-${protocol.key}-auth`}
                    >
                      {t('channels.upstream.auth')}
                    </label>
                    <Select
                      value={endpoint.auth}
                      disabled={props.disabled || !!endpoint.profile}
                      onValueChange={(value) =>
                        value &&
                        update({ auth: value as ImportedEndpoint['auth'] })
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
                        {['bearer', 'x-api-key', 'x-goog-api-key'].map(
                          (auth) => (
                            <SelectItem key={auth} value={auth}>
                              {auth === 'bearer' ? 'Bearer' : auth}
                            </SelectItem>
                          )
                        )}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className='space-y-1.5'>
                    <label
                      className='text-xs font-medium'
                      id={`${id}-${protocol.key}-profile`}
                    >
                      {t('channels.upstream.profile')}
                    </label>
                    <Select
                      value={endpoint.profile ?? 'generic'}
                      disabled={props.disabled}
                      onValueChange={(value) => {
                        if (value) {
                          update({
                            profile:
                              value === 'generic'
                                ? undefined
                                : (value as ImportedEndpoint['profile']),
                            ...(value !== 'generic' ? { auth: 'bearer' } : {}),
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
                        <SelectItem value='generic'>
                          {t('channels.upstream.generic')}
                        </SelectItem>
                        {availableProfiles(protocol.key, props.provider).map(
                          (profile) => (
                            <SelectItem key={profile} value={profile}>
                              {t(`channels.upstream.profiles.${profile}`)}
                            </SelectItem>
                          )
                        )}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                {protocol.key === 'gemini' && (
                  <label className='flex items-center gap-2 text-xs'>
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
      })}
    </div>
  )
}
