import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AccessPolicyEditor } from '../access-policy-editor'

vi.mock('@/lib/api', () => ({
  api: {
    getImportedUpstreams: vi.fn().mockResolvedValue({
      items: [
        { id: 2, name: 'Beta', enabled: true },
        { id: 3, name: 'Gamma', enabled: true },
      ],
      members: [],
    }),
  },
}))
afterEach(cleanup)

it('locates selected direct channels without mutating the policy and retains unavailable selections', async () => {
  const onChange = vi.fn()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AccessPolicyEditor
        value={{ allowedUpstreamChannelIds: [2, 99], modelIds: ['coding'] }}
        onChange={onChange}
      />
    </QueryClientProvider>
  )
  await screen.findByRole('checkbox', { name: 'Beta' })
  expect(screen.getByText('2 selected')).toBeInTheDocument()
  fireEvent.change(screen.getByRole('textbox', { name: 'Search options' }), {
    target: { value: 'Gamma' },
  })
  expect(
    screen.queryByRole('checkbox', { name: 'Beta' })
  ).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Locate selected' }))
  const selected = await screen.findByRole('checkbox', { name: 'Beta' })
  await waitFor(() => expect(selected).toHaveFocus())
  expect(selected).toBeChecked()
  expect(
    screen.queryByRole('checkbox', { name: 'Gamma' })
  ).not.toBeInTheDocument()
  expect(onChange).not.toHaveBeenCalled()
  fireEvent.click(
    screen.getByRole('checkbox', { name: 'Saved unavailable channel #99' })
  )
  expect(onChange).toHaveBeenCalledWith({
    allowedUpstreamChannelIds: [2],
    modelIds: ['coding'],
  })
})
