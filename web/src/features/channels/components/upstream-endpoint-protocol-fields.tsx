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
import type { ImportedEndpoint } from '@/lib/api/imported-upstreams'

import { availableProfiles, connectionSchema } from '../lib/upstream-config'
import { profilePatch, type EndpointProtocol } from './upstream-endpoint-config'
import { UpstreamModelWireEditor } from './upstream-model-wire-editor'

export function EndpointProtocolFields({
  id,
  endpoint,
  protocol,
  provider,
  disabled,
  invalid,
  validationAttempt,
  update,
}: {
  id: string
  endpoint: ImportedEndpoint
  protocol: EndpointProtocol
  provider: string
  disabled?: boolean
  invalid: boolean
  validationAttempt: number
  update: (patch: Partial<ImportedEndpoint>) => void
}) {
  const { t } = useTranslation()
  const profiles = availableProfiles(protocol.key, provider)
  let endpointUrlLabel = 'channels.upstream.endpointUrl'
  if (endpoint.profile === 'bedrock') {
    endpointUrlLabel = 'channels.upstream.modelPrefixUrl'
  }
  if (
    endpoint.profile === 'vertex' ||
    endpoint.profile === 'vertex-anthropic'
  ) {
    endpointUrlLabel = 'channels.upstream.vertexModelPrefixUrl'
  }
  const optionalAuth =
    endpoint.profile === 'ollama' || endpoint.profile === 'ollama-messages'
  const authModes =
    protocol.authModes ??
    (optionalAuth
      ? ['bearer', 'none']
      : ['bearer', 'x-api-key', 'x-goog-api-key'])
  function authLabel(value: unknown) {
    if (value === 'none') return t('channels.upstream.noAuthentication')
    return value === 'bearer' ? 'Bearer' : String(value ?? '')
  }
  return (
    <div
      id={`${id}-${protocol.key}-editor`}
      className='space-y-3 border-t px-4 py-3'
    >
      <label
        className='block space-y-1.5 text-sm font-medium'
        htmlFor={`${id}-${protocol.key}-url`}
      >
        <span>{t(endpointUrlLabel)}</span>
        <Input
          id={`${id}-${protocol.key}-url`}
          value={endpoint.url}
          aria-invalid={
            (invalid &&
              !connectionSchema.shape.baseUrl.safeParse(endpoint.url)
                .success) ||
            undefined
          }
          disabled={disabled}
          onChange={(event) => update({ url: event.target.value })}
          placeholder={
            endpoint.profile === 'bedrock'
              ? 'https://bedrock-runtime.us-east-1.amazonaws.com/model'
              : 'https://…'
          }
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
            disabled={
              disabled ||
              authModes.length === 1 ||
              (!!endpoint.profile && !optionalAuth)
            }
            onValueChange={(value) =>
              value && update({ auth: value as ImportedEndpoint['auth'] })
            }
          >
            <SelectTrigger
              className='w-full'
              aria-labelledby={`${id}-${protocol.key}-auth`}
            >
              <SelectValue>{authLabel}</SelectValue>
            </SelectTrigger>
            <SelectContent aria-labelledby={`${id}-${protocol.key}-auth`}>
              {authModes.map((auth) => (
                <SelectItem key={auth} value={auth}>
                  {authLabel(auth)}
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
              value={endpoint.profile ?? protocol.requiredProfile ?? 'generic'}
              disabled={disabled || !!protocol.requiredProfile}
              onValueChange={(value) => {
                if (value) update(profilePatch(value, endpoint))
              }}
            >
              <SelectTrigger
                className='w-full'
                aria-labelledby={`${id}-${protocol.key}-profile`}
                aria-describedby={
                  endpoint.profile === 'moonshot'
                    ? `${id}-moonshot-json`
                    : undefined
                }
              >
                <SelectValue>
                  {(value) =>
                    value === 'generic'
                      ? t('channels.upstream.generic')
                      : t(`channels.upstream.profiles.${value}`)
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent aria-labelledby={`${id}-${protocol.key}-profile`}>
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
      {endpoint.profile === 'moonshot' && (
        <p
          id={`${id}-moonshot-json`}
          role='note'
          className='text-muted-foreground text-xs'
        >
          {t('channels.upstream.moonshotJsonMode')}
        </p>
      )}
      {endpoint.profile === 'opencode-go' && (
        <UpstreamModelWireEditor
          value={endpoint.modelWireUrls}
          onChange={(modelWireUrls) => update({ modelWireUrls })}
          disabled={disabled}
          invalid={invalid}
          validationAttempt={validationAttempt}
        />
      )}
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
              (invalid &&
                (!endpoint.requestModel?.trim() ||
                  endpoint.requestModel.length > 255)) ||
              undefined
            }
            disabled={disabled}
            onChange={(event) => update({ requestModel: event.target.value })}
          />
          <p
            id={`${id}-${protocol.key}-request-model-hint`}
            className='text-muted-foreground'
          >
            {t('channels.capabilities.requestModelHint')}
          </p>
        </div>
      )}
      {(protocol.key === 'gemini' || protocol.key === 'geminiEmbeddings') && (
        <label className='flex items-center gap-2 text-sm'>
          <Checkbox
            checked={endpoint.modelPath ?? false}
            disabled={disabled || endpoint.profile === 'vertex'}
            onCheckedChange={(checked) =>
              update({ modelPath: checked === true })
            }
          />
          {t('channels.upstream.modelPath')}
        </label>
      )}
    </div>
  )
}
