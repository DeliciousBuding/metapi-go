import React, { useEffect, useState } from 'react';
import { api, type Announcement } from '../api.js';
import { tr } from '../i18n.js';

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

const SEVERITY_TONE: Record<
  Announcement['severity'],
  { bg: string; border: string; icon: string }
> = {
  critical: {
    bg: 'color-mix(in srgb, var(--color-danger) 12%, transparent)',
    border: 'var(--color-danger)',
    icon: '⛔',
  },
  warning: {
    bg: 'color-mix(in srgb, var(--color-warning) 12%, transparent)',
    border: 'var(--color-warning)',
    icon: '⚠',
  },
  info: {
    bg: 'color-mix(in srgb, var(--color-info) 10%, transparent)',
    border: 'var(--color-info)',
    icon: 'ℹ',
  },
};

/**
 * AnnouncementBanner — severity-ranked product risk banners (all-api-hub
 * borrow H1). Fetches active (enabled + not dismissed) announcements and
 * renders them above the dashboard content; dismissing hides until the next
 * content revision.
 */
export default function AnnouncementBanner() {
  const [items, setItems] = useState<Announcement[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    api
      .getActiveAnnouncements()
      .then((res) => {
        if (cancelled) return;
        setItems(Array.isArray(res?.items) ? res.items : []);
      })
      .catch(() => {
        // Banners are non-critical; failures degrade silently.
        if (!cancelled) setItems([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (loading || items.length === 0) return null;

  const dismiss = async (id: number) => {
    try {
      await api.dismissAnnouncement(id);
    } catch {
      // Keep the banner visible on failure.
      return;
    }
    setItems((current) => current.filter((item) => item.id !== id));
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 16 }}>
      {items.map((item) => {
        const tone = SEVERITY_TONE[item.severity] ?? SEVERITY_TONE.info;
        return (
          <div
            key={item.id}
            role="alert"
            className="chart-panel-enter animate-slide-up"
            style={{
              display: 'flex',
              alignItems: 'flex-start',
              gap: 10,
              padding: '10px 14px',
              borderRadius: 'var(--radius-md)',
              background: tone.bg,
              border: `1px solid ${tone.border}`,
            }}
          >
            <span style={{ fontSize: 14, lineHeight: '20px' }}>{tone.icon}</span>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-text)' }}>
                {item.title}
              </div>
              <div style={{ fontSize: 12, color: 'var(--color-text-secondary)', marginTop: 2 }}>
                {item.message}
                {item.link ? (
                  <>
                    {' '}
                    <a
                      href={item.link}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{ color: 'var(--color-primary)', textDecoration: 'underline' }}
                    >
                      {tr('详情')}
                    </a>
                  </>
                ) : null}
              </div>
            </div>
            <button
              type="button"
              aria-label={tr('关闭公告')}
              title={tr('关闭')}
              onClick={() => void dismiss(item.id)}
              style={{
                border: 'none',
                background: 'none',
                cursor: 'pointer',
                color: 'var(--color-text-muted)',
                fontSize: 16,
                lineHeight: 1,
                padding: '2px 4px',
              }}
            >
              ×
            </button>
          </div>
        );
      })}
    </div>
  );
}
