// Behavior test for ClientPill: appName wins over family; known clients get
// the brand glyph (Codex CLI → openai, Claude Code → claude); unknown clients
// render plain text; empty input renders nothing.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { ClientPill } from '../client-pill'

describe('ClientPill', () => {
  afterEach(cleanup)

  it('prefers the app name and resolves the Codex glyph', () => {
    const { container } = render(
      <ClientPill appName='Codex CLI' family='codex' />
    )
    expect(screen.getByText('Codex CLI')).toBeInTheDocument()
    expect(container.querySelector('img')).not.toBeNull()
  })

  it('resolves the Claude glyph for Claude Code', () => {
    const { container } = render(<ClientPill appName='Claude Code' />)
    expect(container.querySelector('img')).not.toBeNull()
  })

  it('falls back to the family label', () => {
    render(<ClientPill appName='' family='openai-node' />)
    expect(screen.getByText('openai-node')).toBeInTheDocument()
  })

  it('renders plain text for an unknown client', () => {
    const { container } = render(<ClientPill family='my-custom-agent' />)
    expect(screen.getByText('my-custom-agent')).toBeInTheDocument()
    expect(container.querySelector('img')).toBeNull()
  })

  it('renders nothing when both fields are empty', () => {
    const { container } = render(<ClientPill appName='' family='' />)
    expect(container).toBeEmptyDOMElement()
  })
})
