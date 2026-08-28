// Component-level axe gate for the Form primitives: a labeled field with a
// description must produce zero structural axe violations — the label is
// properly associated (htmlFor), the description is referenced via
// aria-describedby, and the message stays a valid live region.
import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import axe from 'axe-core'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, it } from 'vitest'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import '@/i18n/config'

type HarnessValues = { token: string }

function FormHarness() {
  const form = useForm<HarnessValues>({ defaultValues: { token: '' } })

  return (
    <Form {...form}>
      <form>
        <FormField
          control={form.control}
          name='token'
          render={({ field }) => (
            <FormItem>
              <FormLabel>Admin token</FormLabel>
              <FormControl>
                <Input type='password' {...field} />
              </FormControl>
              <FormDescription>
                Stored locally, never sent to the server.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  )
}

afterEach(() => cleanup())

describe('Form axe gate', () => {
  it('labeled form field produces zero axe violations', async () => {
    const { container } = render(<FormHarness />)

    const results = await axe.run(container)
    expect(results.violations).toEqual([])
  })

  it('associates the label with the control via htmlFor', () => {
    const { container } = render(<FormHarness />)

    const input = container.querySelector('input')
    const label = container.querySelector('label')
    expect(input).not.toBeNull()
    expect(label).not.toBeNull()
    expect(input!.getAttribute('id')).toBe(label!.getAttribute('for'))
  })
})
