import '@testing-library/jest-dom/vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'

import { BadgeListCell } from '../badge-list-cell'

afterEach(cleanup)

it('makes overflow values available with keyboard focus', async () => {
  const { container } = render(
    <BadgeListCell
      items={['vision', 'reasoning', 'tools', 'audio'].map((tag) => (
        <span key={tag}>{tag}</span>
      ))}
      max={2}
    />
  )
  expect(screen.getByText('+2')).toBeInTheDocument()
  expect(screen.queryByText('audio')).not.toBeInTheDocument()
  const trigger = container.querySelector<HTMLElement>('[tabindex="0"]')
  if (!trigger) throw new Error('Missing focusable overflow summary')
  act(() => trigger.focus())
  expect(await screen.findByRole('tooltip')).toHaveTextContent(
    'visionreasoningtoolsaudio'
  )
  expect(trigger).toHaveAttribute(
    'aria-describedby',
    screen.getByRole('tooltip').id
  )
  fireEvent.keyDown(trigger, { key: 'Escape' })
  await waitFor(() =>
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  )
})

it('keeps a complete list out of the tab order', () => {
  const { container } = render(
    <BadgeListCell items={[<span key='v'>vision</span>]} />
  )
  expect(screen.getByText('vision')).toBeInTheDocument()
  expect(container.querySelector('[tabindex="0"]')).toBeNull()
})
