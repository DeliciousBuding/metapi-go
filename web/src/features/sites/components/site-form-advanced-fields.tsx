import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'

import type { SiteFormValues } from '../lib/sites-schema'
import type { SiteProbeScope } from '../types'
import { CustomHeadersField } from './custom-headers-field'

function nullableBoolToSelectValue(value: boolean | null): string {
  if (value === null) return 'inherit'
  if (value) return 'enabled'
  return 'disabled'
}

function selectValueToNullableBool(
  value: string | null | undefined
): boolean | null {
  if (value === 'inherit' || value == null) return null
  if (value === 'enabled') return true
  return false
}

export function SiteFormAdvancedFields({
  form,
}: {
  form: UseFormReturn<SiteFormValues>
}) {
  const { t } = useTranslation()
  const probeEnabled = form.watch('postRefreshProbeEnabled')
  return (
    <>
      <Separator className='my-6' />
      <fieldset className='min-w-0 space-y-4'>
        <legend className='mb-4 text-sm font-semibold'>
          {t('sites.form.sections.routing')}
        </legend>
        <div className='grid items-start gap-x-5 gap-y-4 sm:grid-cols-2'>
          <FormField
            control={form.control}
            name='globalWeight'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('sites.form.globalWeight')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    step={1}
                    value={field.value}
                    onChange={(event) =>
                      field.onChange(
                        Number.isNaN(event.target.valueAsNumber)
                          ? 0
                          : event.target.valueAsNumber
                      )
                    }
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='maxConcurrency'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('sites.form.maxConcurrency')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={0}
                    step={1}
                    value={field.value}
                    onChange={(event) =>
                      field.onChange(
                        Number.isNaN(event.target.valueAsNumber)
                          ? 0
                          : event.target.valueAsNumber
                      )
                    }
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t('sites.form.maxConcurrencyHint')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        <div className='space-y-4'>
          <FormField
            control={form.control}
            name='postRefreshProbeEnabled'
            render={({ field }) => (
              <FormItem className='flex items-center justify-between gap-4'>
                <div className='space-y-0.5'>
                  <FormLabel>
                    {t('sites.form.postRefreshProbeEnabled')}
                  </FormLabel>
                  <FormDescription>
                    {t('sites.form.postRefreshProbeEnabledHint')}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={(checked) => field.onChange(checked)}
                  />
                </FormControl>
              </FormItem>
            )}
          />
          {probeEnabled && (
            <div className='mt-3 grid items-start gap-x-5 gap-y-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='postRefreshProbeModel'
                render={({ field }) => (
                  <FormItem className='sm:col-span-2'>
                    <FormLabel>
                      {t('sites.form.postRefreshProbeModel')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t(
                          'sites.form.postRefreshProbeModelPlaceholder'
                        )}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='postRefreshProbeScope'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('sites.form.postRefreshProbeScope')}
                    </FormLabel>
                    <Select
                      value={field.value}
                      onValueChange={(value) =>
                        field.onChange(value as SiteProbeScope)
                      }
                    >
                      <FormControl>
                        <SelectTrigger className='w-full'>
                          <SelectValue
                            placeholder={t('sites.form.scopePlaceholder')}
                          >
                            {(value: unknown) =>
                              value === 'all'
                                ? t('sites.form.scopeAll')
                                : t('sites.form.scopeSingle')
                            }
                          </SelectValue>
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value='single'>
                          {t('sites.form.scopeSingle')}
                        </SelectItem>
                        <SelectItem value='all'>
                          {t('sites.form.scopeAll')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='postRefreshProbeLatencyThresholdMs'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('sites.form.postRefreshProbeLatencyThresholdMsLabel')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step={100}
                        value={field.value}
                        onChange={(event) =>
                          field.onChange(
                            Number.isNaN(event.target.valueAsNumber)
                              ? 0
                              : event.target.valueAsNumber
                          )
                        }
                        onBlur={field.onBlur}
                        name={field.name}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('sites.form.postRefreshProbeLatencyThresholdMsHint')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          )}
        </div>
      </fieldset>
      <Separator className='my-6' />
      <fieldset className='min-w-0 space-y-4'>
        <legend className='mb-4 text-sm font-semibold'>
          {t('sites.form.sections.request')}
        </legend>
        <FormField
          control={form.control}
          name='proxyUrl'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('sites.form.proxyUrl')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('sites.form.optionalUrlPlaceholder')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='useSystemProxy'
          render={({ field }) => (
            <FormItem className='flex items-center justify-between gap-4'>
              <div className='space-y-0.5'>
                <FormLabel>{t('sites.form.useSystemProxy')}</FormLabel>
                <FormDescription>
                  {t('sites.form.useSystemProxyHint')}
                </FormDescription>
              </div>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={(checked) => field.onChange(checked)}
                />
              </FormControl>
            </FormItem>
          )}
        />

        <div className='grid items-start gap-x-5 gap-y-4 sm:grid-cols-2'>
          <FormField
            control={form.control}
            name='resinEnabled'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('sites.form.resinEnabled')}</FormLabel>
                <Select
                  value={nullableBoolToSelectValue(field.value)}
                  onValueChange={(value) =>
                    field.onChange(selectValueToNullableBool(value))
                  }
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue>
                        {(selected) => {
                          const resinLabels: Record<string, string> = {
                            enabled: t('sites.form.resinForceOn'),
                            disabled: t('sites.form.resinForceOff'),
                            inherit: t('sites.form.resinInherit'),
                          }
                          return (
                            resinLabels[String(selected)] ??
                            t('sites.form.resinInherit')
                          )
                        }}
                      </SelectValue>
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value='inherit'>
                      {t('sites.form.resinInherit')}
                    </SelectItem>
                    <SelectItem value='enabled'>
                      {t('sites.form.resinForceOn')}
                    </SelectItem>
                    <SelectItem value='disabled'>
                      {t('sites.form.resinForceOff')}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t('sites.form.resinEnabledHint')}
                </FormDescription>
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='useUtls'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('sites.form.useUtls')}</FormLabel>
                <Select
                  value={nullableBoolToSelectValue(field.value)}
                  onValueChange={(value) =>
                    field.onChange(selectValueToNullableBool(value))
                  }
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue>
                        {(selected) => {
                          const utlsLabels: Record<string, string> = {
                            enabled: t('sites.form.utlsForceOn'),
                            disabled: t('sites.form.utlsForceOff'),
                            inherit: t('sites.form.utlsInherit'),
                          }
                          return (
                            utlsLabels[String(selected)] ??
                            t('sites.form.utlsInherit')
                          )
                        }}
                      </SelectValue>
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value='inherit'>
                      {t('sites.form.utlsInherit')}
                    </SelectItem>
                    <SelectItem value='enabled'>
                      {t('sites.form.utlsForceOn')}
                    </SelectItem>
                    <SelectItem value='disabled'>
                      {t('sites.form.utlsForceOff')}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FormDescription>{t('sites.form.useUtlsHint')}</FormDescription>
              </FormItem>
            )}
          />
        </div>

        <FormField
          control={form.control}
          name='customHeaders'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('sites.form.customHeaders')}</FormLabel>
              <FormControl>
                <CustomHeadersField
                  value={field.value}
                  onChange={field.onChange}
                />
              </FormControl>
              <FormDescription>
                {t('sites.form.customHeadersHint')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name='customHeadersOverrideRequestHeaders'
          render={({ field }) => (
            <FormItem className='flex items-center justify-between gap-4'>
              <div className='space-y-0.5'>
                <FormLabel>
                  {t('sites.form.customHeadersOverrideRequestHeaders')}
                </FormLabel>
                <FormDescription>
                  {t('sites.form.customHeadersOverrideRequestHeadersHint')}
                </FormDescription>
              </div>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={(checked) => field.onChange(checked)}
                />
              </FormControl>
            </FormItem>
          )}
        />
      </fieldset>
    </>
  )
}
