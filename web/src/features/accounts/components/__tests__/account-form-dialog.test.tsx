// Regression guard: the account form renders all three credential modes.
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import type { Site } from '../../types'
import { AccountFormDialog } from '../account-form-dialog'

vi.mock('../../api', () => ({
  useCreateAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useLoginAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

afterEach(() => cleanup())

const sites: Site[] = [
  {
    id: 1,
    name: 'OpenRouter',
    url: 'https://openrouter.ai',
    platform: 'openai',
    status: 'active',
  },
]

describe('AccountFormDialog (create mode)', () => {
  it('renders the three credential-mode tabs without error', async () => {
    render(
      <AccountFormDialog
        open
        onOpenChange={() => {}}
        mode='create'
        account={null}
        sites={sites}
      />
    )

    await waitFor(() => {
      // i18n configured → the tabs resolve to real labels (en: Session /
      // API Key / Password; zh-CN: 会话 / API 密钥 / 密码登录).
      expect(
        screen.getAllByRole('tab').length
      ).toBeGreaterThanOrEqual(3)
    })
  })
})
