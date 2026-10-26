import type {
  ImportedCredential,
  ImportedCredentialUpdate,
  ImportedEndpointConfig,
} from './imported-upstreams'
import { request } from './transport'

type UpstreamOwnership = 'native' | 'imported'
export type UpstreamGrant = {
  id: number
  modelId: number
  credentialId: number
  credentialName: string
  enabled: boolean
  protocols: number
  memberCount: number
  ownership: UpstreamOwnership
}
export type UpstreamModel = {
  id: number
  name: string
  enabled: boolean
  ownership: UpstreamOwnership
  grants: UpstreamGrant[]
}
export type UpstreamGroupMember = {
  id: number
  groupId: number
  grantId: number
  priority: number
  weight: number
  protocolOrder: number[]
  ownership: UpstreamOwnership
  modelName: string
  credentialName: string
  channelId: number
}
export type UpstreamGroup = {
  id: number
  name: string
  mode: 'failover' | 'manual'
  enabled: boolean
  activeMemberId: number
  ownership: UpstreamOwnership
  routeId: number
  modelPattern: string
  displayName: string
  members: UpstreamGroupMember[]
}
export type UpstreamMemberCreate = {
  grantId: number
  priority?: number
  weight?: number
  protocolOrder?: number[]
}
export type UpstreamGroupCreate = {
  name: string
  mode?: 'failover' | 'manual'
  enabled?: boolean
  route: {
    modelPattern: string
    displayName?: string
    routingStrategy?: string
  }
  members: UpstreamMemberCreate[]
  activeGrantId?: number
}
export type UpstreamConnectInput = {
  presetId: string
  /** Explicit non-empty subset of the product preset's endpoint keys. */
  protocols?: Array<keyof ImportedEndpointConfig>
  /** Complete custom endpoint configuration; mutually exclusive with protocols. */
  endpointConfig?: ImportedEndpointConfig
  apiKey?: string
  /** SA JSON key document when the preset uses a GCP service account. */
  gcpServiceAccount?: string
  /** GCP project for parameterized defaults (Vertex); omitted for custom URLs. */
  projectId?: string
  baseUrl?: string
  name?: string
  useSystemProxy?: boolean
  channelProxy?: string
  customHeaders?: string
  paramOverride?: string
}
export type UpstreamConnectResult = {
  id: number
  name: string
  enabled: true
  ownership: 'native'
  modelCount: number
  routeCount: number
  discovery: { status: 'discovered' | 'preset' | 'empty'; message?: string }
}

/** One discovery-origin model whose name vanished upstream (E batch). */
type UpstreamStaleModel = {
  id: number
  name: string
  enabled: boolean
  grantCount: number
  routeIds: number[]
}

/**
 * Read-only discovery diff for a channel's model catalog. basisToken binds the
 * catalog rows and the discovered set; apply requires it and refuses with 409
 * when either changed since the preview.
 */
export type UpstreamRefreshPreview = {
  channelId: number
  basisToken: string
  discovery: { status: string; message?: string }
  upstreamCount: number
  added: string[]
  stale: UpstreamStaleModel[]
  kept: string[]
  /** Discovery-origin rows already disabled and still absent upstream (history; apply no-ops on them). */
  offline: string[]
  manualOnly: string[]
  manualKept: string[]
  legacyNativeCount: number
}

export type UpstreamRefreshApplyResult = {
  success: boolean
  addedCount: number
  staleCount: number
  affectedRouteIds: number[]
}

export const upstreamCatalogApi = {
  connectUpstream: (input: UpstreamConnectInput) =>
    request<UpstreamConnectResult>('/api/imported-upstreams/connect', {
      method: 'POST',
      body: JSON.stringify(input),
      skipErrorHandler: true,
    }),
  createUpstreamCredential: (
    channelId: number,
    input: ImportedCredentialUpdate & { name: string }
  ) =>
    request<ImportedCredential>(
      `/api/imported-upstreams/${channelId}/credentials`,
      {
        method: 'POST',
        body: JSON.stringify(input),
      }
    ),
  refreshUpstreamCatalogPreview: (channelId: number) =>
    request<UpstreamRefreshPreview>(
      `/api/imported-upstreams/${channelId}/models/refresh-preview`,
      { method: 'POST', skipErrorHandler: true }
    ),
  applyUpstreamCatalogRefresh: (channelId: number, basisToken: string) =>
    request<UpstreamRefreshApplyResult>(
      `/api/imported-upstreams/${channelId}/models/refresh-apply`,
      {
        method: 'POST',
        body: JSON.stringify({ basisToken }),
        skipErrorHandler: true,
      }
    ),
  getUpstreamModels: (channelId: number) =>
    request<{ items: UpstreamModel[] }>(
      `/api/imported-upstreams/${channelId}/models`
    ),
  createUpstreamModels: (
    channelId: number,
    input: { names: string[]; enabled?: boolean }
  ) =>
    request<{ items: UpstreamModel[] }>(
      `/api/imported-upstreams/${channelId}/models`,
      {
        method: 'POST',
        body: JSON.stringify(input),
      }
    ),
  updateUpstreamModel: (
    id: number,
    input: { name?: string; enabled?: boolean }
  ) =>
    request<{ success: boolean; id: number; affectedRouteIds: number[] }>(
      `/api/imported-upstreams/models/${id}`,
      {
        method: 'PATCH',
        body: JSON.stringify(input),
      }
    ),
  createUpstreamGrant: (input: {
    modelId: number
    credentialId: number
    protocols: number[]
    enabled?: boolean
  }) =>
    request<UpstreamGrant>('/api/imported-upstreams/grants', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  updateUpstreamGrant: (
    id: number,
    input: { protocols?: number[]; enabled?: boolean }
  ) =>
    request<UpstreamGrant>(`/api/imported-upstreams/grants/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(input),
      skipErrorHandler: true,
    }),
  getUpstreamGroups: () =>
    request<{ items: UpstreamGroup[] }>('/api/imported-upstreams/groups'),
  createUpstreamGroup: (input: UpstreamGroupCreate) =>
    request<UpstreamGroup>('/api/imported-upstreams/groups', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  updateUpstreamGroup: (
    id: number,
    input: {
      name?: string
      mode?: 'failover' | 'manual'
      enabled?: boolean
      activeMemberId?: number
    }
  ) =>
    request<UpstreamGroup>(`/api/imported-upstreams/groups/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(input),
    }),
  createUpstreamMember: (id: number, input: UpstreamMemberCreate) =>
    request<UpstreamGroupMember>(
      `/api/imported-upstreams/groups/${id}/members`,
      { method: 'POST', body: JSON.stringify(input) }
    ),
}
