import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { MarkdownContent } from '../markdown-content'

afterEach(cleanup)
describe('shared Markdown content', () => {
  it('renders code and useful links while dropping scripts and unsafe destinations', () => {
    const { container } = render(
      <MarkdownContent>
        {
          '## Example\n\n```ts\nconst greeting = "你好"\n```\n\n[Documentation](https://example.com/docs)\n\n[Unsafe](javascript:alert%281%29)\n\n<script>alert(1)</script>\n\n<img src="https://example.com/tracker" onerror="alert(1)">\n\n![diagram](https://example.com/remote.png)'
        }
      </MarkdownContent>
    )
    expect(screen.getByRole('heading', { name: 'Example' })).toBeInTheDocument()
    expect(container.querySelector('code')).toHaveTextContent(
      'const greeting = "你好"'
    )
    const codeBlock = container.querySelector('pre')
    codeBlock?.focus()
    expect(codeBlock).toHaveFocus()
    expect(screen.getByRole('link', { name: 'Documentation' })).toHaveAttribute(
      'href',
      'https://example.com/docs'
    )
    expect(container.querySelector('a[href^="javascript:"]')).toBeNull()
    expect(container.querySelector('script, img, [onerror]')).toBeNull()
    expect(screen.getByText('diagram')).toBeInTheDocument()
  })
})
