import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AccountFormDialog } from '../account-form-dialog'

const { mutateAsync } = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
}))

vi.mock('../../api', () => ({
  useCreateAccount: () => ({ isPending: false, mutateAsync }),
  useUpdateAccount: () => ({ isPending: false, mutateAsync }),
  useLoginAccount: () => ({ isPending: false, mutateAsync }),
}))

afterEach(() => {
  cleanup()
  mutateAsync.mockReset()
})

describe('AccountFormDialog', () => {
  it('keeps all credential modes interactive and renders password fields', async () => {
    render(
      <AccountFormDialog
        open
        onOpenChange={vi.fn()}
        mode='create'
        sites={[
          {
            id: 1,
            name: 'Browser Smoke Site',
            url: 'https://example.test',
            platform: 'new-api',
            status: 'active',
          },
        ]}
      />
    )

    const tabs = await screen.findAllByRole('tab')
    expect(tabs).toHaveLength(3)

    fireEvent.click(tabs[2])

    await waitFor(() => {
      expect(
        document.querySelector('input[type="password"]')
      ).toBeInTheDocument()
    })
    expect(screen.getAllByRole('tab')[2]).toHaveAttribute('aria-selected', 'true')
  })
})
