import { useQuery } from '@tanstack/react-query'
import { useId, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { formLabelIdFor } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  channelPluginsApi,
  channelPluginQueryKey,
  type PluginBinding,
  type ChannelPluginOption,
} from '@/lib/api/channel-plugins'
import type { ImportedEndpointConfig } from '@/lib/api/imported-upstreams'

type BindingProps = Omit<ComponentProps<'div'>, 'onChange'> & {
  value: PluginBinding[]
  onChange: (value: PluginBinding[]) => void
  endpoints: ImportedEndpointConfig
  disabled?: boolean
}
export function ChannelPluginBindings({
  value,
  onChange,
  endpoints,
  disabled,
  ...restProps
}: BindingProps) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: channelPluginQueryKey,
    queryFn: channelPluginsApi.list,
  })
  const props = {
    value,
    onChange,
    endpoints,
    disabled: disabled || query.isError,
  }
  const items = query.data?.items ?? []
  const orderId = useId()
  const orderedItems = [
    ...props.value.flatMap((binding) =>
      items.filter((item) => item.key === binding.key)
    ),
    ...items.filter(
      (item) => !props.value.some((binding) => binding.key === item.key)
    ),
  ]
  const missing = props.value.filter(
    (binding) => !items.some((item) => item.key === binding.key)
  )
  function update(key: string, patch: Partial<PluginBinding>) {
    props.onChange(
      props.value.map((binding) =>
        binding.key === key ? { ...binding, ...patch } : binding
      )
    )
  }
  function move(key: string, delta: number) {
    const index = props.value.findIndex((binding) => binding.key === key)
    const nextIndex = index + delta
    if (index < 0 || nextIndex < 0 || nextIndex >= props.value.length) return
    const next = [...props.value]
    ;[next[index], next[nextIndex]] = [next[nextIndex], next[index]]
    props.onChange(next)
  }
  return (
    <div
      {...restProps}
      tabIndex={-1}
      role='group'
      aria-label={
        restProps.id || restProps['aria-labelledby']
          ? undefined
          : t('channels.plugins.bindings')
      }
      aria-labelledby={
        restProps['aria-labelledby'] ??
        (restProps.id ? formLabelIdFor(restProps.id) : undefined)
      }
      className='space-y-3'
    >
      <QueryErrorBanner
        error={query.error}
        messageKey='channels.plugins.loadError'
        onRetry={() => void query.refetch()}
        isRetrying={query.isFetching}
      />
      {query.isPending && <p role='status'>{t('common.loading')}</p>}
      {!query.isError &&
        !query.isPending &&
        items.length === 0 &&
        missing.length === 0 && (
          <p className='text-muted-foreground text-sm'>
            {t('channels.plugins.emptyBindings')}
          </p>
        )}
      {orderedItems.map((item) => {
        const binding = props.value.find((binding) => binding.key === item.key)
        const available =
          item.manifest?.protocols.filter(
            (protocol) => props.endpoints[protocol]
          ) ?? []
        const enabled = !!binding
        const index = props.value.findIndex(
          (binding) => binding.key === item.key
        )
        return (
          <section key={item.key} className='space-y-3 rounded-lg border p-3'>
            <label className='flex items-center gap-2 text-sm font-medium'>
              <Checkbox
                aria-describedby={
                  binding ? `${orderId}-${item.key}` : undefined
                }
                checked={enabled}
                disabled={
                  props.disabled ||
                  (!binding &&
                    (!item.enabled ||
                      !available.length ||
                      !!item.loadError ||
                      props.value.length >= 8))
                }
                onCheckedChange={(checked) =>
                  props.onChange(
                    checked
                      ? [
                          ...props.value,
                          { key: item.key, protocols: available },
                        ]
                      : props.value.filter(
                          (binding) => binding.key !== item.key
                        )
                  )
                }
              />
              {item.manifest?.name ?? item.key}
              {!item.enabled && (
                <span className='text-warning-soft-fg'>
                  {t('channels.plugins.disabled')}
                </span>
              )}
              {item.loadError && (
                <span className='text-destructive'>
                  {t('channels.plugins.unavailable')}
                </span>
              )}
            </label>
            {binding && (
              <div className='flex items-center justify-between gap-2'>
                <span
                  id={`${orderId}-${item.key}`}
                  className='text-muted-foreground text-xs'
                >
                  {t('channels.plugins.order', { order: index + 1 })}
                </span>
                <div className='flex gap-1'>
                  <Button
                    type='button'
                    variant='outline'
                    size='xs'
                    disabled={props.disabled || index === 0}
                    onClick={() => move(item.key, -1)}
                    aria-label={t('channels.plugins.moveEarlier', {
                      name: item.manifest?.name ?? item.key,
                    })}
                  >
                    {t('channels.plugins.earlier')}
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='xs'
                    disabled={
                      props.disabled || index === props.value.length - 1
                    }
                    onClick={() => move(item.key, 1)}
                    aria-label={t('channels.plugins.moveLater', {
                      name: item.manifest?.name ?? item.key,
                    })}
                  >
                    {t('channels.plugins.later')}
                  </Button>
                </div>
              </div>
            )}
            {enabled && binding && (
              <>
                <div className='flex flex-wrap gap-3'>
                  {[...new Set([...available, ...binding.protocols])].map(
                    (protocol) => (
                      <label
                        key={protocol}
                        className='flex items-center gap-2 text-sm'
                      >
                        <Checkbox
                          checked={binding.protocols.includes(protocol)}
                          disabled={
                            props.disabled ||
                            (!binding.protocols.includes(protocol) &&
                              !available.includes(protocol)) ||
                            (binding.protocols.length === 1 &&
                              binding.protocols.includes(protocol))
                          }
                          onCheckedChange={(checked) =>
                            update(item.key, {
                              protocols: checked
                                ? [...binding.protocols, protocol]
                                : binding.protocols.filter(
                                    (current) => current !== protocol
                                  ),
                            })
                          }
                        />
                        {t(`channels.capabilities.names.${protocol}`)}
                      </label>
                    )
                  )}
                </div>
                {item.manifest?.options?.map((option) => (
                  <PluginOptionField
                    key={option.key}
                    option={option}
                    value={binding.options?.[option.key] ?? option.default}
                    disabled={props.disabled}
                    onChange={(value) =>
                      update(item.key, {
                        options: { ...binding.options, [option.key]: value },
                      })
                    }
                  />
                ))}
              </>
            )}
          </section>
        )
      })}
      {missing.map((binding) => (
        <label key={binding.key} className='flex items-center gap-2 text-sm'>
          <Checkbox
            checked
            disabled={props.disabled || query.isError || query.isPending}
            onCheckedChange={() =>
              props.onChange(
                props.value.filter((item) => item.key !== binding.key)
              )
            }
          />
          {binding.key} · {t('channels.plugins.unavailable')}
        </label>
      ))}
    </div>
  )
}

function PluginOptionField(props: {
  option: ChannelPluginOption
  value: string | boolean | number
  onChange: (value: string | boolean | number) => void
  disabled?: boolean
}) {
  const id = useId()
  const option = props.option
  return (
    <div className='space-y-1.5'>
      <label id={`${id}-label`} htmlFor={id} className='text-sm'>
        {option.label}
      </label>
      {option.type === 'boolean' ? (
        <Switch
          id={id}
          checked={props.value === true}
          onCheckedChange={props.onChange}
          disabled={props.disabled}
        />
      ) : option.type === 'select' ? (
        <Select
          value={String(props.value)}
          onValueChange={(value) => props.onChange(String(value))}
          disabled={props.disabled}
        >
          <SelectTrigger id={id}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent aria-labelledby={`${id}-label`}>
            {option.choices?.map((choice) => (
              <SelectItem key={choice} value={choice}>
                {choice}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <Input
          id={id}
          type={
            option.type === 'number' || option.type === 'integer'
              ? 'number'
              : 'text'
          }
          min={option.min}
          max={option.max}
          step={option.type === 'integer' ? 1 : 'any'}
          value={String(props.value)}
          disabled={props.disabled}
          onChange={(event) =>
            props.onChange(
              option.type === 'number' || option.type === 'integer'
                ? event.target.valueAsNumber
                : event.target.value
            )
          }
        />
      )}
    </div>
  )
}
