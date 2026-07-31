import React, { useCallback, useEffect, useState } from 'react';

import { api, type RateOverviewResponse } from '../../api.js';
import { useToast } from '../../components/Toast.js';
import { tr } from '../../i18n.js';

/**
 * N9a (New API borrow): rate/multiplier overview — read-only aggregation of
 * every multiplier surface (account unit cost, channel weight, site global
 * weight, downstream key weight, observed model costs) in one table.
 * Never mutates billing or routing; the write surface (N9b) stays out.
 */
export default function RatesOverviewSection() {
  const toast = useToast();
  const [data, setData] = useState<RateOverviewResponse | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      const res = await api.getRateOverview();
      setData(res);
    } catch (error: any) {
      toast.error(error?.message || tr('加载倍率总览失败'));
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void load();
  }, [load]);

  const fmtWeight = (v?: number | null): string =>
    v == null ? '—' : Number(v).toFixed(2);

  return (
    <div className="card" style={{ padding: 20 }}>
      <div style={{ marginBottom: 12 }}>
        <div style={{ fontWeight: 600, fontSize: 14 }}>{tr('倍率与权重总览')}</div>
        <div style={{ fontSize: 12, color: 'var(--color-text-muted)', marginTop: 2 }}>
          {tr('账号单价 / 通道权重 / 站点全局权重 / 下游 key 权重 / 观测成本——只读视图')}
        </div>
      </div>

      {loading ? (
        <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('加载中…')}</div>
      ) : !data ? (
        <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('加载失败')}</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <div style={{ display: 'flex', gap: 16, fontSize: 12, color: 'var(--color-text-muted)', flexWrap: 'wrap' }}>
            <span>{tr('账号')} <b style={{ color: 'var(--color-text-primary)' }}>{data.summary.accountsTotal}</b></span>
            <span>{tr('设置单价')} <b style={{ color: 'var(--color-text-primary)' }}>{data.summary.accountsWithUnitCost}</b></span>
            <span>{tr('通道')} <b style={{ color: 'var(--color-text-primary)' }}>{data.summary.channelsEnabled}/{data.summary.channelsTotal}</b></span>
          </div>

          {data.accounts.length > 0 && (
            <div style={{ overflowX: 'auto', border: '1px solid var(--color-border-light)', borderRadius: 8 }}>
              <table className="table" style={{ minWidth: 560 }}>
                <thead>
                  <tr>
                    <th>{tr('账号单价')}</th>
                    <th>{tr('账号')}</th>
                    <th>{tr('站点')}</th>
                    <th>{tr('通道数')}</th>
                    <th>{tr('通道总权重')}</th>
                  </tr>
                </thead>
                <tbody>
                  {data.accounts.map((a) => (
                    <tr key={a.accountId}>
                      <td style={{ fontWeight: 600 }}>{a.unitCost != null ? `$${Number(a.unitCost).toFixed(4)}` : '—'}</td>
                      <td style={{ fontSize: 12 }}>{a.username || `#${a.accountId}`}</td>
                      <td style={{ fontSize: 12 }}>{a.siteName || '—'}</td>
                      <td style={{ fontSize: 12 }}>{a.channelCount}</td>
                      <td style={{ fontSize: 12 }}>{fmtWeight(a.totalWeight)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {data.channels.length > 0 && (
            <div style={{ overflowX: 'auto', border: '1px solid var(--color-border-light)', borderRadius: 8 }}>
              <table className="table" style={{ minWidth: 560 }}>
                <thead>
                  <tr>
                    <th>{tr('通道权重')}</th>
                    <th>{tr('路由')}</th>
                    <th>{tr('模型')}</th>
                    <th>{tr('账号')}</th>
                    <th>{tr('状态')}</th>
                  </tr>
                </thead>
                <tbody>
                  {data.channels.map((c) => (
                    <tr key={c.channelId}>
                      <td style={{ fontWeight: 600 }}>{fmtWeight(c.weight)}</td>
                      <td style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{c.routePattern || '—'}</td>
                      <td style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{c.modelName || '—'}</td>
                      <td style={{ fontSize: 12 }}>{c.username || `#${c.accountId}`}</td>
                      <td>
                        <span className={`badge ${c.enabled ? 'badge-success' : 'badge-muted'}`} style={{ fontSize: 11 }}>
                          {c.enabled ? tr('启用') : tr('停用')}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {(data.sites.length > 0 || data.keys.length > 0) && (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 14 }}>
              {data.sites.length > 0 && (
                <div>
                  <div style={{ fontSize: 12, color: 'var(--color-text-muted)', marginBottom: 6 }}>{tr('站点全局权重')}</div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    {data.sites.map((s) => (
                      <div key={s.siteId} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
                        <span>{s.siteName}</span>
                        <b>{fmtWeight(s.globalWeight)}</b>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              {data.keys.length > 0 && (
                <div>
                  <div style={{ fontSize: 12, color: 'var(--color-text-muted)', marginBottom: 6 }}>{tr('下游 key 权重')}</div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    {data.keys.map((k) => (
                      <div key={k.keyId} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
                        <span>{k.name || `#${k.keyId}`}</span>
                        <b>{fmtWeight(k.keyWeight)}</b>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          {data.models.length > 0 && (
            <div style={{ overflowX: 'auto', border: '1px solid var(--color-border-light)', borderRadius: 8 }}>
              <table className="table" style={{ minWidth: 480 }}>
                <thead>
                  <tr>
                    <th>{tr('模型观测成本（近 30 天）')}</th>
                    <th>{tr('花费')}</th>
                    <th>{tr('调用')}</th>
                    <th>{tr('Token')}</th>
                  </tr>
                </thead>
                <tbody>
                  {data.models.map((m) => (
                    <tr key={m.model}>
                      <td style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{m.model}</td>
                      <td style={{ fontWeight: 600 }}>${Number(m.spend).toFixed(2)}</td>
                      <td style={{ fontSize: 12 }}>{m.calls}</td>
                      <td style={{ fontSize: 12 }}>{Number(m.tokens).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
