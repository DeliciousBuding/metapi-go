import { useTranslation } from 'react-i18next'

import { StringMapEditor } from '@/components/common/string-map-editor'
import { Button } from '@/components/ui/button'

import { CUSTOM_HEADERS_EXAMPLES } from '../lib/custom-headers-examples'

type CustomHeadersFieldProps = {
  value: string
  /**
   * Always receives the new field text — never a DOM event. The component owns
   * the event→value adaptation so callers (RHF `field.onChange`, the example
   * buttons) share one honest signature, matching `EndpointsEditor`.
   */
  onChange: (value: string) => void
  onBlur?: () => void
} & Omit<React.ComponentProps<'textarea'>, 'value' | 'onChange' | 'onBlur'>

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
  // empty, matching the form schema.
  const hasContent = value.trim() !== ''

  return (
    <>
      <StringMapEditor
        value={value}
        onChange={onChange}
        keyLabel={t('stringMapEditor.headerName')}
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
