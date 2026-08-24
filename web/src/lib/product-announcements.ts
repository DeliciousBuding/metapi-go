// Shared query keys and URL safety for product risk-banner announcements.
// Both the Settings CRUD surface and the Dashboard renderer must use these
// helpers so cache invalidation and link policy cannot drift apart.

export const productAnnouncementKeys = {
  all: ['product-announcements'] as const,
  list: () => [...productAnnouncementKeys.all, 'list'] as const,
  active: () => [...productAnnouncementKeys.all, 'active'] as const,
}

const ABSOLUTE_HTTP_URL = /^https?:\/\//i

export function getSafeProductAnnouncementUrl(
  value: string | null | undefined
): string | null {
  const candidate = value?.trim()
  if (!candidate || !ABSOLUTE_HTTP_URL.test(candidate)) return null

  try {
    const url = new URL(candidate)
    if (
      (url.protocol !== 'http:' && url.protocol !== 'https:') ||
      !url.hostname
    ) {
      return null
    }
    return candidate
  } catch {
    return null
  }
}

export function isValidProductAnnouncementLink(value: string): boolean {
  return !value.trim() || getSafeProductAnnouncementUrl(value) !== null
}
