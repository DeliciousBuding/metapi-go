import { request } from './transport'

export type ImportedUpstreamInventory = {
  items: Array<{
    id: number
    name: string
    originKey: string
    dialect: string
    baseUrl: string
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
