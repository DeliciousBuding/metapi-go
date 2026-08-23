// metapi-go/features/settings/lib — legacy URL redirect map (wave 9 lane B).
//
// The semantic regroup renames three of the five subareas
// (general→basic, models→proxy-models, system-info→operations) and moves
// sections across subareas (general/scheduling→operations,
// general/proxy-transport+routing→proxy-models). Old URLs must keep working:
// bookmarks, docs, dashboard attention links and the search index all carry
// `/settings/<old-subarea>/<section>` paths. The route guards resolve these
// before the "unknown subarea / unknown section" fallbacks so a stale link
// lands on the exact new page instead of bouncing to a default section.
//
// Served URLs that did not change (downstream/*, content/*) need no entry.

/** Old `<subarea>/<section>` path → `[newSubarea, newSection]`. */
const LEGACY_SECTION_ROUTES: Readonly<
  Record<string, readonly [string, string]>
> = {
  'general/site': ['basic', 'site'],
  'general/authentication': ['basic', 'authentication'],
  'general/scheduling': ['operations', 'scheduling'],
  'general/proxy-transport': ['proxy-models', 'proxy-transport'],
  'general/routing': ['proxy-models', 'routing'],
  'models/redirects': ['proxy-models', 'redirects'],
  'models/rates': ['proxy-models', 'rates'],
  'models/allowlist': ['proxy-models', 'allowlist'],
  'models/catalog-sources': ['proxy-models', 'catalog-sources'],
  'system-info/program-logs': ['operations', 'program-logs'],
  'system-info/audit-logs': ['operations', 'audit-logs'],
  'system-info/update-center': ['operations', 'update-center'],
  'system-info/database': ['operations', 'database'],
  'system-info/data-migration': ['operations', 'data-migration'],
  'system-info/maintenance': ['operations', 'maintenance'],
  'system-info/danger-zone': ['operations', 'danger-zone'],
}

/** Old bare subarea paths (`/settings/<id>`) → new subarea id. */
const LEGACY_SUBAREA_REDIRECTS: Readonly<Record<string, string>> = {
  general: 'basic',
  models: 'proxy-models',
  'system-info': 'operations',
}

/**
 * Resolve a full legacy `/settings/<subarea>/<section>` URL.
 * @returns `[newSubarea, newSection]` or `undefined` when the pair is not a
 * known legacy route.
 */
export function resolveLegacySectionRedirect(
  subarea: string,
  section: string
): readonly [string, string] | undefined {
  return LEGACY_SECTION_ROUTES[`${subarea}/${section}`]
}

/**
 * A subarea id that was replaced by the regroup (bare `/settings/<id>` URLs
 * need redirecting before the unknown-id fallback kicks in).
 */
export function isLegacySubarea(subarea: string): boolean {
  return subarea in LEGACY_SUBAREA_REDIRECTS
}

/** New subarea id for a legacy bare subarea URL, or `undefined`. */
export function resolveLegacySubareaRedirect(
  subarea: string
): string | undefined {
  return LEGACY_SUBAREA_REDIRECTS[subarea]
}
