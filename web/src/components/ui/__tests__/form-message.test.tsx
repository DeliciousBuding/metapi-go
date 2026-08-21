// metapi-go/ui — FormMessage announcement contract.
// Field errors rendered by the shared form primitives must be announced by
// assistive tech the moment they appear: the message element carries an
// assertive live-region role, and the invalid control references it through
// aria-describedby (wired by FormControl).
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, it } from 'vitest'

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import '@/i18n/config'

type HarnessValues = { token: string }

function FormMessageHarness() {
  const form = useForm<HarnessValues>({ defaultValues: { token: '' } })

  useEffect(() => {
    form.setError('token', { message: 'The login token is invalid.' })
  }, [form])

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
                <input type='password' {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  )
}

afterEach(() => cleanup())

describe('FormMessage error announcement', () => {
  it('renders the field error inside an assertive live region (role=alert)', async () => {
    render(<FormMessageHarness />)

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('The login token is invalid.')
  })

  it('links the invalid control to the error message via aria-describedby', async () => {
    render(<FormMessageHarness />)

    const alert = await screen.findByRole('alert')
    const input = screen.getByLabelText('Admin token')
    const describedBy = input.getAttribute('aria-describedby') ?? ''

    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(describedBy.split(' ')).toContain(alert.id)
  })
})
