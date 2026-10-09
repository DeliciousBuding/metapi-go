import { describe, expect, it } from 'vitest'

import {
  PLATFORM_CONNECTION_TEMPLATES,
  getConnectionPresetIcon,
  getPlatformDefinition,
  getPlatformDisplayName,
} from '../platform-catalog'

describe('platform display and connection presets', () => {
  it('uses readable names without rewriting unknown platform IDs', () => {
    expect(getPlatformDisplayName('new-api')).toBe('New API')
    expect(getPlatformDisplayName('openai')).toBe('OpenAI')
    expect(getPlatformDisplayName('my-custom-platform')).toBe(
      'my-custom-platform'
    )
  })

  it('keeps service URLs out of local platform templates', () => {
    for (const template of PLATFORM_CONNECTION_TEMPLATES) {
      expect(template.group).not.toBe('api')
      expect(template.url).toBe('')
    }
    expect(getConnectionPresetIcon('xai-api')).toBe('xai')
    expect(getConnectionPresetIcon('deepseek-claude')).toBe('deepseek-color')
    expect(getConnectionPresetIcon('kimi-coding-claude')).toBe('moonshot')
    expect(getConnectionPresetIcon('xiaomi-token-plan-claude')).toBe(
      'xiaomimimo'
    )
    expect(getConnectionPresetIcon('qiniu-openai')).toBe('qiniu-color')
    expect(getConnectionPresetIcon('ppio-openai')).toBe('ppio-color')
    expect(getConnectionPresetIcon('new-provider')).toBeUndefined()
    expect(getPlatformDefinition('grok')?.group).toBe('oauth')
    expect(getPlatformDefinition('sensetime')?.selectable).toBe(false)
  })
})
