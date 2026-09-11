// metapi-go/ui — submit-error focus contract.
//
// A rejected submit must move focus to the first invalid field *of that form*.
// Sheet/dialog forms and page forms are routinely mounted at the same time, so
// the lookup is scoped by the form id `FormItem` stamps rather than by document
// order — an unscoped lookup would focus a field hidden behind the sheet.
// When the invalid node is not itself focusable (a composite `role="group"`
// control), focus falls back to the first focusable element of the same item so
// the error is never announced at a node the user cannot reach.
import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import '@/i18n/config'

type Values = { name: string }

const REQUIRED_RULES = { required: 'Name is required.' }

function NamedForm({ label }: { label: string }) {
  const form = useForm<Values>({ defaultValues: { name: '' } })

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(() => {})}>
        <FormField
          control={form.control}
          name='name'
          rules={REQUIRED_RULES}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{label}</FormLabel>
              <FormControl>
                <input {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <button type='submit'>Submit</button>
      </form>
    </Form>
  )
}

/** Same field, but the control is a non-focusable group wrapping a button. */
function CompositeForm() {
  const form = useForm<Values>({ defaultValues: { name: '' } })

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(() => {})}>
        <FormField
          control={form.control}
          name='name'
          rules={REQUIRED_RULES}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{field.name}</FormLabel>
              <FormControl>
                <div role='group' aria-label='Endpoints'>
                  <button type='button'>Add endpoint</button>
                </div>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <button type='submit'>Submit</button>
      </form>
    </Form>
  )
}

beforeAll(() => {
  // jsdom does not implement scrollIntoView; the focus pass scrolls the item
  // into view before focusing.
  Element.prototype.scrollIntoView = vi.fn()
})

afterEach(() => cleanup())

async function submitAndAwaitInvalid() {
  fireEvent.click(screen.getAllByRole('button', { name: 'Submit' }).at(-1)!)
  await waitFor(() => {
    expect(document.querySelector('[aria-invalid="true"]')).not.toBeNull()
  })
}

describe('Form submit-error focus', () => {
  it('focuses the submitting form field, not an earlier mounted form', async () => {
    render(
      <>
        <NamedForm label='Page name' />
        <NamedForm label='Sheet name' />
      </>
    )

    await submitAndAwaitInvalid()

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByLabelText('Sheet name'))
    })
    expect(screen.getByLabelText('Page name')).not.toHaveFocus()
  })

  it('falls back to the first focusable element of a composite invalid field', async () => {
    render(<CompositeForm />)

    await submitAndAwaitInvalid()

    const group = screen.getByRole('group', { name: 'Endpoints' })
    expect(group).toHaveAttribute('aria-invalid', 'true')
    await waitFor(() => {
      expect(document.activeElement).toBe(
        screen.getByRole('button', { name: 'Add endpoint' })
      )
    })
  })
})
