import type {
  ImportedCredential,
  ImportedCredentialUpdate,
  ImportedUpstreamUpdate,
  ImportedEndpointConfig,
} from './imported-upstreams'
import { request } from './transport'

export type UpstreamOwnership = 'native' | 'imported'
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
export type UpstreamChannelCreate = ImportedUpstreamUpdate & {
  name: string
  provider: string
  baseUrl: string
  endpointConfig: ImportedEndpointConfig
}

export const upstreamCatalogApi = {
  createUpstreamChannel: (input: UpstreamChannelCreate) =>
    request<{
      id: number
      name: string
      enabled: boolean
      ownership: UpstreamOwnership
    }>('/api/imported-upstreams', {
      method: 'POST',
      body: JSON.stringify(input),
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
