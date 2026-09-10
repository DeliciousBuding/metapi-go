// Component-level tests for the extracted `customHeaders` field (#1132).
// The example data contract is owned by ../../__tests__/custom-headers-examples.test.ts;
// this file covers only the controlled-component behavior and the
// `<FormControl>` prop forwarding the sheet's label association depends on.
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { CUSTOM_HEADERS_EXAMPLES } from '../../lib/custom-headers-examples'
import { CustomHeadersField } from '../custom-headers-field'

afterEach(() => cleanup())

const CLIENTS = CUSTOM_HEADERS_EXAMPLES.map((example) => example.client)

function entry(client: string) {
  return screen.getByRole('button', { name: `Insert ${client} example` })
}

function textarea() {
  return screen.getByRole('textbox') as HTMLTextAreaElement
}

describe('CustomHeadersField fill-only insertion', () => {
  it('offers one enabled entry per example when the field is empty', () => {
    render(<CustomHeadersField value='' onChange={vi.fn()} />)
    for (const client of CLIENTS) {
      expect(entry(client)).toBeEnabled()
    }
  })

  it('emits the chosen snippet through onChange and types nothing itself', () => {
    const onChange = vi.fn()
    render(<CustomHeadersField value='' onChange={onChange} />)

    fireEvent.click(entry('Claude Code'))

    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith(
      '{"User-Agent":"claude-cli/<VERSION>","x-app":"cli"}'
    )
    // Controlled component: it must not write to the DOM node behind the
    // form's back, or the field value and the RHF state would diverge.
    expect(textarea().value).toBe('')
  })

  it('treats whitespace-only content as empty', () => {
    render(<CustomHeadersField value={'  \n '} onChange={vi.fn()} />)
    for (const client of CLIENTS) {
      expect(entry(client)).toBeEnabled()
    }
  })

  it('refuses to touch content the user already filled (never merge, never replace)', () => {
    const onChange = vi.fn()
    const written = '{"User-Agent":"my-handwritten-client","X-Keep":"mine"}'
    render(<CustomHeadersField value={written} onChange={onChange} />)

    for (const client of CLIENTS) {
      expect(entry(client)).toBeDisabled()
      fireEvent.click(entry(client))
    }
    expect(onChange).not.toHaveBeenCalled()
    expect(textarea().value).toBe(written)
  })

  it('forwards FormControl props onto the textarea so the label stays associated', () => {
    // `<FormControl>` clones its single child with id / aria-describedby /
    // aria-invalid. Swallowing them is how `EndpointsEditor` lost its
    // `<FormLabel htmlFor>` association.
    render(
      <CustomHeadersField
        value=''
        onChange={vi.fn()}
        id='fld-1'
        aria-describedby='desc-1'
        aria-invalid={false}
      />
    )
    const node = textarea()
    expect(node).toHaveAttribute('id', 'fld-1')
    expect(node).toHaveAttribute('aria-describedby', 'desc-1')
    expect(node).toHaveAttribute('aria-invalid', 'false')
  })

  it('renders the example rules as helper text, not a second form description', () => {
    const { container } = render(
      <CustomHeadersField value='' onChange={vi.fn()} />
    )
    // A second `<FormDescription>` inside the FormItem would emit another
    // element carrying the same `formDescriptionId`.
    expect(
      container.querySelectorAll('[data-slot="form-description"]')
    ).toHaveLength(0)
    const ids = [...container.querySelectorAll('[id]')].map((n) => n.id)
    expect(new Set(ids).size).toBe(ids.length)
  })
})
