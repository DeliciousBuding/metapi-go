import type { UseFormReturn } from 'react-hook-form'

import { Separator } from '@/components/ui/separator'

import type { SiteFormValues } from '../lib/sites-schema'
import { SiteFormRequestFields } from './site-form-request-fields'
import { SiteFormRoutingFields } from './site-form-routing-fields'

export function SiteFormAdvancedFields({
  form,
}: {
  form: UseFormReturn<SiteFormValues>
}) {
  return (
    <>
      <Separator className='my-6' />
      <SiteFormRoutingFields form={form} />
      <Separator className='my-6' />
      <SiteFormRequestFields form={form} />
    </>
  )
}
