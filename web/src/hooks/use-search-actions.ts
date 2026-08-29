// metapi-go/hooks — ⌘K palette action adapter (S5 boundary inversion).
//
// The authenticated app shell must be able to fire high-frequency write
// operations from the command palette without components/layout importing
// feature modules (components ↛ features in docs/internal/web-package-boundaries.md).
// This hook is the feature-facing action adapter: it lives outside the
// restricted layers, wires the existing page mutation hooks into one surface,
// and is consumed by SearchModal.
//
// No business logic is created here; every action reuses the same mutation
// hooks and affordances the pages already expose (#1035 S6).

import { useManualCheckin } from '@/features/checkin'
import {
  useRebuildRoutes,
  useRefreshRouteDecisions,
} from '@/features/token-routes'

export function useSearchActions() {
  return {
    triggerAllCheckin: useManualCheckin(),
    rebuildRoutes: useRebuildRoutes(),
    refreshRouteDecisions: useRefreshRouteDecisions(),
  }
}
