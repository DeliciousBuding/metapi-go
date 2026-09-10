// Focused a11y test for ScheduleEditor's FormControl prop forwarding (#1300).
// ScheduleEditor is a composite — a group of selects/inputs with no single
// labelable element — so it must forward the FormControl-injected
// id / aria-describedby / aria-invalid onto a role="group" root and name that
// group via aria-labelledby -> the FormLabel id (`formLabelIdFor`).
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ScheduleEditor } from '../schedule-editor'

beforeAll(() => {
  // base-ui Select needs matchMedia under jsdom.
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

describe('ScheduleEditor accessible name (#1300)', () => {
  it('forwards the injected id onto a role=group named by the label', () => {
    render(
      <ScheduleEditor
        value={undefined}
        onChange={vi.fn()}
        id='fld-1'
        aria-describedby='desc-1'
        aria-invalid={false}
      />
    )
    const group = screen.getByRole('group')
    expect(group).toHaveAttribute('id', 'fld-1')
    expect(group).toHaveAttribute('aria-describedby', 'desc-1')
    expect(group).toHaveAttribute('aria-invalid', 'false')
    // formLabelIdFor('fld-1') === 'fld-1-label' — the id FormLabel renders.
    expect(group).toHaveAttribute('aria-labelledby', 'fld-1-label')
  })
})
