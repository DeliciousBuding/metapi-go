// metapi-go/data-table — ported from newapi
import type { ColumnFiltersState, OnChangeFn } from '@tanstack/react-table'
import * as React from 'react'

import { useDebounce } from './use-debounce'

type UseDebouncedColumnFilterOptions = {
  columnFilters: ColumnFiltersState
  columnId: string
  onColumnFiltersChange: OnChangeFn<ColumnFiltersState>
  delay?: number
}

export function useDebouncedColumnFilter({
  columnFilters,
  columnId,
  onColumnFiltersChange,
  delay = 500,
}: UseDebouncedColumnFilterOptions) {
  const value =
    (columnFilters.find((filter) => filter.id === columnId)?.value as
      | string
      | undefined) ?? ''
  const [inputValue, setInputValue] = React.useState(value)
  const [pendingValue, setPendingValue] = React.useState(value)
  const isComposingRef = React.useRef(false)
  const debouncedValue = useDebounce(pendingValue, delay)
  const onColumnFiltersChangeRef = React.useRef(onColumnFiltersChange)
  onColumnFiltersChangeRef.current = onColumnFiltersChange

  React.useEffect(() => {
    // Keep the input aligned when URL state changes outside the local field.
    if (!isComposingRef.current) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setInputValue(value)
    }
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPendingValue(value)
  }, [value])

  React.useEffect(() => {
    if (debouncedValue === value) return

    onColumnFiltersChangeRef.current((previous) => {
      const filters = previous.filter((filter) => filter.id !== columnId)
      return debouncedValue
        ? [...filters, { id: columnId, value: debouncedValue }]
        : filters
    })
  }, [columnId, debouncedValue, value])

  const updateInputValue = React.useCallback((nextValue: string) => {
    setInputValue(nextValue)

    if (!isComposingRef.current) {
      setPendingValue(nextValue)
    }
  }, [])

  const handleChange = React.useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      updateInputValue(event.target.value)
    },
    [updateInputValue]
  )

  const handleCompositionStart = React.useCallback(() => {
    isComposingRef.current = true
  }, [])

  const handleCompositionEnd = React.useCallback(
    (event: React.CompositionEvent<HTMLInputElement>) => {
      isComposingRef.current = false
      const nextValue = event.currentTarget.value
      setInputValue(nextValue)
      setPendingValue(nextValue)
    },
    []
  )

  const resetInput = React.useCallback(() => {
    isComposingRef.current = false
    setInputValue('')
    setPendingValue('')
  }, [])

  return {
    value,
    inputValue,
    setInputValue: updateInputValue,
    onChange: handleChange,
    onCompositionStart: handleCompositionStart,
    onCompositionEnd: handleCompositionEnd,
    resetInput,
  }
}
