import type { Control, FieldPath, FieldValues } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

export function PortalField<T extends FieldValues>({
  control,
  name,
  label,
  type = 'text',
  autoComplete,
}: {
  control: Control<T>
  name: FieldPath<T>
  label: string
  type?: string
  autoComplete?: string
}) {
  const { t } = useTranslation()
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t(label)}</FormLabel>
          <FormControl>
            <Input
              {...field}
              value={String(field.value ?? '')}
              type={type}
              autoComplete={autoComplete}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
