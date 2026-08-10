import React, { useEffect, useState } from 'react';
import { tr } from '../i18n.js';
import { useNavigate } from 'react-router-dom';
import { api } from '../api.js';
import { EmptyState as DsEmptyState } from '../design-system/index.js';

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface AttentionItem {
  severity: string; // critical | warning | info
  category: string; // expired_account | low_balance | disabled_site | event
  label: string;
  target: string; // deep-link route + query
  createdAt: string;
}

interface AttentionResponse {
  items: AttentionItem[];
  total: number;
}

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

/**
 * AttentionPanel — severity-ranked actionable items.
 * Shows the operator "what needs my eyes": expired accounts, low-balance
 * accounts, disabled sites, recent warning/error events. Each item is a
 * deep link to the exact page so a click jumps straight to the problem.
 */
export default function AttentionPanel({ limit = 10 }: { limit?: number }) {
  const [items, setItems] = useState<AttentionItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    api
      .getAttention(limit)
      .then((res: AttentionResponse) => {
        if (cancelled) return;
        setItems(Array.isArray(res?.items) ? res.items : []);
      })
      .catch(() => {
        if (cancelled) return;
        setError(true);
        setItems([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [limit]);

  if (loading) {
    return (
      <div style={containerStyle}>
        <div style={headerStyle}>
          <span style={titleStyle}>{tr('需要关注')}</span>
        </div>
        <div
          className="skeleton"
          style={{ width: '100%', height: 120, borderRadius: 'var(--radius-sm)' }}
        />
      </div>
    );
  }

  if (error || items.length === 0) {
    return (
      <div style={containerStyle}>
        <div style={headerStyle}>
          <span style={titleStyle}>{tr('需要关注')}</span>
        </div>
        <DsEmptyState
          className="dashboard-chart-empty"
          tone={error ? 'danger' : 'neutral'}
          icon={error ? '!' : '✓'}
          title={error ? '加载失败' : '一切正常'}
          description={error ? '请稍后重试' : '当前没有需要关注的异常项'}
        />
      </div>
    );
  }

  return (
    <div style={containerStyle}>
      <div style={headerStyle}>
        <span style={titleStyle}>{tr('需要关注')}</span>
        <span style={countStyle}>{items.length}</span>
      </div>
      <ul style={listStyle}>
        {items.map((item, i) => (
          <li
            key={`${item.category}-${i}`}
            style={itemStyle}
            onClick={() => navigate(item.target)}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                navigate(item.target);
              }
            }}
          >
            <span style={dotStyle(severityColor(item.severity))} aria-hidden />
            <span style={labelStyle}>{item.label}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Helpers + styles                                                   */
/* ------------------------------------------------------------------ */

function severityColor(severity: string): string {
  switch (severity) {
    case 'critical':
      return 'var(--color-danger, #dc2626)';
    case 'warning':
      return 'var(--color-warning, #d97706)';
    default:
      return 'var(--color-info, #2563eb)';
  }
}

const containerStyle: React.CSSProperties = {
  background: 'var(--color-bg-card)',
  borderRadius: 'var(--radius-md)',
  border: '1px solid var(--color-border-light)',
  boxShadow: 'var(--shadow-card)',
  padding: 20,
};

const headerStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  marginBottom: 12,
};

const titleStyle: React.CSSProperties = {
  fontSize: 14,
  fontWeight: 600,
  color: 'var(--color-text)',
};

const countStyle: React.CSSProperties = {
  fontSize: 12,
  fontWeight: 600,
  color: 'var(--color-text-muted)',
  background: 'var(--color-bg-muted)',
  borderRadius: 'var(--radius-full, 9999px)',
  padding: '2px 8px',
};

const listStyle: React.CSSProperties = {
  listStyle: 'none',
  margin: 0,
  padding: 0,
  display: 'flex',
  flexDirection: 'column',
  gap: 8,
};

const itemStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 10,
  padding: '8px 10px',
  borderRadius: 'var(--radius-sm)',
  cursor: 'pointer',
  transition: 'background 0.15s',
};

const dotStyle = (color: string): React.CSSProperties => ({
  width: 8,
  height: 8,
  borderRadius: '50%',
  background: color,
  flexShrink: 0,
});

const labelStyle: React.CSSProperties = {
  fontSize: 13,
  color: 'var(--color-text)',
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
};
