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

// Raw API fixture includes the gateway's fourteen explicit capabilities.
const newAPI: UpstreamPreset = JSON.parse(`{
  "id": "new-api-connection", "name": "New API", "label": "New API",
  "provider": "new-api", "platform": "new-api", "group": "gateway",
  "defaultUrl": "", "recommendedModels": [],
  "protocols": ["chat", "responses", "messages", "completions", "embeddings",
    "rerank", "imageGeneration", "imageEdit", "imageVariation", "audioSpeech",
    "audioTranscription", "audioTranslation", "moderations", "video"]
}`)

const description =
  'Text & conversation: Chat, Responses, Messages, Completions, Moderation; Vectors & retrieval: Embeddings, Rerank; Images: Image generation, Image edits, Image variations; Audio: Speech, Transcription, Translation; Video: Video'
const titles = [
  'Text & conversation: Chat, Responses, Messages, Completions, Moderation',
  'Vectors & retrieval: Embeddings, Rerank',
  'Images: Image generation, Image edits, Image variations',
  'Audio: Speech, Transcription, Translation',
  'Video: Video',
]

beforeEach(async () => {
  await i18n.changeLanguage('en')
})
afterEach(cleanup)

describe('upstream preset capability summaries', () => {
  it('summarizes fourteen New API capabilities in five groups and preserves accessible details through selection', () => {
    const onSelect = vi.fn()
    function Picker() {
      const [selectedId, setSelectedId] = useState<string | null>(null)
      return (
        <UpstreamPresetPicker
          presets={[newAPI]}
          selectedId={selectedId}
          manual={false}
          disabled={false}
          onSelect={(preset, name) => {
            onSelect(preset, name)
            setSelectedId(preset.id)
          }}
          onCustom={vi.fn()}
        />
      )
    }
    render(<Picker />)
    const card = screen.getByRole('button', { name: 'Use New API' })
    expect(card).toHaveAccessibleDescription(description)
    expect(within(card).getByText('New API')).toBeVisible()
    expect(within(card).getByText('Chat / Responses +3')).toBeVisible()
    expect(within(card).getByText('Vectors & retrieval · 2')).toBeVisible()
    expect(within(card).getByText('Images · 3')).toBeVisible()
    expect(within(card).getByText('Audio · 3')).toBeVisible()
    expect(within(card).getByText('Video · 1')).toBeVisible()
    expect(
      within(card).getAllByTitle(
        /^(Text & conversation|Vectors & retrieval|Images|Audio|Video):/
      )
    ).toHaveLength(5)
    for (const title of titles) {
      expect(within(card).getByTitle(title)).toBeVisible()
    }

    fireEvent.click(card)
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(newAPI, 'New API')
    const compact = screen.getByRole('region', { name: 'Platform preset' })
    expect(compact).toHaveAccessibleDescription(description)
    expect(within(compact).getByText('New API')).toBeVisible()
    for (const title of titles) {
      expect(within(compact).getByTitle(title)).toBeVisible()
    }
    expect(
      within(compact).getAllByTitle(
        /^(Text & conversation|Vectors & retrieval|Images|Audio|Video):/
      )
    ).toHaveLength(5)
    fireEvent.click(screen.getByRole('button', { name: 'Change' }))
    expect(screen.getByRole('button', { name: 'Use New API' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(onSelect).toHaveBeenCalledTimes(1)
  })

  it('does not imply unlisted media capabilities for a conversation-only preset', () => {
    const onSelect = vi.fn()
    render(
      <UpstreamPresetPicker
        presets={[{ ...newAPI, protocols: ['chat'] }]}
        selectedId={null}
        manual={false}
        disabled
        onSelect={onSelect}
        onCustom={vi.fn()}
      />
    )
    const card = screen.getByRole('button', { name: 'Use New API' })
    expect(card).toBeDisabled()
    expect(card).toHaveAccessibleDescription('Text & conversation: Chat')
    expect(within(card).getByText('Chat')).toBeVisible()
    expect(
      within(card).queryByText(/Images|Audio|Video|Vectors/)
    ).not.toBeInTheDocument()
    fireEvent.click(card)
    expect(onSelect).not.toHaveBeenCalled()
  })
})
