// Behavior test for ModelPill: known models render the brand glyph inside the
// capsule; unknown models still render the name (never an empty pill); blank
// input renders nothing.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { ModelPill } from '../model-pill'

describe('ModelPill', () => {
  afterEach(cleanup)

  it('renders the brand glyph for a known model', () => {
    const { container } = render(<ModelPill model='gpt-4o-mini' />)
    expect(screen.getByText('gpt-4o-mini')).toBeInTheDocument()
    expect(container.querySelector('img')).not.toBeNull()
  })

  it('renders the plain name for an unknown model', () => {
    render(<ModelPill model='my-custom-model-v9' />)
    expect(screen.getByText('my-custom-model-v9')).toBeInTheDocument()
  })

  it('renders nothing for a blank model', () => {
    const { container } = render(<ModelPill model='  ' />)
    expect(container).toBeEmptyDOMElement()
  })
})
