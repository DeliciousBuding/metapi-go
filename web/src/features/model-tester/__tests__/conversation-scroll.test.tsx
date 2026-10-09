import '@testing-library/jest-dom/vitest'
import { fireEvent, render, screen, cleanup } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { TestResponseViewer } from '../components/test-response-viewer'

afterEach(cleanup)
describe('streaming conversation reading', () => {
  it('preserves a manually scrolled reading position until the user returns to latest', () => {
    const props = {
      messages: [],
      content: 'first response',
      reasoningContent: '',
      isRunning: true,
      response: null,
    }
    const { rerender } = render(<TestResponseViewer {...props} />)
    const history = screen.getByRole('log')
    Object.defineProperty(history, 'scrollHeight', {
      configurable: true,
      value: 1000,
    })
    Object.defineProperty(history, 'clientHeight', {
      configurable: true,
      value: 300,
    })
    history.scrollTop = 100
    fireEvent.scroll(history)
    rerender(
      <TestResponseViewer
        {...props}
        content='first response with another chunk'
      />
    )
    expect(history.scrollTop).toBe(100)
    fireEvent.click(screen.getByRole('button', { name: 'Jump to latest' }))
    expect(history.scrollTop).toBeGreaterThanOrEqual(700)
    expect(
      screen.queryByRole('button', { name: 'Jump to latest' })
    ).not.toBeInTheDocument()
  })
})
