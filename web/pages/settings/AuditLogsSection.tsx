import React, { useCallback, useEffect, useRef, useState } from 'react';

import { api } from '../../api.js';
import { useToast } from '../../components/Toast.js';
import { tr } from '../../i18n.js';

interface AuditLogItem {
  id: number;
  actor: string;
  method: string;
  path: string;
  status: number;
  requestId?: string;
  remoteIp?: string;
  createdAt: string;
}

interface AuditLogsResponse {
  items: AuditLogItem[];
  total: number;
  limit: number;
}

const methodTone: Record<string, string> = {
  POST: 'badge-primary',
  PUT: 'badge-warning',
  PATCH: 'badge-warning',
  DELETE: 'badge-danger',
};

/**
 * B1 (sub2api/cliproxyapi borrow): admin write-operation audit log.
 * Traces authenticated admin mutations (method/path/status/actor/ip) —
 * the compliance floor for a managed gateway. Read-only surface.
 */
export default function AuditLogsSection() {
  const toast = useToast();
  const [items, setItems] = useState<AuditLogItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [method, setMethod] = useState('');
  const [pathQuery, setPathQuery] = useState('');
  // Submitted filters — typing updates pathQuery but only Enter/查询 refetch.
  const [submitted, setSubmitted] = useState({ method: '', path: '' });
  const requestSeq = useRef(0);

  const load = useCallback(async (m: string, p: string) => {
    const seq = ++requestSeq.current;
    setLoading(true);
    try {
      const params = new URLSearchParams();
      if (m) params.set('method', m);
      if (p.trim()) params.set('path', p.trim());
      const res = await api.getAdminAuditLogs(params);
      if (seq !== requestSeq.current) return; // stale response — drop it
      setItems(res.items);
      setTotal(res.total);
    } catch (error: any) {
      if (seq !== requestSeq.current) return;
      toast.error(error?.message || tr('加载审计日志失败'));
    } finally {
      if (seq === requestSeq.current) setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void load(submitted.method, submitted.path);
  }, [load, submitted]);

  const applyFilters = () => {
    setSubmitted({ method, path: pathQuery });
  };

  return (
    <div className="card animate-slide-up stagger-5" style={{ padding: 20 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 4, flexWrap: 'wrap', gap: 8 }}>
        <div>
          <div style={{ fontWeight: 600, fontSize: 14 }}>{tr('管理操作审计')}</div>
          <div style={{ fontSize: 12, color: 'var(--color-text-muted)', marginTop: 2 }}>
            {tr('管理员写操作（增/删/改）留痕——方法 / 路径 / 状态 / 操作者 / IP')}
          </div>
        </div>
        {!loading && (
          <span style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>
            {tr('共')} {total} {tr('条')}
          </span>
        )}
      </div>

      <div style={{ display: 'flex', gap: 8, margin: '10px 0 12px', flexWrap: 'wrap' }}>
        <select
          value={method}
          onChange={(e) => setMethod(e.target.value)}
          style={{
            padding: '4px 8px',
            fontSize: 12,
            borderRadius: 4,
            border: '1px solid var(--color-border)',
            background: 'var(--color-bg)',
            color: 'var(--color-text-primary)',
          }}
        >
          <option value="">{tr('全部方法')}</option>
          <option value="POST">POST</option>
          <option value="PUT">PUT</option>
          <option value="PATCH">PATCH</option>
          <option value="DELETE">DELETE</option>
        </select>
        <input
          type="text"
          placeholder={tr('按路径搜索…')}
          value={pathQuery}
          onChange={(e) => setPathQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') applyFilters();
          }}
          style={{
            flex: 1,
            minWidth: 180,
            padding: '4px 8px',
            fontSize: 12,
            borderRadius: 4,
            border: '1px solid var(--color-border)',
            background: 'var(--color-bg)',
            color: 'var(--color-text-primary)',
          }}
        />
        <button
          type="button"
          className="btn btn-ghost"
          style={{ padding: '3px 12px', fontSize: 12, border: '1px solid var(--color-border)' }}
          onClick={applyFilters}
        >
          {tr('查询')}
        </button>
      </div>

      {loading ? (
        <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('加载中…')}</div>
      ) : items.length === 0 ? (
        <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('暂无审计记录（写操作后自动留痕）')}</div>
      ) : (
        <div style={{ overflowX: 'auto', border: '1px solid var(--color-border-light)', borderRadius: 8 }}>
          <table className="table" style={{ minWidth: 720 }}>
            <thead>
              <tr>
                <th>{tr('时间')}</th>
                <th>{tr('方法')}</th>
                <th>{tr('路径')}</th>
                <th>{tr('状态')}</th>
                <th>{tr('操作者')}</th>
                <th>{tr('IP')}</th>
                <th>{tr('请求 ID')}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id}>
                  <td style={{ fontSize: 12, whiteSpace: 'nowrap' }}>
                    {new Date(item.createdAt).toLocaleString()}
                  </td>
                  <td>
                    <span className={`badge ${methodTone[item.method] ?? 'badge-muted'}`} style={{ fontSize: 11 }}>
                      {item.method}
                    </span>
                  </td>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: 12, wordBreak: 'break-all' }}>
                    {item.path}
                  </td>
                  <td style={{ fontSize: 12 }}>
                    <span style={{ color: item.status >= 400 ? 'var(--color-danger, #ea4335)' : 'var(--color-success, #34a853)' }}>
                      {item.status}
                    </span>
                  </td>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{item.actor}</td>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{item.remoteIp || '—'}</td>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--color-text-muted)' }}>
                    {item.requestId || '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
