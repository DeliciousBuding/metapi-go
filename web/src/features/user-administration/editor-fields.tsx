import type { Control, FieldPath, FieldValues } from 'react-hook-form'
import { z } from 'zod'

import {
  FormField,
  FormItem,
  FormLabel,
  FormControl,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

export type EditorState = { dirty: boolean; pending: boolean }

export function TextField<T extends FieldValues>({
  control,
  name,
  label,
  type = 'text',
  disabled = false,
}: {
  control: Control<T>
  name: FieldPath<T>
  label: string
  type?: string
  disabled?: boolean
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input
              {...field}
              value={String(field.value ?? '')}
              type={type}
              step={type === 'number' ? '0.000001' : undefined}
              disabled={disabled}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function BooleanField<T extends FieldValues>({
  control,
  name,
  label,
}: {
  control: Control<T>
  name: FieldPath<T>
  label: string
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem className='flex items-center justify-between gap-4'>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Switch
              name={field.name}
              checked={Boolean(field.value)}
              onCheckedChange={field.onChange}
              onBlur={field.onBlur}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

const decimalPattern = /^(?:0|[1-9]\d*)(?:\.\d{1,6})?$/
export const amountInput = (message: string, positive = false) =>
  z
    .string()
    .trim()
    .regex(decimalPattern, message)
    .refine(
      (value) =>
        Number(value) <= 1e9 &&
        (positive ? Number(value) > 0 : Number(value) >= 0),
      message
    )
