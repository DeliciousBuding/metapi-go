// metapi-go/features/dashboard/lib — re-export of the shared display formatters.
// The canonical implementation lives in `@/lib/format`; this barrel preserves
// the existing `@/features/dashboard/lib/format` import path for dashboard
// sections while centralizing the formatting vocabulary.

export { formatInt, formatRatio, formatPercent } from '@/lib/format'