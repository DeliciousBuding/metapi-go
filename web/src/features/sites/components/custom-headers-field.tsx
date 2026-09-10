// metapi-go/features/sites/components — the site `customHeaders` field:
// a JSON textarea plus the read-only "insert example" affordance (#1132).
//
// Thin wrapper in the same shape as `endpoints-editor.tsx`: it owns no form
// state, only `value`/`onChange`. It is rendered inside `<FormControl>`, which
// clones its single child with `id` / `aria-describedby` / `aria-invalid` — so
// the remaining props MUST be forwarded onto the textarea or the
// `<FormLabel htmlFor>` association silently breaks.
//
// Exactly one `<FormDescription>` may live in a `FormItem`: it renders
// `<p id={formDescriptionId}>`, and a second one would duplicate that id and
// steal `aria-describedby`. The example rules below are therefore plain
// helper text (the pattern used across features), not a second description.
//
// The snippets themselves live in `../lib/custom-headers-examples` — this file
// exports only the component, as `react(only-export-components)` requires for
// Fast Refresh.

import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

import { CUSTOM_HEADERS_EXAMPLES } from '../lib/custom-headers-examples'

type CustomHeadersFieldProps = {
  value: string
  /**
   * Always receives the new field text — never a DOM event. The component owns
   * the event→value adaptation so callers (RHF `field.onChange`, the example
   * buttons) share one honest signature, matching `EndpointsEditor`.
   */
  onChange: (value: string) => void
} & Omit<React.ComponentProps<'textarea'>, 'value' | 'onChange'>

export function CustomHeadersField({
  value,
  onChange,
  ...props
}: CustomHeadersFieldProps) {
  const { t } = useTranslation()

  // Fill-only insert semantics (#1132): an example may populate the field only
  // while it is empty, so JSON the user wrote (or that an existing site already
  // carries) is never replaced or merged behind their back. Merging was rejected
  // because every example shares `User-Agent`, so a user-wins merge would be a
  // silent no-op in exactly the case the entry exists for. Whitespace counts as
  // empty, matching `isEmptyOrValidJson` in ../lib/sites-schema.
  const hasContent = value.trim() !== ''

  return (
    <>
      <Textarea
        rows={3}
        placeholder='{"X-Custom-Header":"value"}'
        className='font-mono text-xs'
        value={value}
        onChange={(event) => onChange(event.target.value)}
        {...props}
      />
      <div className='mt-2 flex flex-wrap items-center gap-1.5'>
        <span className='text-muted-foreground text-xs'>
          {t('sites.form.customHeadersExamplesLabel')}
        </span>
        {CUSTOM_HEADERS_EXAMPLES.map((example) => (
          <Button
            key={example.client}
            type='button'
            variant='outline'
            size='xs'
            disabled={hasContent}
            aria-label={t('sites.form.customHeadersExampleInsert', {
              client: example.client,
            })}
            onClick={() => onChange(example.snippet)}
          >
            {example.client}
          </Button>
        ))}
      </div>
      <p className='text-muted-foreground text-xs'>
        {t('sites.form.customHeadersExamplesHint')}
      </p>
    </>
  )
}
