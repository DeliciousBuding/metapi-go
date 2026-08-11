function parseServerUtcDate(value: string | null | undefined): { date: Date | null; raw: string } {
  if (!value) return { date: null, raw: '' };

  const raw = String(value).trim();
  if (!raw) return { date: null, raw };

  if (/^\d{10,13}$/.test(raw)) {
    const asNumber = Number(raw);
    if (Number.isFinite(asNumber)) {
      const epochMs = raw.length === 13 ? asNumber : asNumber * 1000;
      const date = new Date(epochMs);
      if (!Number.isNaN(date.getTime())) {
        return { date, raw };
      }
    }
  }

  let normalized = raw;
  if (!normalized.includes('T') && normalized.includes(' ')) {
    normalized = normalized.replace(' ', 'T');
  }
  normalized = normalized.replace(/([+-]\d{2})(\d{2})$/, '$1:$2');
  normalized = normalized.replace(/([+-]\d{2})$/, '$1:00');

  const hasTimeZone = /[zZ]$|[+-]\d{2}:\d{2}$/.test(normalized);
  if (!hasTimeZone) {
    normalized = `${normalized}Z`;
  }

  const date = new Date(normalized);
  if (Number.isNaN(date.getTime())) {
    return { date: null, raw };
  }

  return { date, raw };
}


function formatWithParts(
  value: string | null | undefined,
  options: Intl.DateTimeFormatOptions,
  locale = 'zh-CN',
  timeZone?: string,
): string {
  const { date, raw } = parseServerUtcDate(value);
  if (!date) return raw || '-';

  return new Intl.DateTimeFormat(locale, {
    ...options,
    ...(timeZone ? { timeZone } : {}),
  }).format(date);
}

export function formatDateTimeLocal(
  value: string | null | undefined,
  locale = 'zh-CN',
  timeZone?: string,
): string {
  return formatWithParts(value, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }, locale, timeZone);
}

export function formatDateTimeMinuteLocal(
  value: string | null | undefined,
  locale = 'zh-CN',
  timeZone?: string,
): string {
  return formatWithParts(value, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }, locale, timeZone);
}

export function formatDateLocal(
  value: string | null | undefined,
  locale = 'zh-CN',
  timeZone?: string,
): string {
  return formatWithParts(value, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }, locale, timeZone);
}

export function formatCheckinLogTime(
  value: string | null | undefined,
  locale?: string,
  timeZone?: string,
): string {
  return formatDateTimeLocal(value, locale, timeZone);
}

/**
 * Format a Date as the local `datetime-local` input value (`YYYY-MM-DDTHH:mm`),
 * the format ProxyLogs `from`/`to` fields expect. Used by date-range presets.
 */
export function toLocalDatetimeInputValue(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}
