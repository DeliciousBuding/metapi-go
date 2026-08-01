import React, { useEffect, useState } from 'react';
import { tr } from '../i18n.js';
import { api, type SchedulerRunStatus } from '../api.js';
import { EmptyState as DsEmptyState } from '../design-system/index.js';

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

/**
 * SchedulerStatusPanel — unified recurring-scheduler run history
 * (all-api-hub borrow C1). Shows each automation's last run, 24h activity,
 * and success rate so operators see at a glance which jobs are healthy.
 */
export default function SchedulerStatusPanel() {
  const [items, setItems] = useState<SchedulerRunStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    api
      .getSchedulerStatus()
      .then((res) => {
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
  }, []);

  if (loading) {
    return (
      <div style={containerStyle}>
        <div style={headerStyle}>
          <span style={titleStyle}>{tr('调度任务状态')}</span>
        </div>
        <div
          className="skeleton"
          style={{ width: '100%', height: 160, borderRadius: 'var(--radius-sm)' }}
        />
      </div>
    );
  }

  if (error || items.length === 0) {
    return (
      <div style={containerStyle}>
        <div style={headerStyle}>
          <span style={titleStyle}>{tr('调度任务状态')}</span>
        </div>
        <DsEmptyState
          className="dashboard-chart-empty"
          tone={error ? 'danger' : 'neutral'}
          icon={error ? '!' : '◇'}
          title={error ? '加载失败' : '暂无调度任务'}
          description={error ? '请稍后重试' : '启动调度器后将展示各任务运行历史'}
        />
      </div>
    );
  }

  return (
    <div style={containerStyle}>
      <div style={headerStyle}>
        <span style={titleStyle}>{tr('调度任务状态')}</span>
      </div>
      <ul style={listStyle}>
        {items.map((item) => (
          <li key={item.job} style={itemStyle}>
            <span style={dotStyle(statusColor(item))} aria-hidden />
            <span style={jobStyle}>{item.job}</span>
            <span style={metaStyle}>
              {formatLastRun(item)}
              {item.runs24h > 0 && ` · 24h ${item.runs24h} 次`}
              {item.success24h > 0 && item.runs24h > 0 && item.success24h < item.runs24h
                ? `（成功 ${item.success24h}）`
                : ''}
              {item.note ? ` · ${item.note}` : ''}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Helpers + styles                                                   */
/* ------------------------------------------------------------------ */

function statusColor(item: SchedulerRunStatus): string {
  if (!item.enabled) return 'var(--color-text-muted, #9ca3af)';
  switch (item.lastStatus) {
    case 'failed':
      return 'var(--color-danger, #dc2626)';
    case 'running':
      return 'var(--color-warning, #d97706)';
    case 'success':
      return 'var(--color-success, #16a34a)';
    default:
      return 'var(--color-info, #2563eb)';
  }
}

function formatLastRun(item: SchedulerRunStatus): string {
  if (!item.lastRunAt) return item.enabled ? '从未运行' : '未启用';
  const t = new Date(item.lastRunAt);
  if (Number.isNaN(t.getTime())) return item.lastRunAt;
  const now = Date.now();
  const diffMs = now - t.getTime();
  const mins = Math.floor(diffMs / 60_000);
  if (mins < 1) return '刚刚';
  if (mins < 60) return `${mins} 分钟前`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours} 小时前`;
  const days = Math.floor(hours / 24);
  return `${days} 天前`;
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
  padding: '6px 10px',
  borderRadius: 'var(--radius-sm)',
};

const dotStyle = (color: string): React.CSSProperties => ({
  width: 8,
  height: 8,
  borderRadius: '50%',
  background: color,
  flexShrink: 0,
});

const jobStyle: React.CSSProperties = {
  fontSize: 13,
  fontWeight: 500,
  color: 'var(--color-text)',
  minWidth: 130,
};

const metaStyle: React.CSSProperties = {
  fontSize: 12,
  color: 'var(--color-text-muted)',
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
};
