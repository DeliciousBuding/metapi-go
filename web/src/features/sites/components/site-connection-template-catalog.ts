import type { SiteInitializationPreset } from '@/lib/api/sites'
import {
  getConnectionPresetIcon,
  type ConnectionTemplate,
} from '@/lib/platform-catalog'

export const TEMPLATE_CATEGORIES = [
  'common',
  'gateway',
  'api',
  'coding',
  'oauth',
] as const
export type TemplateCategory = (typeof TEMPLATE_CATEGORIES)[number]

const COMMON_TEMPLATE_IDS = [
  'new-api-connection',
  'bailian',
  'codingplan-openai',
  'siliconflow',
  'deepseek-openai',
  'moonshot-openai',
  'minimax-openai',
  'zhipu-coding-plan-openai',
  'zai-coding-plan-openai',
  'kimi-coding-openai',
  'doubao-coding-openai',
  'xiaomi-token-plan-claude',
]
const DOMESTIC_PROVIDERS = [
  'bailian',
  'siliconflow',
  'deepseek',
  'moonshot',
  'minimax',
  'modelscope',
  'zhipu',
  'zai',
  'doubao',
  'xiaomi',
  'stepfun',
  'mimo',
  'xiaomimimo',
  'ppio',
  'qiniu',
]

function isCodingPlan(template: ConnectionTemplate) {
  return (
    template.group === 'api' &&
    (/coding|token-plan/.test(template.id) ||
      template.id.startsWith('minimax-'))
  )
}

function providerFamily(template: ConnectionTemplate) {
  const family = template.id.split('-')[0]
  if (family === 'codingplan') return 'bailian'
  if (family === 'kimi') return 'moonshot'
  if (family === 'volcengine') return 'doubao'
  return family
}

function protocolOrder(template: ConnectionTemplate) {
  return ['openai', 'claude', 'gemini'].indexOf(template.platform)
}

function compareWithinBrand(a: ConnectionTemplate, b: ConnectionTemplate) {
  return (
    providerFamily(a).localeCompare(providerFamily(b), 'en') ||
    protocolOrder(a) - protocolOrder(b) ||
    a.id.localeCompare(b.id, 'en')
  )
}

const CODING_PROVIDERS = [
  'bailian',
  'zhipu',
  'moonshot',
  'doubao',
  'minimax',
  'xiaomi',
  'zai',
]

function codingOrder(template: ConnectionTemplate) {
  const index = CODING_PROVIDERS.indexOf(providerFamily(template))
  return index === -1 ? CODING_PROVIDERS.length : index
}

function serviceOrder(template: ConnectionTemplate) {
  const domestic = DOMESTIC_PROVIDERS.indexOf(providerFamily(template))
  if (domestic !== -1) return domestic
  const official = ['openai-api', 'anthropic-api', 'gemini-api'].indexOf(
    template.id
  )
  if (official !== -1) return 100 + official
  return 50
}

export function templatesForCategory(
  templates: ConnectionTemplate[],
  category: TemplateCategory,
  query: string
) {
  if (query) {
    return templates
      .filter((template) =>
        `${template.name} ${template.providerLabel ?? ''} ${template.label ?? ''} ${template.platform} ${template.url}`
          .toLowerCase()
          .includes(query)
      )
      .sort(
        (a, b) => serviceOrder(a) - serviceOrder(b) || compareWithinBrand(a, b)
      )
  }
  if (category === 'common') {
    return COMMON_TEMPLATE_IDS.flatMap((id) =>
      templates.filter((template) => template.id === id)
    )
  }
  if (category === 'coding') {
    return templates
      .filter(isCodingPlan)
      .sort(
        (a, b) => codingOrder(a) - codingOrder(b) || compareWithinBrand(a, b)
      )
  }
  if (category === 'api') {
    return templates
      .filter(
        (template) =>
          template.group === 'api' &&
          (!isCodingPlan(template) || template.id.startsWith('minimax-'))
      )
      .sort(
        (a, b) => serviceOrder(a) - serviceOrder(b) || compareWithinBrand(a, b)
      )
  }
  return templates.filter((template) => template.group === category)
}

export function templateFromPreset(
  preset: SiteInitializationPreset,
  name: string
): ConnectionTemplate {
  let protocols: ConnectionTemplate['protocols'] = ['chat']
  if (preset.id === 'openai-api') protocols = ['chat', 'responses']
  else if (preset.platform === 'claude') protocols = ['messages']
  else if (preset.platform === 'gemini') protocols = ['gemini']
  return {
    id: preset.id,
    name,
    providerLabel: preset.providerLabel,
    label: preset.label,
    platform: preset.platform,
    icon: getConnectionPresetIcon(preset.id),
    url: preset.defaultUrl,
    group: 'api',
    protocols,
    available: true,
  }
}
