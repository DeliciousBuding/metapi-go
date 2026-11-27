type KeyPolicyPresetId = 'claude-code' | 'codex-cli' | 'gemini-cli' | 'sandbox'

export type KeyPolicyPreset = {
  id: KeyPolicyPresetId
  labelKey: string
  supportedModels: string[]
  maxRequests?: number
  maxCost?: number
}

export const KEY_POLICY_PRESETS: readonly KeyPolicyPreset[] = [
  {
    id: 'claude-code',
    labelKey: 'settings.downstream.keys.presets.claudeCode',
    supportedModels: ['claude-*'],
  },
  {
    id: 'codex-cli',
    labelKey: 'settings.downstream.keys.presets.codexCli',
    supportedModels: ['gpt-*', 'o3*', 'o4*', 'codex-*'],
  },
  {
    id: 'gemini-cli',
    labelKey: 'settings.downstream.keys.presets.geminiCli',
    supportedModels: ['gemini-*'],
  },
  {
    id: 'sandbox',
    labelKey: 'settings.downstream.keys.presets.sandbox',
    supportedModels: ['gpt-4o-mini', 'claude-3-5-haiku-*', 'gemini-2.5-flash'],
    maxRequests: 100,
    maxCost: 1,
  },
] as const

export type PolicyWarningCode =
  | 'emptyModels'
  | 'emptyDirectChannels'
  | 'credentialConflict'
  | 'modelIntersectionEmpty'
  | 'zeroQuota'

function globToRegex(pattern: string): RegExp | null {
  const escaped = pattern
    .replace(/[.+?^${}()|[\]\\]/g, '\\$&')
    .replace(/\*/g, '.*')
  try {
    return new RegExp(`^${escaped}$`, 'i')
  } catch {
    return null
  }
}

function matchesModelRule(modelName: string, rule: string): boolean {
  const trimmedRule = rule.trim()
  const trimmedModel = modelName.trim()
  if (!trimmedRule || !trimmedModel) return false
  if (trimmedRule === '*') return true
  if (trimmedRule.startsWith('re:')) {
    try {
      return new RegExp(trimmedRule.slice(3)).test(trimmedModel)
    } catch {
      return false
    }
  }
  if (trimmedRule.includes('*')) {
    const regex = globToRegex(trimmedRule)
    return regex ? regex.test(trimmedModel) : false
  }
  return trimmedModel.toLowerCase() === trimmedRule.toLowerCase()
}

type RefLike = { kind?: string; id?: number }
type AccessPolicyLike = {
  allowedUpstreamChannelIds?: readonly (number | undefined)[]
  modelIds?: readonly (string | undefined)[]
  quota?: {
    requests?: number
    totalTokens?: number
    cost?: number
  } | null
} | null

export function detectPolicyIntersectionWarnings(input: {
  supportedModels?: readonly (string | undefined)[]
  routeGrantCount?: number
  allowedCredentialRefs?: readonly (RefLike | undefined)[]
  excludedCredentialRefs?: readonly (RefLike | undefined)[]
  accessPolicy?: AccessPolicyLike
  maxRequests?: number
  maxCost?: number
}): PolicyWarningCode[] {
  const warnings: PolicyWarningCode[] = []
  const models = (input.supportedModels ?? []).filter(
    (m): m is string => typeof m === 'string' && m.trim().length > 0
  )
  const routeGrants = input.routeGrantCount ?? 0

  if (models.length === 0 && routeGrants === 0) {
    warnings.push('emptyModels')
  }

  const policy = input.accessPolicy
  if (
    policy?.allowedUpstreamChannelIds !== undefined &&
    policy.allowedUpstreamChannelIds.length === 0
  ) {
    warnings.push('emptyDirectChannels')
  }

  const allowedRefs = (input.allowedCredentialRefs ?? []).filter(
    (ref): ref is { kind: string; id: number } =>
      typeof ref?.kind === 'string' && typeof ref?.id === 'number'
  )
  const excludedRefs = (input.excludedCredentialRefs ?? []).filter(
    (ref): ref is { kind: string; id: number } =>
      typeof ref?.kind === 'string' && typeof ref?.id === 'number'
  )
  if (allowedRefs.length > 0 && excludedRefs.length > 0) {
    const excludedSet = new Set(
      excludedRefs.map((ref) => `${ref.kind}:${ref.id}`)
    )
    if (allowedRefs.every((ref) => excludedSet.has(`${ref.kind}:${ref.id}`))) {
      warnings.push('credentialConflict')
    }
  }

  const policyModels = (policy?.modelIds ?? []).filter(
    (m): m is string => typeof m === 'string' && m.trim().length > 0
  )
  if (
    models.length > 0 &&
    policyModels.length > 0 &&
    !policyModels.some((modelId) =>
      models.some((rule) => matchesModelRule(modelId, rule))
    )
  ) {
    warnings.push('modelIntersectionEmpty')
  }

  if (
    input.maxRequests === 0 ||
    input.maxCost === 0 ||
    policy?.quota?.requests === 0 ||
    policy?.quota?.totalTokens === 0 ||
    policy?.quota?.cost === 0
  ) {
    warnings.push('zeroQuota')
  }

  return warnings
}
