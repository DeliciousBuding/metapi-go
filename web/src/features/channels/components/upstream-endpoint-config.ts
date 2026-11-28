import type {
  ImportedEndpoint,
  ImportedEndpointConfig,
} from '@/lib/api/imported-upstreams'

import {
  connectionSchema,
  upstreamProtocols,
  upstreamProtocolGroups,
} from '../lib/upstream-config'

export type EndpointEditorProps = {
  value: ImportedEndpointConfig
  onChange: (value: ImportedEndpointConfig) => void
  provider: string
  disabled?: boolean
}

export type EndpointProtocol = (typeof upstreamProtocols)[number]

export function endpointValidation(
  value: ImportedEndpointConfig,
  showValidation: boolean
) {
  const invalidKeys = new Set(
    upstreamProtocols
      .filter((protocol) => {
        const endpoint = value[protocol.key]
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
  return { invalidKeys, invalidGroups }
}

export function profilePatch(
  value: string,
  endpoint: ImportedEndpoint
): Partial<ImportedEndpoint> {
  const modelPath = ['bedrock', 'vertex', 'vertex-anthropic'].includes(value)
  const previousModelPath = ['bedrock', 'vertex', 'vertex-anthropic'].includes(
    endpoint.profile ?? ''
  )
  return {
    profile:
      value === 'generic' ? undefined : (value as ImportedEndpoint['profile']),
    ...(value !== 'generic' || endpoint.auth === 'none'
      ? { auth: 'bearer' }
      : {}),
    ...(modelPath || previousModelPath
      ? { modelPath: modelPath || undefined }
      : {}),
    ...(value !== 'codex-image' ? { requestModel: undefined } : {}),
    modelWireUrls:
      value === 'opencode-go'
        ? (endpoint.modelWireUrls ?? {
            responses: '',
            messages: '',
          })
        : undefined,
  }
}
