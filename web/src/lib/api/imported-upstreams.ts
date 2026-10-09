import { request } from './transport'

type ImportedEndpoint = {
  url: string
  auth: 'bearer' | 'x-api-key' | 'x-goog-api-key'
  modelPath?: boolean
  profile?: 'codex' | 'claudecode'
}

export type ImportedCredential = {
  id: number
  name: string
  enabled: boolean
  kind: 'api_key' | 'oauth'
  expiresAt?: number
  canRefresh: boolean
}

export type ImportedCredentialUpdate = {
  enabled?: boolean
  apiKey?: string
  oauth?: {
    accessToken: string
    refreshToken?: string
    clientId?: string
    expiresAt?: number
    idToken?: string
    accountId?: string
  }
}

export type ImportedUpstreamInventory = {
  items: Array<{
    id: number
    name: string
    originKey: string
    dialect: string
    provider?: string
    baseUrl: string
    endpointConfig?: {
      chat?: ImportedEndpoint
      responses?: ImportedEndpoint
      messages?: ImportedEndpoint
      gemini?: ImportedEndpoint
    }
    enabled: boolean
    credentialCount: number
    modelCount: number
  }>
  members: Array<{
    id: number
    groupId: number
    groupName: string
    routeId: number
    channelId: number
    modelName: string
    credentialName: string
    credentialId?: number
    credentialKind?: 'api_key' | 'oauth'
    credentialEnabled: boolean
    protocols: number
    protocolOrder?: number[]
    mode: string
    activeItemId: number
    priority: number
    weight: number
    cooldownUntil?: string | null
    cooldownReasonCode?: string | null
    successCount?: number
    failCount?: number
  }>
}

export const importedUpstreamsApi = {
  getImportedCredentials: (channelId: number) =>
    request<{ items: ImportedCredential[] }>(
      `/api/imported-upstreams/${channelId}/credentials`
    ),
  updateImportedCredential: (id: number, input: ImportedCredentialUpdate) =>
    request<{ success: boolean; id: number }>(
      `/api/imported-upstreams/credentials/${id}`,
      { method: 'PATCH', body: JSON.stringify(input) }
    ),
  getImportedUpstreams: () =>
    request<ImportedUpstreamInventory>('/api/imported-upstreams'),
  setImportedUpstreamEnabled: (id: number, enabled: boolean) =>
    request<{ success: boolean; id: number; enabled: boolean }>(
      `/api/imported-upstreams/${id}`,
      { method: 'PATCH', body: JSON.stringify({ enabled }) }
    ),
}
