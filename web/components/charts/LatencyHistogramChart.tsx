import React, { useEffect, useMemo, useState } from 'react';
import { tr } from '../../i18n.js';
import { VChart } from '@visactor/react-vchart';
import { api, type LatencyHistogramResponse } from '../../api.js';
import { EmptyState as DsEmptyState } from '../../design-system/index.js';

/* ------------------------------------------------------------------ */
/*  Props                                                              */
/* ------------------------------------------------------------------ */

interface LatencyHistogramChartProps {
  /** Lookback window in days. Default 7. */
  days?: number;
  /** Bucket width in milliseconds. Default 500. */
  bucketMs?: number;
}

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

/**
 * LatencyHistogramChart — request-count distribution over latency buckets
 * (all-api-hub borrow A2). Shows where request latency concentrates so an
 * operator can judge whether the gateway or an upstream is the bottleneck.
 */
export default function LatencyHistogramChart({
  days = 7,
  bucketMs = 500,
}: LatencyHistogramChartProps) {
  const [data, setData] = useState<LatencyHistogramResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    api
      .getLatencyHistogram(days, bucketMs)
      .then((res) => {
        if (cancelled) return;
        setData(res);
      })
      .catch(() => {
        if (cancelled) return;
        setError(true);
        setData(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [days, bucketMs]);

  const buckets = useMemo(
    () => (Array.isArray(data?.buckets) ? data.buckets : []),
    [data],
  );

  /* ---------- loading ---------- */

  if (loading) {
    return (
      <div style={containerStyle}>
        <div style={headerStyle}>
          <div
            className="skeleton"
            style={{ width: 180, height: 24, borderRadius: 'var(--radius-sm)' }}
          />
        </div>
        <div
          className="skeleton"
          style={{ width: '100%', height: 280, borderRadius: 'var(--radius-sm)' }}
        />
      </div>
    );
  }

  /* ---------- error / empty ---------- */

  if (error || buckets.length === 0) {
    return (
      <div style={containerStyle}>
        <div style={headerStyle}>
          <span style={titleStyle}>延迟直方图（近 {days} 天）</span>
        </div>
        <DsEmptyState
          className="dashboard-chart-empty"
          tone="neutral"
          icon="◇"
          title={error ? '加载失败' : '暂无延迟数据'}
          description={error ? '请稍后重试' : '有代理请求后自动生成延迟分布'}
        />
      </div>
    );
  }

  /* ---------- vchart spec ---------- */

  const spec: Record<string, unknown> = {
    type: 'bar' as const,
    data: [{ id: 'data', values: buckets }],
    xField: 'label',
    yField: 'count',
    bar: { style: { cornerRadius: [4, 4, 0, 0] } },
    legends: { visible: false },
    tooltip: {
      mark: {
        title: { value: (datum: Record<string, unknown>) => datum?.label ?? '' },
        content: [
          {
            key: tr('请求数'),
            value: (datum: Record<string, unknown>) =>
              Number(datum?.count ?? 0).toLocaleString(),
          },
          {
            key: tr('占比'),
            value: (datum: Record<string, unknown>) =>
              `${Number(datum?.percent ?? 0).toFixed(1)}%`,
          },
        ],
      },
    },
    color: ['var(--color-chart-2)'],
    background: 'transparent',
    padding: { left: 8, right: 16, top: 8, bottom: 8 },
    axes: [
      {
        orient: 'bottom',
        label: {
          style: { fontSize: 10, fill: 'var(--color-text-muted)' },
          rotate: buckets.length > 8 ? -30 : 0,
        },
        domainLine: { style: { stroke: 'var(--color-border-light)' } },
        tick: { style: { stroke: 'var(--color-border-light)' } },
      },
      {
        orient: 'left',
        label: { style: { fontSize: 11, fill: 'var(--color-text-muted)' } },
        grid: { style: { stroke: 'var(--color-border-light)', lineDash: [4, 4] } },
        domainLine: { visible: false },
      },
    ],
  };

  /* ---------- render ---------- */

  return (
    <div style={containerStyle}>
      <div style={headerStyle}>
        <span style={titleStyle}>延迟直方图（近 {days} 天）</span>
        <span style={totalStyle}>共 {data?.total ?? 0} 次请求</span>
      </div>
      <div style={{ width: '100%', height: 280 }}>
        <VChart spec={spec as any} style={{ width: '100%', height: '100%' }} />
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Styles                                                             */
/* ------------------------------------------------------------------ */

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
  marginBottom: 16,
};

const titleStyle: React.CSSProperties = {
  fontSize: 14,
  fontWeight: 600,
  color: 'var(--color-text)',
};

const totalStyle: React.CSSProperties = {
  fontSize: 12,
  fontWeight: 500,
  color: 'var(--color-text-secondary)',
};
