// metapi-go/data-table — ported from newapi
// use-debounce vendored locally into data-table/hooks because @/hooks/use-debounce
// is not yet present in metapi-go. When the shared @/hooks/use-debounce lands, the
// imports in toolbar/toolbar.tsx and ./use-debounced-column-filter.ts can switch back.
import * as React from 'react'

export function useDebounce<T>(value: T, delay: number): T {
  const [debouncedValue, setDebouncedValue] = React.useState<T>(value)

  React.useEffect(() => {
    const timer = setTimeout(() => setDebouncedValue(value), delay)
    return () => clearTimeout(timer)
  }, [value, delay])

  return debouncedValue
}
