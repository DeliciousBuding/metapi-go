import React, { useEffect, useMemo, useState } from 'react';
import { tr } from '../../i18n.js';
import { VChart } from '@visactor/react-vchart';
import { api } from '../../api.js';
import { EmptyState as DsEmptyState } from '../../design-system/index.js';
import { useChartColors } from '../useThemeLabelColor.js';
import { prefersReducedMotion } from '../motion.js';

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface BalancePoint {
  day: string;
  balance: number;
  balanceUsed: number;
  quota: number;
  capturedAt: string;
}

interface BalanceSeries {
  accountId: number;
  points: BalancePoint[];
}

interface BalanceHistoryResponse {
  series: BalanceSeries[];
  days: number;
}

interface BalanceHistoryChartProps {
  /** Account to show. 0 (default) = aggregate across all accounts. */
  accountId?: number;
  /** Lookback window in days. Default 30. */
  days?: number;
}

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

/**
 * BalanceHistoryChart — daily balance trend (all-api-hub borrow A1).
 * Aggregates per-day total balance across accounts (sum) so a gateway
 * operator sees liquidity over time. Per-account drill-down via accountId.
 */
export default function BalanceHistoryChart({
  accountId = 0,
  days = 30,
}: BalanceHistoryChartProps) {
  const [series, setSeries] = useState<BalanceSeries[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const colors = useChartColors();

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    api
      .getBalanceHistory(accountId, days)
      .then((res: BalanceHistoryResponse) => {
        if (cancelled) return;
        setSeries(Array.isArray(res?.series) ? res.series : []);
      })
      .catch(() => {
        if (cancelled) return;
        setError(true);
        setSeries([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [accountId, days]);

  /* ---------- data transform: aggregate balance per day ---------- */

  const flatData = useMemo(() => {
    if (series.length === 0) return [];
    const byDay = new Map<string, number>();
    for (const s of series) {
      for (const p of s.points) {
        byDay.set(p.day, (byDay.get(p.day) ?? 0) + (p.balance ?? 0));
      }
    }
    return Array.from(byDay.entries())
      .map(([day, balance]) => ({ day, balance }))
      .sort((a, b) => (a.day < b.day ? -1 : a.day > b.day ? 1 : 0));
  }, [series]);

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
          <span style={titleStyle}>余额趋势（近 {days} 天）</span>
        </div>
        <DsEmptyState
          className="dashboard-chart-empty"
          tone="neutral"
          icon="◇"
          title={error ? '加载失败' : '暂无余额历史'}
          description={
            error
              ? '请稍后重试'
              : '刷新账号余额后将自动记录每日快照并展示趋势'
          }
        />
      </div>
    );
  }

  /* ---------- vchart spec ---------- */

  const spec: Record<string, unknown> = {
    type: 'line' as const,
    data: [{ id: 'data', values: flatData }],
    xField: 'day',
    yField: 'balance',
    point: { visible: true, style: { size: 5 } },
    line: { style: { lineWidth: 2, curveType: 'monotone' } },
    legends: { visible: false },
    tooltip: {
      mark: {
        title: { value: (datum: Record<string, unknown>) => datum?.day ?? '' },
        content: [
          {
            key: tr('余额'),
            value: (datum: Record<string, unknown>) =>
              `$${Number(datum?.balance ?? 0).toFixed(2)}`,
          },
        ],
      },
    },
    animation: !prefersReducedMotion(),
    animationAppear: {
      line: { type: 'clipIn', duration: 800, easing: 'cubicOut' },
      point: { type: 'fadeIn', duration: 600, delay: 400, easing: 'cubicOut' },
    },
    axes: [
      {
        orient: 'bottom',
        label: { style: { fontSize: 11, fill: colors.axisLabel } },
        domainLine: { style: { stroke: colors.grid } },
        tick: { style: { stroke: colors.grid } },
      },
      {
        orient: 'left',
        label: { style: { fontSize: 11, fill: colors.axisLabel } },
        grid: { style: { stroke: colors.grid, lineDash: [4, 4] } },
        domainLine: { visible: false },
      },
    ],
    color: ['var(--color-chart-1)'],
    background: 'transparent',
    padding: { left: 8, right: 16, top: 8, bottom: 8 },
  };

  /* ---------- render ---------- */

  return (
    <div style={containerStyle}>
      <div style={headerStyle}>
        <span style={titleStyle}>余额趋势（近 {days} 天）</span>
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
