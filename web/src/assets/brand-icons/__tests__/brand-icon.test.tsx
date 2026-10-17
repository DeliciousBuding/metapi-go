import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'

import { BrandGlyph, BrandIcon } from '../BrandIcon'

afterEach(cleanup)

it('keeps the vendored icon and requested size for known models', () => {
  render(<BrandIcon model='claude-example' size={40} />)
  expect(screen.getByRole('img', { name: 'Anthropic' })).toHaveStyle({
    width: '40px',
    height: '40px',
  })
})

it('uses the same neutral lettermark in standalone and inline identities', () => {
  const { container } = render(
    <>
      <BrandIcon model='vendor/模型' size={40} />
      <BrandGlyph model='vendor/模型' size={16} />
    </>
  )
  expect(screen.queryByRole('img')).not.toBeInTheDocument()
  const marks = screen.getAllByText('V模')
  expect(marks).toHaveLength(2)
  for (const mark of marks) {
    expect(mark).toHaveAttribute('aria-hidden', 'true')
    expect(mark).toHaveStyle({
      background: 'var(--color-secondary)',
      color: 'var(--color-secondary-foreground)',
    })
  }
  expect(container).toHaveTextContent('V模V模')
})

it('keeps an identity when a known icon cannot load', () => {
  render(<BrandIcon model='claude-example' size={40} />)
  fireEvent.error(screen.getByRole('img'))
  expect(screen.queryByRole('img')).not.toBeInTheDocument()
  expect(screen.getByText('AN')).toHaveAttribute('aria-hidden', 'true')
})
