// metapi-go/lib/helpers — document title resolution for route `staticData`.
//
// `staticData.title` (registered via the StaticDataRouteOption augmentation
// in main.tsx) is either a static i18n key, a list of i18n keys joined with
// " · ", or a resolver over the route params for param-driven routes
// (`/dashboard/$section`, `/settings/$subarea/$section`). This helper
// normalizes all three shapes into a flat key list consumed by the root
// `useDocumentTitle` hook.

/**
 * The `staticData.title` contract: one i18n key, a list of i18n keys joined
 * with " · ", or a resolver over the route params (for `$section` /
 * `$subarea/$section` routes) returning either shape. Resolvers may return
 * `undefined` to fall back to the bare product name.
 */
export type RouteTitleSpec =
  | string
  | readonly string[]
  | ((
      params: Record<string, string>
    ) => string | readonly string[] | undefined)

/**
 * Normalize a `staticData.title` spec into a flat i18n key list for the
 * given route params. Unknown/invalid resolver results yield an empty list
 * (caller falls back to the bare product name).
 */
export function resolveDocumentTitleKeys(
  title: RouteTitleSpec | undefined,
  params: Record<string, string>
): readonly string[] {
  if (!title) return []
  const resolved = typeof title === 'function' ? title(params) : title
  if (!resolved) return []
  return typeof resolved === 'string' ? [resolved] : resolved
}
