import React, { useCallback, useEffect, useState } from 'react';
import CenteredModal from './CenteredModal.js';
import {
  api,
  type ModelVerifyItem,
  type VerifyBatchResponse,
  type VerifyHistoryResponse,
} from '../api.js';
import { tr } from '../i18n.js';

/* ------------------------------------------------------------------ */
/*  Props                                                              */
/* ------------------------------------------------------------------ */

interface ModelVerifyDialogProps {
  open: boolean;
  onClose: () => void;
  /** Current filter-scoped model names; empty = verify all enabled channels. */
  models: string[];
}

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

/**
 * ModelVerifyDialog — batch model verification + verification history
 *. Runs one operator-initiated probe pass over the
 * selected models via POST /api/models/verify-batch, showing per-row status /
 * latency / HTTP result, with a durable history tab.
 */
export default function ModelVerifyDialog({
  open,
  onClose,
  models,
}: ModelVerifyDialogProps) {
  const [tab, setTab] = useState<'verify' | 'history'>('verify');
  const [running, setRunning] = useState(false);
  const [verifyResult, setVerifyResult] = useState<VerifyBatchResponse | null>(null);
  const [verifyError, setVerifyError] = useState<string | null>(null);
  const [history, setHistory] = useState<ModelVerifyItem[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState(false);

  const runVerify = useCallback(async () => {
    setRunning(true);
    setVerifyError(null);
    try {
      const res = await api.verifyModelsBatch(models.slice(0, 50));
      setVerifyResult(res);
      // Refresh history after a new batch.
      try {
        const h = await api.getModelVerifyHistory(50);
        setHistory(h.items);
      } catch {
        // history refresh is best-effort after verify
      }
    } catch (err) {
      setVerifyError(err instanceof Error ? err.message : String(err));
      setVerifyResult(null);
    } finally {
      setRunning(false);
    }
  }, [models]);

  const loadHistory = useCallback(async () => {
    setHistoryLoading(true);
    setHistoryError(false);
    try {
      const res = await api.getModelVerifyHistory(50);
      setHistory(res.items);
    } catch {
      setHistoryError(true);
    } finally {
      setHistoryLoading(false);
    }
  }, []);

  // Reset transient state when the dialog reopens.
  useEffect(() => {
    if (!open) return;
    setTab('verify');
    setVerifyResult(null);
    setVerifyError(null);
  }, [open]);

  useEffect(() => {
    if (open && tab === 'history') void loadHistory();
  }, [open, tab, loadHistory]);

  const summary = verifyResult?.summary;
  const probed = verifyResult?.probed ?? 0;

  return (
    <CenteredModal
      open={open}
      onClose={onClose}
      title={tr('批量验证模型')}
      maxWidth={880}
      closeOnBackdrop
      closeOnEscape
      footer={
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button className="btn btn-ghost" onClick={onClose} style={{ border: '1px solid var(--color-border)' }}>
            {tr('关闭')}
          </button>
        </div>
      }
    >
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', marginBottom: 16 }}>
        <div style={{ display: 'flex', gap: 4, background: 'var(--color-bg)', borderRadius: 8, padding: 3 }}>
          <button
            className={`view-toggle-btn ${tab === 'verify' ? 'active' : ''}`}
            onClick={() => setTab('verify')}
            style={{ padding: '5px 14px', fontSize: 13 }}
          >
            {tr('验证')}
          </button>
          <button
            className={`view-toggle-btn ${tab === 'history' ? 'active' : ''}`}
            onClick={() => setTab('history')}
            style={{ padding: '5px 14px', fontSize: 13 }}
          >
            {tr('验证历史')}
          </button>
        </div>
        {tab === 'verify' && (
          <button
            className="btn btn-primary"
            onClick={() => void runVerify()}
            disabled={running}
            style={{ padding: '6px 16px' }}
          >
            {running ? tr('验证中...') : tr('开始验证')}
          </button>
        )}
      </div>

      {tab === 'verify' && (
        <div>
          <p style={{ fontSize: 13, color: 'var(--color-text-muted)', margin: '0 0 12px' }}>
            {models.length > 0
              ? `${tr('将对当前筛选的')} ${Math.min(models.length, 50)} ${tr('个模型逐个发起轻量探测请求')}`
              : tr('将对全部启用渠道发起轻量探测请求')}
            。{tr('结果会记录到验证历史，并同步路由健康状态。')}
          </p>

          {verifyError && (
            <div style={{ fontSize: 13, color: 'var(--color-danger)', marginBottom: 12 }}>{verifyError}</div>
          )}

          {verifyResult && !verifyError && (
            <div style={{ marginBottom: 12, fontSize: 13 }}>
              <span className="badge badge-success" style={{ marginRight: 8 }}>
                {tr('成功')} {summary?.success ?? 0}
              </span>
              <span className="badge badge-danger" style={{ marginRight: 8 }}>
                {tr('失败')} {summary?.failure ?? 0}
              </span>
              <span className="badge badge-warning" style={{ marginRight: 8 }}>
                {tr('不确定')} {summary?.inconclusive ?? 0}
              </span>
              <span className="badge badge-muted">{tr('跳过')} {summary?.skipped ?? 0}</span>
              <span style={{ marginLeft: 12, color: 'var(--color-text-muted)' }}>
                {tr('共探测')} {probed} {tr('条')}
              </span>
            </div>
          )}

          {verifyResult && verifyResult.items.length > 0 && (
            <div style={{ overflowX: 'auto', border: '1px solid var(--color-border-light)', borderRadius: 8 }}>
              <table className="table" style={{ minWidth: 640 }}>
                <thead>
                  <tr>
                    <th>{tr('模型')}</th>
                    <th>{tr('站点')}</th>
                    <th>{tr('状态')}</th>
                    <th>{tr('延迟')}</th>
                    <th>{tr('HTTP')}</th>
                    <th>{tr('错误')}</th>
                  </tr>
                </thead>
                <tbody>
                  {verifyResult.items.map((item, idx) => (
                    <tr key={`${item.model}-${item.channelId ?? idx}`}>
                      <td style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{item.model}</td>
                      <td style={{ fontSize: 12 }}>{item.siteName ?? '—'}</td>
                      <td><StatusBadge status={item.status} /></td>
                      <td style={{ fontSize: 12 }}>{item.latencyMs != null && item.latencyMs > 0 ? `${Math.round(item.latencyMs)} ms` : '—'}</td>
                      <td style={{ fontSize: 12 }}>{item.httpStatus ?? '—'}</td>
                      <td style={{ fontSize: 12, maxWidth: 220, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={item.errorText ?? ''}>
                        {item.errorText || '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {verifyResult && verifyResult.items.length === 0 && (
            <div style={{ fontSize: 13, color: 'var(--color-text-muted)' }}>
              {verifyResult.note ?? tr('没有匹配的启用渠道')}
            </div>
          )}
        </div>
      )}

      {tab === 'history' && (
        <div>
          {historyLoading && (
            <div className="skeleton" style={{ width: '100%', height: 200, borderRadius: 8 }} />
          )}
          {!historyLoading && historyError && (
            <div style={{ fontSize: 13, color: 'var(--color-danger)' }}>{tr('验证历史加载失败')}</div>
          )}
          {!historyLoading && !historyError && history.length === 0 && (
            <div style={{ fontSize: 13, color: 'var(--color-text-muted)' }}>{tr('暂无验证历史')}</div>
          )}
          {!historyLoading && !historyError && history.length > 0 && (
            <div style={{ overflowX: 'auto', border: '1px solid var(--color-border-light)', borderRadius: 8 }}>
              <table className="table" style={{ minWidth: 640 }}>
                <thead>
                  <tr>
                    <th>{tr('时间')}</th>
                    <th>{tr('模型')}</th>
                    <th>{tr('站点')}</th>
                    <th>{tr('状态')}</th>
                    <th>{tr('延迟')}</th>
                    <th>{tr('HTTP')}</th>
                  </tr>
                </thead>
                <tbody>
                  {history.map((item) => (
                    <tr key={`${item.createdAt}-${item.model}-${item.channelId}`}>
                      <td style={{ fontSize: 12, whiteSpace: 'nowrap' }}>{formatTime(item.createdAt)}</td>
                      <td style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{item.model}</td>
                      <td style={{ fontSize: 12 }}>{item.siteName ?? '—'}</td>
                      <td><StatusBadge status={item.status} /></td>
                      <td style={{ fontSize: 12 }}>{item.latencyMs != null && item.latencyMs > 0 ? `${Math.round(item.latencyMs)} ms` : '—'}</td>
                      <td style={{ fontSize: 12 }}>{item.httpStatus ?? '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </CenteredModal>
  );
}

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function StatusBadge({ status }: { status: ModelVerifyItem['status'] }) {
  const tone =
    status === 'success' ? 'success' : status === 'failure' ? 'danger' : status === 'inconclusive' ? 'warning' : 'muted';
  const label =
    status === 'success' ? tr('成功') : status === 'failure' ? tr('失败') : status === 'inconclusive' ? tr('不确定') : tr('跳过');
  return <span className={`badge badge-${tone}`} style={{ fontSize: 12 }}>{label}</span>;
}

function formatTime(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
