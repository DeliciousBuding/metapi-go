import { request } from './transport'

export type ImportedEndpoint = {
  url: string
  auth: 'bearer' | 'x-api-key' | 'x-goog-api-key' | 'none'
  modelPath?: boolean
  profile?:
    | 'codex'
    | 'claudecode'
    | 'deepseek'
    | 'zai'
    | 'bailian'
    | 'cline'
    | 'moonshot'
    | 'longcat'
    | 'openrouter'
    | 'cerebras'
    | 'nanogpt'
    | 'openrouter-image'
    | 'opencode-go'
    | 'jina-embeddings'
    | 'minimax-image'
    | 'modelscope-image'
    | 'codex-image'
    | 'ollama'
    | 'ollama-messages'
    | 'bedrock'
    | 'seedance-video'
    | 'zenmux-video'
    | 'codex-alpha-search'
  requestModel?: string
  modelWireUrls?: { responses: string; messages: string }
}

export type ImportedEndpointConfig = Partial<
  Record<
    | 'chat'
    | 'responses'
    | 'messages'
    | 'gemini'
    | 'completions'
    | 'embeddings'
    | 'rerank'
    | 'imageGeneration'
    | 'imageEdit'
    | 'imageVariation'
    | 'audioSpeech'
    | 'audioTranscription'
    | 'audioTranslation'
    | 'moderations'
    | 'video'
    | 'geminiEmbeddings'
    | 'jinaEmbeddings'
    | 'modelscopeImageGeneration'
    | 'seedanceVideo'
    | 'zenmuxVideo'
    | 'ollama'
    | 'systemOne'
    | 'alphaSearch',
    ImportedEndpoint
  >
>

export type ImportedRequestConfig = {
  channelProxy: string
  customHeaders: string
  paramOverride: string
}

export type ImportedUpstreamDetail = {
  ownership?: 'native' | 'imported'
  id: number
  name: string
  originKey: string
  provider: string
  dialect: string
  enabled: boolean
  baseUrl: string
  endpointConfig?: ImportedEndpointConfig
  openaiChatCompletionPath: string
  openaiResponsePath: string
  anthropicMessagePath: string
  useSystemProxy: boolean
  hasChannelProxy: boolean
  hasCustomHeaders: boolean
  hasParamOverride: boolean
  channelProxyDisplay: string
}

export type ImportedUpstreamUpdate = Partial<
  Pick<
    ImportedUpstreamDetail,
    | 'name'
    | 'enabled'
    | 'baseUrl'
    | 'endpointConfig'
    | 'openaiChatCompletionPath'
    | 'openaiResponsePath'
    | 'anthropicMessagePath'
    | 'useSystemProxy'
  > &
    ImportedRequestConfig
>

export type ImportedCredential = {
  ownership?: 'native' | 'imported'
  id: number
  name: string
  enabled: boolean
  kind: 'api_key' | 'oauth' | 'none'
  expiresAt?: number // Unix milliseconds
  canRefresh: boolean
}

export type ImportedCredentialUpdate = {
  name?: string
  enabled?: boolean
  apiKey?: string
  kind?: 'api_key' | 'oauth' | 'none'
  oauth?: {
    accessToken: string
    refreshToken?: string
    clientId?: string
    expiresAt?: number // Unix milliseconds
    idToken?: string
    accountId?: string
  }
}

export type ImportedUpstreamInventory = {
  items: Array<{
    ownership?: 'native' | 'imported'
    id: number
    name: string
    originKey: string
    dialect: string
    provider?: string
    baseUrl: string
    endpointConfig?: ImportedEndpointConfig
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
    credentialKind?: 'api_key' | 'oauth' | 'none'
    credentialEnabled: boolean
    grantId?: number
    channelEnabled?: boolean
    modelEnabled?: boolean
    groupEnabled?: boolean
    routeEnabled?: boolean
    grantEnabled?: boolean
    selectedByGroup?: boolean
    effectiveEnabled?: boolean
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

export type ImportedUpstream = ImportedUpstreamInventory['items'][number]
export type ImportedMember = ImportedUpstreamInventory['members'][number]
export type ImportedMemberUpdate = {
  priority?: number
  weight?: number
  protocolOrder?: number[]
}

export const importedUpstreamsApi = {
  getImportedUpstream: (id: number) =>
    request<ImportedUpstreamDetail>(`/api/imported-upstreams/${id}`),
  getImportedRequestConfig: (id: number, signal?: AbortSignal) =>
    request<ImportedRequestConfig>(
      `/api/imported-upstreams/${id}/request-config`,
      { signal, disableDuplicate: true }
    ),
  updateImportedUpstream: (id: number, input: ImportedUpstreamUpdate) =>
    request<{ success: boolean; id: number; enabled: boolean }>(
      `/api/imported-upstreams/${id}`,
      { method: 'PATCH', body: JSON.stringify(input) }
    ),
  updateImportedMember: (id: number, input: ImportedMemberUpdate) =>
    request<{ success: boolean; id: number }>(
      `/api/imported-upstreams/members/${id}`,
      { method: 'PATCH', body: JSON.stringify(input) }
    ),
  clearImportedMemberCooldown: (id: number) =>
    request<{ success: boolean; id: number; grantId: number }>(
      `/api/imported-upstreams/members/${id}/cooldown/clear`,
      { method: 'POST' }
    ),
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
