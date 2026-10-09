// Display metadata only. IDs stay aligned with platform/registry.go; API-compatible
// service presets reuse an existing adapter and never create a new platform ID.
export type PlatformDefinition = {
  id: string
  name: string
  icon?: string
  group: 'api' | 'gateway' | 'oauth'
  selectable?: boolean
  descriptionKey: string
}

export const PLATFORM_CATALOG: readonly PlatformDefinition[] = [
  {
    id: 'openai',
    name: 'OpenAI',
    icon: 'openai',
    group: 'api',
    descriptionKey: 'platforms.openai',
  },
  {
    id: 'claude',
    name: 'Anthropic Claude',
    icon: 'claude-color',
    group: 'api',
    descriptionKey: 'platforms.claude',
  },
  {
    id: 'gemini',
    name: 'Google Gemini',
    icon: 'gemini-color',
    group: 'api',
    descriptionKey: 'platforms.gemini',
  },
  {
    id: 'sensetime',
    name: 'SenseTime',
    icon: 'sensenova-brand-color',
    group: 'api',
    selectable: false,
    descriptionKey: 'platforms.sensetime',
  },
  {
    id: 'codex',
    name: 'OpenAI Codex',
    icon: 'openai',
    group: 'oauth',
    descriptionKey: 'platforms.codex',
  },
  {
    id: 'gemini-cli',
    name: 'Gemini CLI',
    icon: 'gemini-color',
    group: 'oauth',
    descriptionKey: 'platforms.geminiCli',
  },
  {
    id: 'antigravity',
    name: 'Antigravity',
    group: 'oauth',
    descriptionKey: 'platforms.antigravity',
  },
  {
    id: 'grok',
    name: 'xAI Grok',
    icon: 'xai',
    group: 'oauth',
    descriptionKey: 'platforms.grok',
  },
  {
    id: 'new-api',
    name: 'New API',
    icon: 'new-api',
    group: 'gateway',
    descriptionKey: 'platforms.newApi',
  },
  {
    id: 'one-api',
    name: 'One API',
    group: 'gateway',
    descriptionKey: 'platforms.oneApi',
  },
  {
    id: 'one-hub',
    name: 'One Hub',
    group: 'gateway',
    descriptionKey: 'platforms.oneHub',
  },
  {
    id: 'done-hub',
    name: 'Done Hub',
    group: 'gateway',
    descriptionKey: 'platforms.doneHub',
  },
  {
    id: 'veloera',
    name: 'Veloera',
    group: 'gateway',
    descriptionKey: 'platforms.veloera',
  },
  {
    id: 'anyrouter',
    name: 'AnyRouter',
    group: 'gateway',
    descriptionKey: 'platforms.anyrouter',
  },
  {
    id: 'sub2api',
    name: 'Sub2API',
    group: 'gateway',
    descriptionKey: 'platforms.sub2api',
  },
  {
    id: 'cliproxyapi',
    name: 'CLIProxyAPI',
    group: 'gateway',
    descriptionKey: 'platforms.cliproxyapi',
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

export type ConnectionPreset = {
  id: string
  name: string
  platform: string
  icon?: string
  url: string
  protocols?: readonly ('chat' | 'responses' | 'messages' | 'gemini')[]
}

export const CONNECTION_PRESETS: readonly ConnectionPreset[] = [
  {
    id: 'openai-api',
    name: 'OpenAI API',
    platform: 'openai',
    icon: 'openai',
    url: 'https://api.openai.com',
    protocols: ['chat', 'responses'],
  },
  {
    id: 'anthropic-api',
    name: 'Anthropic API',
    platform: 'claude',
    icon: 'claude-color',
    url: 'https://api.anthropic.com',
    protocols: ['messages'],
  },
  {
    id: 'gemini-api',
    name: 'Gemini API',
    platform: 'gemini',
    icon: 'gemini-color',
    url: 'https://generativelanguage.googleapis.com',
    protocols: ['gemini'],
  },
  {
    id: 'deepseek',
    name: 'DeepSeek',
    platform: 'openai',
    icon: 'deepseek-color',
    url: 'https://api.deepseek.com',
  },
  {
    id: 'openrouter',
    name: 'OpenRouter',
    platform: 'openai',
    icon: 'openrouter',
    url: 'https://openrouter.ai/api',
  },
  {
    id: 'groq',
    name: 'Groq',
    platform: 'openai',
    icon: 'groq',
    url: 'https://api.groq.com/openai',
  },
  {
    id: 'mistral',
    name: 'Mistral AI',
    platform: 'openai',
    icon: 'mistral-color',
    url: 'https://api.mistral.ai',
  },
  {
    id: 'xai-api',
    name: 'xAI API',
    platform: 'openai',
    icon: 'xai',
    url: 'https://api.x.ai',
  },
  {
    id: 'moonshot',
    name: 'Moonshot / Kimi',
    platform: 'openai',
    icon: 'moonshot',
    url: 'https://api.moonshot.cn',
  },
  {
    id: 'siliconflow',
    name: 'SiliconFlow',
    platform: 'openai',
    icon: 'siliconcloud-color',
    url: 'https://api.siliconflow.cn',
  },
  {
    id: 'bailian',
    name: 'Alibaba Bailian',
    platform: 'openai',
    icon: 'bailian-color',
    url: 'https://dashscope.aliyuncs.com/compatible-mode',
  },
  {
    id: 'fireworks',
    name: 'Fireworks AI',
    platform: 'openai',
    icon: 'fireworks-color',
    url: 'https://api.fireworks.ai/inference',
  },
  {
    id: 'cerebras',
    name: 'Cerebras',
    platform: 'openai',
    icon: 'cerebras-brand-color',
    url: 'https://api.cerebras.ai',
  },
  {
    id: 'modelscope',
    name: 'ModelScope',
    platform: 'openai',
    icon: 'modelscope-color',
    url: 'https://api-inference.modelscope.cn',
  },
]

export type ConnectionTemplate = ConnectionPreset & {
  group: PlatformDefinition['group']
  available: boolean
}

// The creation view derives software/OAuth templates from the same platform
// catalog. Service URLs remain owned by CONNECTION_PRESETS above.
export const CONNECTION_TEMPLATES: readonly ConnectionTemplate[] = [
  ...CONNECTION_PRESETS.map((preset) => ({
    ...preset,
    protocols: preset.protocols ?? (['chat'] as const),
    group: 'api' as const,
    available: true,
  })),
  ...PLATFORM_CATALOG.filter((platform) => platform.group !== 'api').map(
    (platform) => ({
      id: `${platform.id}-connection`,
      name: platform.name,
      platform: platform.id,
      icon: platform.icon,
      url: '',
      group: platform.group,
      available: platform.id !== 'antigravity',
    })
  ),
]
