import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ModelDetailSheet } from '../model-detail-sheet'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

afterEach(cleanup)

it('opens at the model identity instead of focusing the tester action at the bottom', async () => {
  render(
    <ModelDetailSheet
      open
      onOpenChange={vi.fn()}
      model={{
        name: 'example-long-model-name',
        description: 'Model details',
        accountCount: 0,
        tokenCount: 0,
        avgLatency: null,
        successRate: null,
        tags: [],
        supportedEndpointTypes: [],
        pricingSources: [],
        accounts: [],
      }}
    />
  )
  const heading = await screen.findByRole('heading', {
    name: 'example-long-model-name',
  })
  await waitFor(() => expect(heading).toHaveFocus())
  expect(screen.getByRole('button', { name: 'Open tester' })).not.toHaveFocus()
})
