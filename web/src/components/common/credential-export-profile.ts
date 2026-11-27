import type {
  CredentialExportProfile as ExportProfile,
  CredentialExportProfileId as ExportProfileId,
  CredentialExportResponse as ExportResponse,
} from '@/lib/api/credential-export'

import type { CredentialVerifyProtocol } from './credential-verify-section'

const PROFILE_PROTOCOL: Record<ExportProfileId, CredentialVerifyProtocol> = {
  openai: 'chat',
  cherry: 'chat',
  generic: 'chat',
  'claude-code': 'messages',
  codex: 'responses',
  openwebui: 'chat',
}
const CC_SWITCH_APP: Partial<Record<ExportProfileId, string>> = {
  codex: 'codex',
  'claude-code': 'claude',
}
export const CLIENT_CONFIG_META: Record<
  ExportProfileId,
  { requiresV1: boolean; pathKey: string }
> = {
  codex: { requiresV1: true, pathKey: 'connect.clientPaths.codex' },
  'claude-code': {
    requiresV1: false,
    pathKey: 'connect.clientPaths.claude-code',
  },
  openai: { requiresV1: true, pathKey: 'connect.clientPaths.openai' },
  cherry: { requiresV1: true, pathKey: 'connect.clientPaths.cherry' },
  openwebui: { requiresV1: true, pathKey: 'connect.clientPaths.openwebui' },
  generic: { requiresV1: true, pathKey: 'connect.clientPaths.generic' },
}

function buildCherryStudioLink(
  keyId: number,
  keyName: string,
  apiBase: string,
  apiKey: string
): string {
  const payload = JSON.stringify({
    id: String(keyId),
    name: keyName,
    baseUrl: apiBase,
    apiKey,
  })
  const data = encodeURIComponent(btoa(unescape(encodeURIComponent(payload))))
  return `cherrystudio://providers/api-keys?v=1&data=${data}`
}

function buildCcSwitchLink(
  app: string,
  apiBase: string,
  apiKey: string
): string {
  const params = new URLSearchParams({
    resource: 'provider',
    app,
    endpoint: apiBase,
    apiKey,
  })
  return `ccswitch://v1/import?${params.toString()}`
}

/** Render profile content as clipboard text. */
export function stringifyProfileContent(profile: ExportProfile): string {
  if (typeof profile.content === 'string') {
    return profile.content
  }
  return JSON.stringify(profile.content, null, 2)
}

export type CredentialExportTarget = {
  id: number
  name: string
  keyMasked?: string
}

export function getClientContext(
  data: ExportResponse,
  client: ExportProfileId
) {
  const apiBase = `${data.baseUrl.replace(/\/+$/, '')}/v1`
  const genericProfile = data.profiles.find((p) => p.id === 'generic')
  const cherryProfile = data.profiles.find((p) => p.id === 'cherry')
  const selectedProfile = data.profiles.find((p) => p.id === client)
  const protocol = selectedProfile?.protocol ?? PROFILE_PROTOCOL[client]
  const clientBase = client === 'claude-code' ? data.baseUrl : apiBase
  const ccApp = CC_SWITCH_APP[client]
  const keyContent = cherryProfile?.content ?? genericProfile?.content
  const apiKey =
    typeof keyContent === 'object' && typeof keyContent.apiKey === 'string'
      ? keyContent.apiKey
      : ''
  return {
    client,
    selectedProfile,
    protocol,
    apiBase,
    clientBase,
    ccApp,
    apiKey,
  }
}

export type ClientContext = ReturnType<typeof getClientContext>
export type ClientImport = { url: string; fallback: string }

export function buildClientImport(
  target: CredentialExportTarget,
  context: ClientContext
): ClientImport | null {
  const { client, ccApp, apiBase, clientBase, apiKey, selectedProfile } =
    context
  if (client === 'cherry') {
    return {
      url: buildCherryStudioLink(target.id, target.name, apiBase, apiKey),
      fallback: JSON.stringify(
        { name: target.name, type: 'openai', apiHost: apiBase, apiKey },
        null,
        2
      ),
    }
  }
  if (!ccApp) return null
  return {
    url: buildCcSwitchLink(ccApp, clientBase, apiKey),
    fallback: selectedProfile
      ? stringifyProfileContent(selectedProfile)
      : `${apiBase} ${apiKey}`,
  }
}
