import React, { useCallback, useEffect, useState } from 'react';

import { api, type RateOverviewResponse } from '../../api.js';
import { useToast } from '../../components/Toast.js';
import { tr } from '../../i18n.js';

/**
 * N9a/N9b-a (New API borrow): rate/multiplier overview + batch editing.
 * Read-only aggregation of every multiplier surface (account unit cost,
 * channel weight, site global weight, downstream key weight, observed model
 * costs). N9b-a adds inline editing for accounts.unit_cost + channels.weight
 * — pure config writes; estimated_cost stays ratio-based (N9b-b closed).
 */
export default function RatesOverviewSection() {
  const toast = useToast();
  const [data, setData] = useState<RateOverviewResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  // Inline edit state: which cell is being edited and its current input.
  const [editing, setEditing] = useState<{
    kind: 'account' | 'channel';
    id: number;
    value: string;
  } | null>(null);

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

  const startEdit = (kind: 'account' | 'channel', id: number, current?: number | null) => {
    setEditing({ kind, id, value: current != null ? String(current) : '' });
  };

  const commitEdit = async () => {
    if (!editing || saving) return;
    const parsed = Number(editing.value);
    if (!editing.value.trim() || !Number.isFinite(parsed) || parsed < 0) {
      toast.error(tr('请输入不小于 0 的数值'));
      return;
    }
    setSaving(true);
    try {
      const body =
        editing.kind === 'account'
          ? { accounts: [{ id: editing.id, unitCost: parsed }] }
          : { channels: [{ id: editing.id, weight: parsed }] };
      const res = await api.updateRates(body);
      if (!res.success) {
        toast.error(tr('保存失败'));
        return;
      }
      toast.success(
        editing.kind === 'account' ? tr('账号单价已更新') : tr('通道权重已更新'),
      );
      setEditing(null);
      await load();
    } catch (error: any) {
      toast.error(error?.message || tr('保存失败'));
    } finally {
      setSaving(false);
    }
  };

  const cancelEdit = () => setEditing(null);

  const editCell = (
    kind: 'account' | 'channel',
    id: number,
    current: number | null | undefined,
    renderDisplay: () => React.ReactNode,
  ) => {
    if (editing && editing.kind === kind && editing.id === id) {
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
          <input
            type="number"
            min={0}
            step="any"
            autoFocus
            value={editing.value}
            onChange={(e) => setEditing({ ...editing, value: e.target.value })}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void commitEdit();
              if (e.key === 'Escape') cancelEdit();
            }}
            style={{
              width: 84,
              padding: '2px 6px',
              fontSize: 12,
              borderRadius: 4,
              border: '1px solid var(--color-border)',
              background: 'var(--color-bg)',
              color: 'var(--color-text-primary)',
            }}
          />
          <button
            type="button"
            className="btn btn-primary"
            disabled={saving}
            onClick={() => void commitEdit()}
            style={{ padding: '1px 8px', fontSize: 11, lineHeight: '18px' }}
          >
            {tr('保存')}
          </button>
          <button
            type="button"
            className="btn btn-ghost"
            disabled={saving}
            onClick={cancelEdit}
            style={{ padding: '1px 8px', fontSize: 11, lineHeight: '18px' }}
          >
            {tr('取消')}
          </button>
        </span>
      );
    }
    return (
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
        {renderDisplay()}
        <button
          type="button"
          className="btn btn-ghost"
          title={tr('编辑')}
          onClick={() => startEdit(kind, id, current)}
          style={{
            padding: '0 4px',
            fontSize: 12,
            lineHeight: '18px',
            border: '1px solid var(--color-border)',
          }}
        >
          ✎
        </button>
      </span>
    );
  };

  return (
    <div className="card" style={{ padding: 20 }}>
      <div style={{ marginBottom: 12 }}>
        <div style={{ fontWeight: 600, fontSize: 14 }}>{tr('倍率与权重总览')}</div>
        <div style={{ fontSize: 12, color: 'var(--color-text-muted)', marginTop: 2 }}>
          {tr('账号单价 / 通道权重 / 站点全局权重 / 下游 key 权重 / 观测成本')}
          {' — '}
          {tr('点击 ✎ 编辑单价与权重（不影响计费口径）')}
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
                      <td style={{ fontWeight: 600 }}>
                        {editCell('account', a.accountId, a.unitCost, () =>
                          a.unitCost != null ? `$${Number(a.unitCost).toFixed(4)}` : '—',
                        )}
                      </td>
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
                      <td style={{ fontWeight: 600 }}>
                        {editCell('channel', c.channelId, c.weight, () => fmtWeight(c.weight))}
                      </td>
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
