import { describe, expect, it } from 'vitest'

import {
  CONNECTION_PRESETS,
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

  it('keeps API-key presets on API adapters and separates public xAI from Grok OAuth', () => {
    for (const preset of CONNECTION_PRESETS) {
      expect(getPlatformDefinition(preset.platform)?.group).toBe('api')
      expect(preset.platform).not.toBe('sensetime')
    }
    expect(
      CONNECTION_PRESETS.find((preset) => preset.id === 'xai-api')
    ).toMatchObject({
      platform: 'openai',
      url: 'https://api.x.ai',
    })
    expect(getPlatformDefinition('grok')?.group).toBe('oauth')
  })
})
