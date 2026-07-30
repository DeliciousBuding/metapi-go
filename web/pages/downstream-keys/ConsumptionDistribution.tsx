import { useMemo, useState } from 'react';
import { tr } from '../../i18n.js';

export type ConsumptionItem = {
  id: number;
  name: string;
  usedCost: number;
  usedRequests: number;
};

type Metric = 'cost' | 'requests';

const LABEL: Record<Metric, string> = { cost: '已用费用', requests: '已用请求' };

function formatValue(metric: Metric, v: number): string {
  if (metric === 'cost') return `$${v.toFixed(2)}`;
  return String(v);
}

/**
 * Cross-key consumption distribution (N5). Aggregates the visible downstream
 * keys by usedCost / usedRequests and renders a top-10 horizontal bar list
 * with % share. Pure frontend aggregation over already-loaded summary data —
 * no new backend endpoint (the per-key trend endpoint is time-bounded and
 * single-key; this is the cumulative cross-key view the audit asked for).
 */
export function ConsumptionDistribution({ items }: { items: ConsumptionItem[] }) {
  const [metric, setMetric] = useState<Metric>('cost');

  const ranked = useMemo(() => {
    const field: (i: ConsumptionItem) => number = metric === 'cost' ? (i) => i.usedCost : (i) => i.usedRequests;
    const total = items.reduce((sum, i) => sum + Math.max(0, field(i)), 0);
    const top = [...items]
      .map((i) => ({ id: i.id, name: i.name, value: Math.max(0, field(i)) }))
      .filter((x) => x.value > 0)
      .sort((a, b) => b.value - a.value)
      .slice(0, 10);
    const max = top.length ? top[0].value : 0;
    return { total, top, max };
  }, [items, metric]);

  return (
    <div className="consumption-distribution" role="region" aria-label={tr('消费分布')}>
      <div className="consumption-distribution-header">
        <span className="consumption-distribution-title">{tr('消费分布')}</span>
        <div className="consumption-distribution-toggle" role="tablist">
          {(['cost', 'requests'] as const).map((m) => (
            <button
              key={m}
              type="button"
              role="tab"
              aria-selected={metric === m}
              className={`consumption-distribution-tab${metric === m ? ' is-active' : ''}`}
              onClick={() => setMetric(m)}
            >
              {tr(LABEL[m])}
            </button>
          ))}
        </div>
      </div>
      {ranked.top.length === 0 ? (
        <div className="consumption-distribution-empty">{tr('暂无消费数据')}</div>
      ) : (
        <ul className="consumption-distribution-list">
          {ranked.top.map((row) => {
            const pct = ranked.total > 0 ? (row.value / ranked.total) * 100 : 0;
            const widthPct = ranked.max > 0 ? (row.value / ranked.max) * 100 : 0;
            return (
              <li key={row.id} className="consumption-distribution-row">
                <span className="consumption-distribution-name" title={row.name}>{row.name}</span>
                <div className="consumption-distribution-bar-track" aria-hidden="true">
                  <span
                    className="consumption-distribution-bar-fill"
                    style={{ width: `${widthPct}%` }}
                  />
                </div>
                <span className="consumption-distribution-value">{formatValue(metric, row.value)}</span>
                <span className="consumption-distribution-pct">{pct.toFixed(1)}%</span>
              </li>
            );
          })}
        </ul>
      )}
      <div className="consumption-distribution-foot">
        {tr('合计')} {formatValue(metric, ranked.total)} · {tr('基于当前可见密钥')}
      </div>
    </div>
  );
}

export default ConsumptionDistribution;
