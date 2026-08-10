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

interface IncomeOutcomePoint {
  day: string;
  income: number;
  outcome: number;
  net: number;
}

interface IncomeOutcomeResponse {
  days: number;
  points: IncomeOutcomePoint[];
  summary: {
    totalIncome: number;
    totalOutcome: number;
    net: number;
    accounts: number;
  };
}

interface IncomeOutcomeChartProps {
  /** Lookback window in days. Default 30. */
  days?: number;
}

const containerStyle: React.CSSProperties = { width: '100%' };
const headerStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  marginBottom: 12,
  flexWrap: 'wrap',
  gap: 8,
};
const titleStyle: React.CSSProperties = { fontWeight: 600, fontSize: 14 };
const mutedStyle: React.CSSProperties = { fontSize: 12, color: 'var(--color-text-muted)' };

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

/**
 * IncomeOutcomeChart — balance inflow vs consumption.
 * Derived from the A1 daily snapshots via the accounting identity
 * income - outcome = Δbalance; grouped bars show free-quota/recharge inflow
 * against chargeable spend, so an operator sees whether balance is being
 * replenished faster than it burns.
 */
export default function IncomeOutcomeChart({ days = 30 }: IncomeOutcomeChartProps) {
  const [points, setPoints] = useState<IncomeOutcomePoint[]>([]);
  const [summary, setSummary] = useState<IncomeOutcomeResponse['summary'] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const colors = useChartColors();

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    api
      .getBalanceIncomeOutcome(days)
      .then((res: IncomeOutcomeResponse) => {
        if (cancelled) return;
        setPoints(Array.isArray(res?.points) ? res.points : []);
        setSummary(res?.summary ?? null);
      })
      .catch(() => {
        if (cancelled) return;
        setError(true);
        setPoints([]);
        setSummary(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [days]);

  /* ---------- data transform: long format for grouped bars ---------- */

  const flatData = useMemo(() => {
    const out: Array<{ day: string; type: string; value: number }> = [];
    for (const p of points) {
      out.push({ day: p.day, type: tr('收入'), value: p.income });
      out.push({ day: p.day, type: tr('消费'), value: p.outcome });
    }
    return out;
  }, [points]);

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
          <span style={titleStyle}>余额流入 vs 消费（近 {days} 天）</span>
        </div>
        <DsEmptyState
          className="dashboard-chart-empty"
          tone="neutral"
          icon="◇"
          title={error ? '加载失败' : '暂无余额历史'}
          description={error ? '请稍后重试' : '刷新账号余额后将自动记录每日快照并展示分析'}
        />
      </div>
    );
  }

  /* ---------- vchart spec: grouped bars ---------- */

  const spec: Record<string, unknown> = {
    type: 'bar' as const,
    data: [{ id: 'data', values: flatData }],
    xField: 'day',
    yField: 'value',
    seriesField: 'type',
    stack: false,
    bar: { style: { maxWidth: 14 } },
    legends: { visible: true, orient: 'top',
      item: { label: { style: { fill: colors.axisLabel } } } },
    tooltip: {
      mark: {
        title: { value: (datum: Record<string, unknown>) => datum?.day ?? '' },
        content: [
          {
            key: (datum: Record<string, unknown>) => String(datum?.type ?? ''),
            value: (datum: Record<string, unknown>) =>
              `$${Number(datum?.value ?? 0).toFixed(2)}`,
          },
        ],
      },
    },
    animation: !prefersReducedMotion(),
    animationAppear: {
      bar: { type: 'growHeightIn', duration: 800, easing: 'cubicOut' },
    },
    axes: [
      {
        orient: 'bottom',
        label: { style: { fontSize: 11, fill: colors.axisLabel } },
        tick: { visible: false },
        domainLine: { style: { stroke: colors.grid } },
      },
      {
        orient: 'left',
        label: { style: { fontSize: 11, fill: colors.axisLabel } },
        domainLine: { visible: false },
        grid: { style: { stroke: colors.grid } },
        tick: { visible: false },
      },
    ],
  };

  return (
    <div style={containerStyle}>
      <div style={headerStyle}>
        <span style={titleStyle}>余额流入 vs 消费（近 {days} 天）</span>
        {summary && (
          <span style={mutedStyle}>
            收入{' '}
            <b style={{ color: 'var(--color-text-primary)' }}>${summary.totalIncome.toFixed(2)}</b>
            {' · '}消费{' '}
            <b style={{ color: 'var(--color-text-primary)' }}>${summary.totalOutcome.toFixed(2)}</b>
            {' · '}净{' '}
            <b
              style={{
                color:
                  summary.net >= 0 ? 'var(--color-success, #34a853)' : 'var(--color-danger, #ea4335)',
              }}
            >
              ${summary.net.toFixed(2)}
            </b>
          </span>
        )}
      </div>
      <VChart spec={spec as any} style={{ width: '100%', height: '100%' }} />
    </div>
  );
}
