import '@testing-library/jest-dom/vitest'
import {
  cleanup,
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
  it('selects one platform card without presenting protocol or capability choices', () => {
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
    const card = screen.getByRole('button', { name: 'Use New API' })
    expect(within(card).getByText('New API')).toBeVisible()
    expect(
      within(card).queryByText(/Chat|Responses|Messages|Images|Audio/)
    ).not.toBeInTheDocument()
    expect(card).not.toHaveAttribute('aria-description')
    fireEvent.click(card)
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(newAPI, 'New API')
    const selected = screen.getByRole('region', { name: 'Platform preset' })
    expect(within(selected).getByText('New API')).toBeVisible()
    expect(
      within(selected).queryByText(/Chat|Responses|Messages/)
    ).not.toBeInTheDocument()
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
    expect(screen.getByRole('button', { name: 'Use Bailian' })).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Use Bailian Coding Plan' })
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
})
