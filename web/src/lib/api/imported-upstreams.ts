import { request } from './transport'

type ImportedEndpoint = { url: string; auth: 'bearer' | 'x-api-key' | 'x-goog-api-key'; modelPath?: boolean }

export type ImportedUpstreamInventory = {
  items: Array<{
    id: number
    name: string
    originKey: string
    dialect: string
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
  getImportedUpstreams: () =>
    request<ImportedUpstreamInventory>('/api/imported-upstreams'),
  setImportedUpstreamEnabled: (id: number, enabled: boolean) =>
    request<{ success: boolean; id: number; enabled: boolean }>(
      `/api/imported-upstreams/${id}`,
      { method: 'PATCH', body: JSON.stringify({ enabled }) }
    ),
}
