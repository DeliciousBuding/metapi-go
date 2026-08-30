// metapi-go/lib — shared event-title → i18n slug map.
//
// Backend event titles are persisted English strings (events.title — the row
// is written once at emission time and never re-localized server-side), and
// TWO surfaces translate them at render time: the program-logs page and the
// attention pipeline (bell popover + availability panel). They share this
// single map so a newly introduced producer title is mapped once, not twice.
//
// Unknown titles fall through to the raw label at the call site (honest
// residual, never a half-translation). Dynamic titles (e.g. "Site
// announcement: <site name>" — upstream-pushed foreign data) are
// intentionally NOT mapped.
//
// The slug resolves under the shared `events.titles.*` locale section.

/** All known persisted producer titles → stable slug. */
const EVENT_TITLE_SLUGS: Record<string, string> = {
  'All proxies failed': 'allProxiesFailed',
  'Low balance': 'lowBalance',
  'Token expired': 'tokenExpired',
  'Site disabled': 'siteDisabled',
  'Site enabled': 'siteEnabled',
  'checkin success': 'checkinSuccess',
  'checkin failed': 'checkinFailed',
  'checkin failed (cloudflare challenge)': 'checkinFailedCloudflare',
  'checkin skipped': 'checkinSkipped',
  'account token sync failed': 'tokenSyncFailed',
  'Account token sync completed': 'tokenSyncCompleted',
  'Admin login token updated': 'adminTokenUpdated',
  'Runtime settings updated': 'runtimeSettingsUpdated',
  'Disabled model repaired (mapping auto-restored)': 'disabledModelRepaired',
  'Backup import added downstream API keys': 'backupImportAddedKeys',
}

/**
 * Slug for a persisted event title, or undefined for unknown/dynamic titles
 * (caller renders the raw label).
 *
 * Call sites must use a literal template key — `t(`events.titles.${slug}`)` —
 * so the i18n key-coverage gate statically verifies the `events.titles`
 * prefix (a function-built key would be invisible to the scanner).
 */
export function eventTitleSlug(title: string): string | undefined {
  return EVENT_TITLE_SLUGS[title]
}
