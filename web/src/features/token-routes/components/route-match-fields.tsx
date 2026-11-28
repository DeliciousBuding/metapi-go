import { useMemo } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import type { RouteFormValues } from '../lib/routes-schema'
import type { RouteSummaryRow } from '../types'
import { getModelPatternError, isRegexModelPattern } from '../utils'

export function RoutePatternField({
  form,
}: {
  form: UseFormReturn<RouteFormValues>
}) {
  const { t } = useTranslation()
  const modelPattern = form.watch('modelPattern') ?? ''
  const isRegex = isRegexModelPattern(modelPattern)
  const patternError = useMemo(
    () => (modelPattern ? getModelPatternError(modelPattern) : null),
    [modelPattern]
  )
  return (
    <FormField
      control={form.control}
      name='modelPattern'
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('tokenRoutes.formPattern.modelRule')}</FormLabel>
          <FormControl>
            <Input
              placeholder={t('tokenRoutes.formPattern.modelRulePlaceholder')}
              className='font-mono'
              aria-invalid={Boolean(patternError)}
              {...field}
              value={field.value ?? ''}
            />
          </FormControl>
          <FormDescription>
            {isRegex
              ? t('tokenRoutes.formPattern.regexHint')
              : t('tokenRoutes.formPattern.exactHint')}
          </FormDescription>
          {patternError && (
            <p className='text-destructive text-sm'>{patternError}</p>
          )}
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function GroupModeFields({
  form,
  availableRoutes,
}: {
  form: UseFormReturn<RouteFormValues>
  availableRoutes: RouteSummaryRow[]
}) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='displayName'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('tokenRoutes.formGroup.displayName')}</FormLabel>
            <FormControl>
              <Input
                placeholder={t('tokenRoutes.formGroup.displayNamePlaceholder')}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>
              {t('tokenRoutes.formGroup.displayNameHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name='sourceRouteIds'
        render={({ field }) => {
          const selected = field.value ?? []
          const selectedSet = new Set(selected)
          const toggleRoute = (routeId: number, checked: boolean) => {
            if (checked) {
              field.onChange([...selected, routeId])
            } else {
              field.onChange(selected.filter((id) => id !== routeId))
            }
          }
          return (
            <FormItem>
              <FormLabel>{t('tokenRoutes.formGroup.sourceRoutes')}</FormLabel>
              <FormDescription>
                {t('tokenRoutes.formGroup.sourceRoutesHint')}
              </FormDescription>
              <div className='max-h-56 space-y-1 overflow-y-auto rounded-lg border p-2'>
                {availableRoutes.length === 0 && (
                  <p className='text-muted-foreground px-2 py-4 text-center text-sm'>
                    {t('tokenRoutes.formGroup.sourceRoutesEmpty')}
                  </p>
                )}
                {availableRoutes.map((route) => (
                  <label
                    key={route.id}
                    className='hover:bg-muted flex items-center gap-2 rounded px-2 py-1'
                  >
                    <Checkbox
                      checked={selectedSet.has(route.id)}
                      onCheckedChange={(value) =>
                        toggleRoute(route.id, Boolean(value))
                      }
                    />
                    <span className='truncate font-mono text-sm'>
                      {route.modelPattern}
                    </span>
                  </label>
                ))}
              </div>
              <FormMessage />
            </FormItem>
          )
        }}
      />
    </>
  )
}
