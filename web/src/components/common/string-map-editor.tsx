import {
  ArrowDown,
  ArrowRight,
  ArrowUp,
  Braces,
  List,
  Plus,
  Trash2,
} from 'lucide-react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  isValidMapObject,
  isValidStringMap,
  parseStringMap,
  serializeStringMap,
  stringMapEntryError,
  type StringMapEntry,
} from '@/lib/helpers/string-map'

type StringMapEditorProps = {
  value: string
  structuredOnly?: boolean
  ordered?: boolean
  onChange: (value: string) => void
  keyLabel?: string
  valueLabel?: string
  onBlur?: () => void
} & Omit<React.ComponentProps<'textarea'>, 'value' | 'onChange' | 'onBlur'>

/** String API at the form boundary; rows never discard duplicate keys or drafts. */
export function StringMapEditor({ ...props }: StringMapEditorProps) {
  const { t } = useTranslation()
  const generatedId = useId()
  const id = props.id ?? generatedId
  const [advanced, setAdvanced] = useState(() => !isValidStringMap(props.value))
  const entries = parseStringMap(props.value)
  const [rowIds, setRowIds] = useState(() =>
    (entries ?? []).map(() => crypto.randomUUID())
  )
  if (entries && rowIds.length !== entries.length) {
    setRowIds(entries.map((_, index) => rowIds[index] ?? crypto.randomUUID()))
  }
  // Unsupported external values also stay visible verbatim, even after reset.
  const showJson = !props.structuredOnly && (advanced || entries === null)
  const invalid = props.ordered
    ? entries === null
    : !isValidMapObject(props.value)
  const errorId = `${id}-error`
  const description =
    [props['aria-describedby'], invalid ? errorId : undefined]
      .filter(Boolean)
      .join(' ') || undefined
  const keyLabel = props.keyLabel ?? t('stringMapEditor.key')
  const valueLabel = props.valueLabel ?? t('stringMapEditor.value')
  const {
    value: _value,
    onChange: _onChange,
    structuredOnly: _structuredOnly,
    ordered: _ordered,
    keyLabel: _keyLabel,
    valueLabel: _valueLabel,
    ...restProps
  } = props

  function updateRow(index: number, patch: Partial<StringMapEntry>) {
    if (!entries) return
    props.onChange(
      serializeStringMap(
        entries.map((entry, i) =>
          i === index ? { ...entry, ...patch } : entry
        )
      )
    )
  }

  return (
    <div className='bg-background overflow-hidden rounded-xl border'>
      <div className='bg-muted/30 flex items-center justify-between gap-3 border-b px-3 py-2'>
        <span className='text-muted-foreground text-xs'>
          {showJson
            ? t('stringMapEditor.jsonHint')
            : t('stringMapEditor.rowsHint')}
        </span>
        {!props.structuredOnly && (
          <Button
            type='button'
            variant='ghost'
            size='xs'
            disabled={props.disabled || (showJson && entries === null)}
            onClick={() => setAdvanced(!showJson)}
          >
            {showJson ? (
              <List aria-hidden='true' />
            ) : (
              <Braces aria-hidden='true' />
            )}
            {showJson
              ? t('stringMapEditor.editRows')
              : t('stringMapEditor.editJson')}
          </Button>
        )}
      </div>
      <div className='space-y-3 p-3'>
        {showJson ? (
          <Textarea
            {...restProps}
            id={id}
            value={props.value}
            onChange={(event) => props.onChange(event.target.value)}
            rows={5}
            className='font-mono text-xs'
            aria-invalid={invalid || props['aria-invalid']}
            aria-describedby={description}
          />
        ) : (
          <>
            {!entries?.length && (
              <p className='text-muted-foreground py-3 text-center text-sm'>
                {t('stringMapEditor.empty')}
              </p>
            )}
            {entries?.map((entry, index) => {
              const error =
                props.ordered && entry.key.trim()
                  ? null
                  : stringMapEntryError(entries, index)
              const rowErrorId = `${id}-row-${index}-error`
              return (
                <div key={rowIds[index]} className='space-y-1.5'>
                  <div className='grid grid-cols-[minmax(0,1fr)_auto] items-end gap-2 sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)_auto]'>
                    <label className='col-span-2 space-y-1 text-xs font-medium sm:col-span-1'>
                      <span>{keyLabel}</span>
                      <Input
                        id={index === 0 ? id : `${id}-key-${index}`}
                        ref={
                          index === 0
                            ? (props.ref as React.Ref<HTMLInputElement>)
                            : undefined
                        }
                        name={props.name}
                        value={entry.key}
                        disabled={props.disabled}
                        readOnly={props.readOnly}
                        onBlur={props.onBlur}
                        onChange={(event) =>
                          updateRow(index, { key: event.target.value })
                        }
                        aria-invalid={!!error || props['aria-invalid']}
                        aria-describedby={
                          [description, error ? rowErrorId : undefined]
                            .filter(Boolean)
                            .join(' ') || undefined
                        }
                      />
                    </label>
                    <ArrowRight
                      className='text-muted-foreground mb-2.5 hidden size-4 sm:block'
                      aria-hidden='true'
                    />
                    <label className='space-y-1 text-xs font-medium'>
                      <span>{valueLabel}</span>
                      <Input
                        value={entry.value}
                        disabled={props.disabled}
                        readOnly={props.readOnly}
                        onBlur={props.onBlur}
                        onChange={(event) =>
                          updateRow(index, { value: event.target.value })
                        }
                        aria-describedby={description}
                      />
                    </label>
                    <div className='flex items-center gap-1'>
                      {props.ordered && (
                        <>
                          <Button
                            type='button'
                            variant='ghost'
                            size='icon'
                            disabled={
                              props.disabled || props.readOnly || index === 0
                            }
                            aria-label={t('stringMapEditor.moveUp', {
                              index: index + 1,
                            })}
                            onClick={() => {
                              const next = [...entries]
                              ;[next[index - 1], next[index]] = [
                                next[index],
                                next[index - 1],
                              ]
                              const ids = [...rowIds]
                              ;[ids[index - 1], ids[index]] = [
                                ids[index],
                                ids[index - 1],
                              ]
                              setRowIds(ids)
                              props.onChange(serializeStringMap(next))
                            }}
                          >
                            <ArrowUp className='size-4' />
                          </Button>
                          <Button
                            type='button'
                            variant='ghost'
                            size='icon'
                            disabled={
                              props.disabled ||
                              props.readOnly ||
                              index === entries.length - 1
                            }
                            aria-label={t('stringMapEditor.moveDown', {
                              index: index + 1,
                            })}
                            onClick={() => {
                              const next = [...entries]
                              ;[next[index + 1], next[index]] = [
                                next[index],
                                next[index + 1],
                              ]
                              const ids = [...rowIds]
                              ;[ids[index + 1], ids[index]] = [
                                ids[index],
                                ids[index + 1],
                              ]
                              setRowIds(ids)
                              props.onChange(serializeStringMap(next))
                            }}
                          >
                            <ArrowDown className='size-4' />
                          </Button>
                        </>
                      )}
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon'
                        disabled={props.disabled || props.readOnly}
                        aria-label={t('stringMapEditor.remove', {
                          index: index + 1,
                        })}
                        onClick={() => {
                          setRowIds(rowIds.filter((_, i) => i !== index))
                          props.onChange(
                            serializeStringMap(
                              entries.filter((_, i) => i !== index)
                            )
                          )
                        }}
                      >
                        <Trash2 className='size-4' />
                      </Button>
                    </div>
                  </div>
                  {error && (
                    <p
                      role='alert'
                      id={rowErrorId}
                      className='text-destructive text-xs'
                    >
                      {t(`stringMapEditor.${error}`)}
                    </p>
                  )}
                </div>
              )
            })}
            <Button
              id={!entries?.length ? id : undefined}
              ref={
                !entries?.length
                  ? (props.ref as React.Ref<HTMLButtonElement>)
                  : undefined
              }
              onBlur={props.onBlur}
              type='button'
              variant='outline'
              size='sm'
              disabled={props.disabled || props.readOnly}
              aria-invalid={props['aria-invalid']}
              aria-describedby={description}
              onClick={() =>
                props.onChange(
                  serializeStringMap([
                    ...(entries ?? []),
                    { key: '', value: '' },
                  ])
                )
              }
            >
              <Plus aria-hidden='true' />
              {t('stringMapEditor.add')}
            </Button>
          </>
        )}
        {showJson && entries === null && !invalid && (
          <p className='text-muted-foreground text-xs'>
            {t('stringMapEditor.advancedOnly')}
          </p>
        )}
        {invalid && showJson && (
          <p id={errorId} role='alert' className='text-destructive text-xs'>
            {t('stringMapEditor.invalid')}
          </p>
        )}
      </div>
    </div>
  )
}
