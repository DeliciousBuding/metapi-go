import React, { useEffect, useMemo, useState } from 'react';
import { tr } from '../../i18n.js';
import { VChart } from '@visactor/react-vchart';
import { api, type LatencyTrendResponse } from '../../api.js';
import { EmptyState as DsEmptyState } from '../../design-system/index.js';
import { useChartColors } from '../useThemeLabelColor.js';
import { prefersReducedMotion } from '../motion.js';

/* ------------------------------------------------------------------ */
/*  Props                                                              */
/* ------------------------------------------------------------------ */

interface LatencyTrendChartProps {
  /** Lookback window in days. Default 7. */
  days?: number;
}

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

/**
 * LatencyTrendChart — per-day average + P95 latency lines (all-api-hub
 * borrow A2). A widening avg/P95 gap signals long-tail slow requests;
 * truncatedDays (from the backend's bounded p95 sample) are surfaced as a
 * small note instead of being hidden.
 */
export default function LatencyTrendChart({ days = 7 }: LatencyTrendChartProps) {
  const [data, setData] = useState<LatencyTrendResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const colors = useChartColors();

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    api
      .getLatencyTrend(days)
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
  }, [days]);

  const flatData = useMemo(() => {
    const points = Array.isArray(data?.points) ? data.points : [];
    return points.flatMap((p) => {
      const rows: Array<{ date: string; metric: string; latency: number }> = [];
      if (p.avgLatencyMs != null) {
        rows.push({ date: p.date, metric: tr('平均延迟'), latency: p.avgLatencyMs });
      }
      if (p.p95LatencyMs != null) {
        rows.push({ date: p.date, metric: 'P95', latency: p.p95LatencyMs });
      }
      return rows;
    });
  }, [data]);

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

  if (error || flatData.length === 0) {
    return (
      <div style={containerStyle}>
        <div style={headerStyle}>
          <span style={titleStyle}>延迟趋势（近 {days} 天）</span>
        </div>
        <DsEmptyState
          className="dashboard-chart-empty"
          tone="neutral"
          icon="◇"
          title={error ? '加载失败' : '暂无延迟数据'}
          description={error ? '请稍后重试' : '有代理请求后自动生成延迟趋势'}
        />
      </div>
    );
  }

  /* ---------- vchart spec ---------- */

  const truncatedCount = data?.truncatedDays?.length ?? 0;
  const spec: Record<string, unknown> = {
    type: 'line' as const,
    animation: !prefersReducedMotion(),
    data: [{ id: 'data', values: flatData }],
    xField: 'date',
    yField: 'latency',
    seriesField: 'metric',
    point: { visible: false },
    line: { style: { lineWidth: 2, curveType: 'monotone' } },
    legends: { visible: true, position: 'bottom', orient: 'bottom',
      item: { label: { style: { fill: colors.axisLabel } } } },
    tooltip: {
      mark: {
        title: { value: (datum: Record<string, unknown>) => datum?.date ?? '' },
        content: [
          {
            key: tr('延迟'),
            value: (datum: Record<string, unknown>) =>
              `${Number(datum?.latency ?? 0).toFixed(0)} ms`,
          },
        ],
      },
    },
    color: [colors.series[0], colors.series[2]],
    background: 'transparent',
    padding: { left: 8, right: 16, top: 8, bottom: 8 },
    axes: [
      {
        orient: 'bottom',
        label: { style: { fontSize: 11, fill: colors.axisLabel } },
        domainLine: { style: { stroke: colors.grid } },
        tick: { style: { stroke: colors.grid } },
      },
      {
        orient: 'left',
        label: {
          style: { fontSize: 11, fill: colors.axisLabel },
          formatMethod: (value: unknown) => `${value} ms`,
        },
        grid: { style: { stroke: colors.grid, lineDash: [4, 4] } },
        domainLine: { visible: false },
      },
    ],
  };

  /* ---------- render ---------- */

  return (
    <div style={containerStyle}>
      <div style={headerStyle}>
        <span style={titleStyle}>延迟趋势（近 {days} 天）</span>
        {truncatedCount > 0 && (
          <span style={noteStyle} title="延迟样本量过大，P95 基于有界采样估算">
            P95 采样截断（{truncatedCount} 天）
          </span>
        )}
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

const noteStyle: React.CSSProperties = {
  fontSize: 11,
  fontWeight: 500,
  color: 'var(--color-warning)',
};
