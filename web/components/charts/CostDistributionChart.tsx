import React, { useEffect, useMemo, useState } from 'react';
import { tr } from '../../i18n.js';
import { VChart } from '@visactor/react-vchart';
import { api, type ModelCostDistributionResponse } from '../../api.js';
import { EmptyState as DsEmptyState } from '../../design-system/index.js';
import { useChartColors } from '../useThemeLabelColor.js';
import { prefersReducedMotion } from '../motion.js';

/* ------------------------------------------------------------------ */
/*  Props                                                              */
/* ------------------------------------------------------------------ */

interface CostDistributionChartProps {
  /** Lookback window in days. Default 30. */
  days?: number;
  /** How many models to show before folding the rest into "其他模型". */
  topN?: number;
}


/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

/**
 * CostDistributionChart — model cost share donut (all-api-hub borrow A2).
 * Top-N models by estimated cost with the remainder folded into an "Other"
 * bucket, mirroring all-api-hub UsageAnalytics' topN-with-Other pattern.
 */
export default function CostDistributionChart({
  days = 30,
  topN = 8,
}: CostDistributionChartProps) {
  const [data, setData] = useState<ModelCostDistributionResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const colors = useChartColors();

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    api
      .getModelCostDistribution(days, topN)
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
  }, [days, topN]);

  const items = useMemo(
    () => (Array.isArray(data?.items) ? data.items : []),
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

  if (error || items.length === 0) {
    return (
      <div style={containerStyle}>
        <div style={headerStyle}>
          <span style={titleStyle}>模型成本分布（近 {days} 天）</span>
        </div>
        <DsEmptyState
          className="dashboard-chart-empty"
          tone="neutral"
          icon="◇"
          title={error ? '加载失败' : '暂无模型成本数据'}
          description={error ? '请稍后重试' : '有代理请求后自动生成成本分布'}
        />
      </div>
    );
  }

  /* ---------- vchart spec ---------- */

  const totalCost = data?.totals?.cost ?? 0;
  const spec: Record<string, unknown> = {
    type: 'pie' as const,
    animation: !prefersReducedMotion(),
    data: [{ id: 'data', values: items }],
    valueField: 'cost',
    categoryField: 'label',
    innerRadius: 0.62,
    outerRadius: 0.85,
    padAngle: 0.6,
    cornerRadius: 3,
    label: { visible: false },
    legends: {
      visible: true,
      position: 'bottom',
      orient: 'bottom',
      layout: 'horizontal',
      item: {
        maxWidth: 160,
        label: { style: { fill: colors.axisLabel } },
      },
    },
    tooltip: {
      mark: {
        title: { value: (datum: Record<string, unknown>) => datum?.label ?? '' },
        content: [
          {
            key: tr('成本'),
            value: (datum: Record<string, unknown>) =>
              `$${Number(datum?.cost ?? 0).toFixed(4)}`,
          },
          {
            key: tr('请求'),
            value: (datum: Record<string, unknown>) => `${datum?.calls ?? 0}`,
          },
          {
            key: 'Token',
            value: (datum: Record<string, unknown>) =>
              Number(datum?.tokens ?? 0).toLocaleString(),
          },
          ...(totalCost > 0
            ? [
                {
                  key: tr('占比'),
                  value: (datum: Record<string, unknown>) =>
                    `${((Number(datum?.cost ?? 0) / totalCost) * 100).toFixed(1)}%`,
                },
              ]
            : []),
        ],
      },
    },
    color: colors.series,
    background: 'transparent',
    padding: { left: 8, right: 8, top: 8, bottom: 8 },
  };

  /* ---------- render ---------- */

  return (
    <div style={containerStyle}>
      <div style={headerStyle}>
        <span style={titleStyle}>模型成本分布（近 {days} 天）</span>
        <span style={totalStyle}>
          总成本 ${totalCost.toFixed(2)}
        </span>
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
