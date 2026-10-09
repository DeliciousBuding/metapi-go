import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import { Badge } from '@/components/ui/badge'
import {
  getConnectionPresetIcon,
  getConnectionPresetNameKey,
  getPlatformDefinition,
} from '@/lib/platform-catalog'

const names: Record<string, string> = {
  deepseek: 'DeepSeek',
  openai: 'OpenAI',
  anthropic: 'Anthropic',
  gemini: 'Gemini',
  openrouter: 'OpenRouter',
  siliconflow: 'SiliconFlow',
  minimax: 'MiniMax',
  modelscope: 'ModelScope',
  claudecode: 'Claude Code',
  codex: 'OpenAI Codex',
  fenno: 'OpenAI Codex',
  xai: 'xAI',
  groq: 'Groq',
  mistral: 'Mistral',
  generic: 'API',
  openai_compatible: 'OpenAI Compatible',
}

export function UpstreamIdentity(props: {
  provider?: string
  dialect: string
}) {
  const { t } = useTranslation()
  const id = props.provider || props.dialect
  const presetId = id.replaceAll('_', '-')
  const family = presetId.split('-')[0]
  const platform = getPlatformDefinition(id)
  const key = getConnectionPresetNameKey(presetId)
  const name = key
    ? t(key)
    : platform?.name ||
      names[id] ||
      names[family] ||
      id.replaceAll(/(^|[-_])\w/g, (part) =>
        part.replace(/[-_]/, ' ').toUpperCase()
      )
  const icon =
    platform?.icon ||
    getConnectionPresetIcon(presetId) ||
    (id === 'claudecode' ? 'claude-color' : undefined)
  return (
    <Badge
      variant='outline'
      className='bg-muted/20 max-w-full gap-1.5 rounded-full py-1 pr-2.5 pl-1.5'
    >
      <span aria-hidden='true' className='inline-flex shrink-0'>
        <BrandGlyph icon={icon} size={14} fallbackText={name} />
      </span>
      <span className='truncate'>{name}</span>
    </Badge>
  )
}
