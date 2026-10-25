import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  act,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import i18n from 'i18next'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import { UpstreamPresetPicker } from '../components/upstream-preset-picker'

const newAPI: UpstreamPreset = {
  id: 'new-api',
  name: 'New API',
  label: 'New API',
  provider: 'new-api',
  platform: 'new-api',
  group: 'gateway',
  defaultUrl: '',
  recommendedModels: [],
  requiresBaseUrl: false,
  credentialMode: 'apiKey',
  protocols: [
    'chat',
    'responses',
    'messages',
    'imageGeneration',
    'audioSpeech',
  ],
}
beforeEach(async () => {
  await i18n.changeLanguage('en')
})
afterEach(cleanup)

describe('upstream platform selection', () => {
  it('shows default protocols and capabilities on the product card and selected summary', () => {
    const onSelect = vi.fn()
    function Picker() {
      const [selectedId, setSelectedId] = useState<string | null>(null)
      return (
        <UpstreamPresetPicker
          presets={[newAPI]}
          selectedId={selectedId}
          disabled={false}
          onSelect={(preset, name) => {
            onSelect(preset, name)
            setSelectedId(preset.id)
          }}
        />
      )
    }
    render(<Picker />)
    expect(
      fireEvent.keyDown(
        screen.getByRole('textbox', {
          name: 'Search platforms, Coding Plans or protocols',
        }),
        { key: 'Enter' }
      )
    ).toBe(false)
    expect(onSelect).not.toHaveBeenCalled()
    const card = screen.getByRole('button', { name: 'Use New API' })
    expect(within(card).getByText('New API')).toBeVisible()
    expect(
      within(card).getByText(
        /Default APIs · Chat · Responses · Messages · Image generation · Speech/
      )
    ).toBeVisible()
    expect(card).toHaveAttribute(
      'aria-description',
      'Preset default APIs: Chat · Responses · Messages · Image generation · Speech'
    )
    fireEvent.click(card)
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(newAPI, 'New API')
    const selected = screen.getByRole('region', { name: 'Platform preset' })
    expect(within(selected).getByText('New API')).toBeVisible()
    expect(
      within(selected).getByText(/Chat · Responses · Messages/)
    ).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Change' }))
    expect(screen.getByRole('button', { name: 'Use New API' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
  })

  it('preserves distinct platform products and prioritizes common providers over official direct APIs', () => {
    const products: UpstreamPreset[] = [
      newAPI,
      {
        ...newAPI,
        id: 'bailian',
        name: 'Bailian',
        label: 'Bailian',
        provider: 'bailian',
        group: 'domestic',
      },
      {
        ...newAPI,
        id: 'codingplan',
        name: 'Bailian Coding Plan',
        label: 'Bailian Coding Plan',
        provider: 'bailian',
        group: 'coding',
      },
      {
        ...newAPI,
        id: 'openai',
        name: 'OpenAI',
        label: 'OpenAI',
        provider: 'openai',
        group: 'other',
      },
    ]
    render(
      <UpstreamPresetPicker
        presets={products}
        selectedId={null}
        disabled={false}
        onSelect={vi.fn()}
      />
    )
    expect(screen.getAllByRole('button', { name: /^Use / })).toHaveLength(3)
    expect(
      screen.getByRole('button', { name: 'Use Alibaba Cloud Bailian' })
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Use Alibaba Cloud Coding Plan' })
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Use OpenAI' })
    ).not.toBeInTheDocument()
    fireEvent.change(
      screen.getByRole('textbox', { name: i18n.t('channels.create.search') }),
      { target: { value: 'OpenAI' } }
    )
    expect(screen.getByRole('button', { name: 'Use OpenAI' })).toBeVisible()
  })

  it('finds products by localized capability names and labels custom gateways as templates', async () => {
    render(
      <UpstreamPresetPicker
        presets={[
          newAPI,
          {
            ...newAPI,
            id: 'custom',
            name: 'Custom',
            label: 'Custom',
            provider: 'generic',
            protocols: ['chat', 'responses', 'messages', 'gemini'],
          },
        ]}
        selectedId={null}
        disabled={false}
        onSelect={vi.fn()}
      />
    )
    expect(
      screen.getByRole('button', { name: 'Use Custom gateway' })
    ).toHaveTextContent('Configuration template')
    fireEvent.change(
      screen.getByRole('textbox', { name: i18n.t('channels.create.search') }),
      { target: { value: 'Speech' } }
    )
    expect(screen.getByRole('button', { name: 'Use New API' })).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Use Custom gateway' })
    ).not.toBeInTheDocument()
    await act(async () => {
      await i18n.changeLanguage('zhCN')
    })
    fireEvent.change(
      screen.getByRole('textbox', { name: '搜索平台、Coding Plan 或协议' }),
      { target: { value: '图像' } }
    )
    expect(
      screen.getByRole('button', { name: '使用 New API' })
    ).toHaveTextContent('图像生成')
    fireEvent.change(
      screen.getByRole('textbox', { name: '搜索平台、Coding Plan 或协议' }),
      { target: { value: 'Gemini' } }
    )
    expect(
      screen.getByRole('button', { name: '使用 自定义网关' })
    ).toHaveTextContent('配置模板')
    expect(
      screen.queryByRole('button', { name: '使用 New API' })
    ).not.toBeInTheDocument()
  })
})
