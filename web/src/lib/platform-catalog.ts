// Display metadata only. IDs stay aligned with platform/registry.go; API-compatible
// service presets reuse an existing adapter and never create a new platform ID.
export type PlatformDefinition = {
  id: string
  name: string
  icon?: string
  group: 'api' | 'gateway' | 'oauth'
  selectable?: boolean
}

export const PLATFORM_CATALOG: readonly PlatformDefinition[] = [
  {
    id: 'openai',
    name: 'OpenAI',
    icon: 'openai',
    group: 'api',
  },
  {
    id: 'claude',
    name: 'Anthropic Claude',
    icon: 'claude-color',
    group: 'api',
  },
  {
    id: 'gemini',
    name: 'Google Gemini',
    icon: 'gemini-color',
    group: 'api',
  },
  {
    id: 'sensetime',
    name: 'SenseTime',
    icon: 'sensenova-brand-color',
    group: 'api',
    selectable: false,
  },
  {
    id: 'codex',
    name: 'OpenAI Codex',
    icon: 'openai',
    group: 'oauth',
  },
  {
    id: 'gemini-cli',
    name: 'Gemini CLI',
    icon: 'gemini-color',
    group: 'oauth',
  },
  {
    id: 'antigravity',
    name: 'Antigravity',
    group: 'oauth',
  },
  {
    id: 'grok',
    name: 'xAI Grok',
    icon: 'xai',
    group: 'oauth',
  },
  {
    id: 'new-api',
    name: 'New API',
    icon: 'new-api',
    group: 'gateway',
  },
  {
    id: 'one-api',
    name: 'One API',
    group: 'gateway',
  },
  {
    id: 'one-hub',
    name: 'One Hub',
    group: 'gateway',
  },
  {
    id: 'done-hub',
    name: 'Done Hub',
    group: 'gateway',
  },
  {
    id: 'veloera',
    name: 'Veloera',
    group: 'gateway',
  },
  {
    id: 'anyrouter',
    name: 'AnyRouter',
    group: 'gateway',
  },
  {
    id: 'sub2api',
    name: 'Sub2API',
    group: 'gateway',
  },
  {
    id: 'cliproxyapi',
    name: 'CLIProxyAPI',
    group: 'gateway',
  },
]

export function getPlatformDefinition(platform: string | null | undefined) {
  const id = platform?.trim().toLowerCase()
  return PLATFORM_CATALOG.find((entry) => entry.id === id)
}

export function getPlatformDisplayName(
  platform: string | null | undefined
): string {
  return getPlatformDefinition(platform)?.name ?? platform?.trim() ?? ''
}

export type ConnectionTemplate = {
  id: string
  name: string
  label?: string
  providerLabel?: string
  platform: string
  icon?: string
  url: string
  protocols?: readonly ('chat' | 'responses' | 'messages' | 'gemini')[]
  group: PlatformDefinition['group']
  available: boolean
}

const PRESET_BRAND_ICONS: Record<string, string> = {
  openai: 'openai',
  anthropic: 'claude-color',
  claude: 'claude-color',
  gemini: 'gemini-color',
  openrouter: 'openrouter',
  groq: 'groq',
  mistral: 'mistral-color',
  xai: 'xai',
  siliconflow: 'siliconcloud-color',
  bailian: 'bailian-color',
  fireworks: 'fireworks-color',
  cerebras: 'cerebras-brand-color',
  codingplan: 'bailian-color',
  zhipu: 'zhipu-color',
  deepseek: 'deepseek-color',
  moonshot: 'moonshot',
  minimax: 'minimax-color',
  modelscope: 'modelscope-color',
  doubao: 'doubao-color',
  kimi: 'moonshot',
  xiaomi: 'xiaomimimo',
  zai: 'zai',
  ppio: 'ppio-color',
  qiniu: 'qiniu-color',
  jina: 'jina',
  ollama: 'ollama',
  bedrock: 'bedrock',
  seedance: 'doubao-color',
  zenmux: 'zenmux',
  cline: 'cline',
  opencode: 'opencode',
  longcat: 'longcat-color',
  nanogpt: 'nanogpt',
}

export function getConnectionPresetIcon(id: string): string | undefined {
  return PRESET_BRAND_ICONS[id.split('-')[0]]
}

const PRESET_NAME_KEYS: Record<string, { normal: string; plan?: string }> = {
  bailian: { normal: 'sites.templates.brands.bailian' },
  codingplan: { normal: 'sites.templates.brands.bailianPlan' },
  zhipu: {
    normal: 'sites.templates.brands.glm',
    plan: 'sites.templates.brands.glmPlan',
  },
  zai: {
    normal: 'sites.templates.brands.zai',
    plan: 'sites.templates.brands.zaiPlan',
  },
  doubao: {
    normal: 'sites.templates.brands.ark',
    plan: 'sites.templates.brands.arkPlan',
  },
  volcengine: {
    normal: 'sites.templates.brands.ark',
    plan: 'sites.templates.brands.arkPlan',
  },
  qiniu: { normal: 'sites.templates.brands.qiniu' },
  xiaomi: {
    normal: 'sites.templates.brands.mimo',
    plan: 'sites.templates.brands.mimoPlan',
  },
  moonshot: {
    normal: 'sites.templates.brands.kimi',
    plan: 'sites.templates.brands.kimiPlan',
  },
  kimi: {
    normal: 'sites.templates.brands.kimi',
    plan: 'sites.templates.brands.kimiPlan',
  },
}

export function getConnectionPresetNameKey(id: string): string | undefined {
  const keys = PRESET_NAME_KEYS[id.split('-')[0]]
  return /coding|token-plan/.test(id)
    ? (keys?.plan ?? keys?.normal)
    : keys?.normal
}

// API service defaults come from /api/sites/initialization-presets. Only local
// software/OAuth display metadata remains here; these templates supply no URL.
export const PLATFORM_CONNECTION_TEMPLATES: readonly ConnectionTemplate[] =
  PLATFORM_CATALOG.filter((platform) => platform.group !== 'api').map(
    (platform) => ({
      id: `${platform.id}-connection`,
      name: platform.name,
      platform: platform.id,
      icon: platform.icon,
      url: '',
      group: platform.group,
      available: platform.id !== 'antigravity',
    })
  )
