export type OAuthProviderInfo = {
  provider: string
  label: string
  platform: string
  enabled: boolean
  loginType: 'oauth'
  requiresProjectId: boolean
  supportsDirectAccountRouting: boolean
  supportsCloudValidation: boolean
  supportsNativeProxy: boolean
  supportsDeviceFlow: boolean
}

export type OAuthProvidersResponse = {
  providers: OAuthProviderInfo[]
  defaults?: {
    systemProxyConfigured?: boolean
  }
}

export type OAuthRouteUnitStrategy = 'round_robin' | 'stick_until_unavailable'

type OAuthRouteUnitSummary = {
  id?: number
  routeUnitId?: number
  name: string
  strategy: OAuthRouteUnitStrategy
  memberCount: number
}

type OAuthRouteParticipation =
  | { kind: 'single' }
  | ({ kind: 'route_unit' } & OAuthRouteUnitSummary)

type OAuthStartInstructions = {
  redirectUri: string
  callbackPort: number
  callbackPath: string
  manualCallbackDelayMs: number
  sshTunnelCommand?: string
  sshTunnelKeyCommand?: string
  deviceFlow?: {
    userCode: string
    verificationUri: string
    pollIntervalSeconds: number
    expiresInSeconds: number
  }
}

export type OAuthStartResponse = {
  provider: string
  state: string
  authorizationUrl: string
  instructions: OAuthStartInstructions
}

export type OAuthSessionInfo = {
  provider: string
  state: string
  status: 'pending' | 'success' | 'error'
  accountId?: number
  siteId?: number
  error?: string
  retryAfterSeconds?: number
}

type OAuthQuotaWindowInfo = {
  supported: boolean
  limit?: number | null
  used?: number | null
  remaining?: number | null
  resetAt?: string | null
  message?: string | null
}

export type OAuthQuotaInfo = {
  status: 'supported' | 'unsupported' | 'error'
  source: 'official' | 'reverse_engineered'
  lastSyncAt?: string | null
  lastError?: string | null
  providerMessage?: string | null
  subscription?: {
    planType?: string | null
    activeStart?: string | null
    activeUntil?: string | null
  } | null
  windows: {
    fiveHour: OAuthQuotaWindowInfo
    sevenDay: OAuthQuotaWindowInfo
  }
  lastLimitResetAt?: string | null
}

export type OAuthConnectionInfo = {
  accountId: number
  siteId: number
  provider: string
  username?: string | null
  email?: string | null
  accountKey?: string | null
  planType?: string | null
  projectId?: string | null
  modelCount: number
  modelsPreview: string[]
  status: 'healthy' | 'abnormal'
  quota?: OAuthQuotaInfo | null
  routeChannelCount?: number
  lastModelSyncAt?: string | null
  lastModelSyncError?: string | null
  proxyUrl?: string | null
  useSystemProxy?: boolean
  routeUnit?: OAuthRouteUnitSummary | null
  routeParticipation?: OAuthRouteParticipation | null
  site?: { id: number; name: string; url: string; platform: string } | null
}

export type OAuthConnectionsResponse = {
  items: OAuthConnectionInfo[]
  total: number
  limit: number
  offset: number
}
