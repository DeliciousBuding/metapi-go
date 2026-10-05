import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { DetailField } from '../detail-field'
import { ExpandableText } from '../expandable-text'

afterEach(() => cleanup())

describe('long detail text', () => {
  it('keeps short values fully visible without a disclosure control', () => {
    render(<DetailField label='Model'>gpt-4.1</DetailField>)
    expect(screen.getByText('gpt-4.1')).toBeVisible()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('spans a narrow detail grid and exposes a keyboard-operable full text view', () => {
    const value = `https://example.com/${'very-long-path/'.repeat(12)}`
    const { container } = render(
      <dl className='grid grid-cols-2'>
        <DetailField label='Endpoint'>{value}</DetailField>
      </dl>
    )
    const field = screen.getByText('Endpoint').parentElement
    expect(field).toHaveClass('col-span-2')
    const body = screen.getByText(value)
    expect(body).toHaveClass('line-clamp-2')
    const toggle = container.querySelector('button[aria-controls]')
    expect(toggle).not.toBeNull()
    if (!toggle) throw new Error('disclosure control did not render')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(toggle).toHaveAttribute('aria-controls', body.id)
    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(body).not.toHaveClass('line-clamp-2')
    expect(body).toHaveTextContent(value)
    fireEvent.click(toggle)
    expect(body).toHaveClass('line-clamp-2')
  })

  it('folds multi-line diagnostics even when their character count is small', () => {
    render(<ExpandableText text={'first line\nsecond line\nthird line'} />)
    expect(screen.getByText(/first line/)).toHaveClass('line-clamp-2')
    expect(screen.getByRole('button')).toHaveAttribute('aria-expanded', 'false')
  })

  it('keeps rich long content linked while exposing a touch-sized disclosure', () => {
    const value = `https://example.com/${'very-long-path/'.repeat(12)}`
    render(
      <ExpandableText text={value}>
        <a href={value}>
          <span className='break-all'>{value}</span>
        </a>
      </ExpandableText>
    )

    expect(screen.getByRole('link')).toHaveAttribute('href', value)
    expect(screen.getByRole('link').parentElement).toHaveClass('max-h-[2lh]')
    const toggle = screen.getByRole('button', {
      name: /Show full text|展开全文/,
    })
    expect(toggle).toHaveClass('min-h-10')
    expect(toggle).toHaveClass('min-w-10')
    expect(toggle).toHaveClass('mt-1.5')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('button', { name: /Show less|收起/ })).toHaveClass(
      'min-w-10',
      'min-h-10'
    )
    expect(screen.getByRole('link')).toHaveTextContent(value)
  })
})
