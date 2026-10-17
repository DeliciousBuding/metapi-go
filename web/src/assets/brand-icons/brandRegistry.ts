import { BRAND_ICONS } from './icons.js'
// Official New API mark: QuantumNous/new-api, web/public/logo.png (MIT).
import newApiLogo from './new-api.png'

export { getBrand, type BrandInfo } from '../../../shared/modelBrand.js'

const LEGACY_ICON_ALIASES: Record<string, string> = {
  anthropic: 'claude-color',
  'claude.color': 'claude-color',
  'cohere.color': 'cohere-color',
  'doubao.color': 'doubao-color',
  'gemini.color': 'gemini-color',
  'hunyuan.color': 'hunyuan-color',
  meta: 'meta-color',
  'meta-brand-color': 'meta-color',
  'minimax.color': 'minimax-color',
  'qwen.color': 'qwen-color',
  'spark.color': 'spark-color',
  stability: 'stability-color',
  'stability-brand-color': 'stability-color',
  stepfun: 'stepfun-color',
  'wenxin.color': 'wenxin-color',
  xai: 'xai',
  'yi.color': 'yi-color',
  'zhipu.color': 'zhipu-color',
  azure: 'microsoft-color',
  'bytedance-brand-color': 'bytedance-color',
}

function normalizeInput(value: string): string {
  return String(value || '')
    .trim()
    .toLowerCase()
}

export function normalizeBrandIconKey(
  icon: string | null | undefined
): string | null {
  const normalized = normalizeInput(icon || '').replaceAll('.', '-')
  if (!normalized) return null
  return LEGACY_ICON_ALIASES[normalized] || normalized
}

export function getBrandIconUrl(
  icon: string | null | undefined,
  variant: 'dark' | 'light'
): string | null {
  const normalized = normalizeBrandIconKey(icon)
  if (!normalized) return null
  if (normalized === 'new-api') return newApiLogo
  return BRAND_ICONS[normalized]?.[variant] ?? null
}

export function avatarLetters(name: string): string {
  const parts = name
    .replaceAll(/[-_/.]/g, ' ')
    .trim()
    .split(/\s+/)
    .filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return name.slice(0, 2).toUpperCase()
}
