import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { SoftToneBadge, SoftToneDot, type SoftTone } from '../soft-tone-badge'

afterEach(() => cleanup())

describe('SoftToneBadge', () => {
  it.each<SoftTone>(['success', 'info', 'warning', 'destructive', 'neutral'])(
    'renders dual-channel status marker for tone=%s while hiding decorative marker from AT',
    (tone) => {
      const { container } = render(
        <SoftToneBadge tone={tone}>Status {tone}</SoftToneBadge>
      )

      const badge = screen.getByText(`Status ${tone}`)
      expect(badge).toHaveAttribute('data-tone', tone)

      const marker = container.querySelector(`[data-tone-marker="${tone}"]`)
      expect(marker).not.toBeNull()
      expect(marker).toHaveAttribute('aria-hidden', 'true')
    }
  )

  it('uses distinct geometric SVG silhouettes for non-success tones so state never relies on color alone', () => {
    const { container } = render(
      <div>
        <SoftToneDot tone='success' />
        <SoftToneDot tone='info' />
        <SoftToneDot tone='warning' />
        <SoftToneDot tone='destructive' />
        <SoftToneDot tone='neutral' />
      </div>
    )

    expect(
      container.querySelector('[data-tone-marker="success"]')?.tagName
    ).toBe('SPAN')
    expect(container.querySelector('[data-tone-marker="info"]')?.tagName).toBe(
      'svg'
    )
    expect(
      container.querySelector('[data-tone-marker="warning"]')?.tagName
    ).toBe('svg')
    expect(
      container.querySelector('[data-tone-marker="destructive"]')?.tagName
    ).toBe('svg')
    expect(
      container.querySelector('[data-tone-marker="neutral"]')?.tagName
    ).toBe('svg')

    const infoPath = container
      .querySelector('[data-tone-marker="info"] path')
      ?.getAttribute('d')
    const warningPath = container
      .querySelector('[data-tone-marker="warning"] path')
      ?.getAttribute('d')
    const destructivePath = container
      .querySelector('[data-tone-marker="destructive"] path')
      ?.getAttribute('d')
    expect(new Set([infoPath, warningPath, destructivePath]).size).toBe(3)
    expect(
      container.querySelector('[data-tone-marker="neutral"] circle')
    ).not.toBeNull()
  })

  it('omits the leading marker when showDot is false', () => {
    const { container } = render(
      <SoftToneBadge tone='warning' showDot={false}>
        Slow
      </SoftToneBadge>
    )

    expect(screen.getByText('Slow')).toBeInTheDocument()
    expect(container.querySelector('[data-tone-marker]')).toBeNull()
  })
})
