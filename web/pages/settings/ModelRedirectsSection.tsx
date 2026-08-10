import React, { useCallback, useEffect, useState } from 'react';

import { api, type ModelRedirect, type RedirectFixCandidate } from '../../api.js';
import { useToast } from '../../components/Toast.js';
import { tr } from '../../i18n.js';

/**
 * K1a: model name redirects — operator surface.
 * Sync generation maps canonical route names to actual upstream names
 * (e.g. claude-3-5-sonnet → claude-3-5-sonnet-20241022); disabled-model
 * entries fixable via redirects are previewed (dry-run) and applied on
 * confirmation. Route matching canonicalization stays out of scope.
 */
export default function ModelRedirectsSection() {
  const toast = useToast();
  const [items, setItems] = useState<ModelRedirect[]>([]);
  const [loading, setLoading] = useState(true);
  const [generating, setGenerating] = useState(false);
  const [fixCandidates, setFixCandidates] = useState<RedirectFixCandidate[]>([]);
  const [previewed, setPreviewed] = useState(false);
  const [applying, setApplying] = useState(false);
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const [promotingId, setPromotingId] = useState<number | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await api.getModelRedirects();
      setItems(Array.isArray(res?.items) ? res.items : []);
    } catch (error: any) {
      toast.error(error?.message || tr('加载模型映射失败'));
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void load();
  }, [load]);

  const generate = async () => {
    setGenerating(true);
    try {
      const res = await api.generateModelRedirects(0);
      toast.success(res.accounts != null
        ? `${tr('映射已生成')}：${res.accounts} ${tr('个账号')} / ${res.created} ${tr('条新建')}`
        : `${tr('映射已生成')}：${res.created} ${tr('条新建')}`);
      await load();
    } catch (error: any) {
      toast.error(error?.message || tr('生成映射失败'));
    } finally {
      setGenerating(false);
    }
  };

  const preview = async () => {
    try {
      const res = await api.applyModelRedirects(true);
      setFixCandidates(res.candidates ?? []);
      setPreviewed(true);
      if (res.count === 0) {
        toast.info(tr('没有可通过映射修复的禁用模型'));
      }
    } catch (error: any) {
      toast.error(error?.message || tr('预览失败'));
    }
  };

  const apply = async () => {
    setApplying(true);
    try {
      const res = await api.applyModelRedirects(false);
      toast.success(`${tr('已修复')} ${res.removed ?? 0} ${tr('个禁用条目')}`);
      setFixCandidates([]);
      setPreviewed(false);
    } catch (error: any) {
      toast.error(error?.message || tr('应用修复失败'));
    } finally {
      setApplying(false);
    }
  };

  const promote = async (item: ModelRedirect) => {
    setPromotingId(item.id);
    try {
      await api.updateModelRedirect(item.id, { source: 'manual' });
      toast.success(tr('已转为手动映射（同步不会覆盖）'));
      await load();
    } catch (error: any) {
      toast.error(error?.message || tr('转换失败'));
    } finally {
      setPromotingId(null);
    }
  };

  const remove = async (id: number) => {
    setDeletingId(id);
    try {
      await api.deleteModelRedirect(id);
      toast.success(tr('映射已删除'));
      await load();
    } catch (error: any) {
      toast.error(error?.message || tr('删除失败'));
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div className="card" style={{ padding: 20 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
        <div>
          <div style={{ fontWeight: 600, fontSize: 14 }}>{tr('模型重定向映射')}</div>
          <div style={{ fontSize: 12, color: 'var(--color-text-muted)', marginTop: 2 }}>
            {tr('上游模型名 → 标准名（如 claude-3-5-sonnet-20241022 → claude-3-5-sonnet），模型同步后自动生成')}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button
            type="button"
            className="btn btn-soft-primary"
            style={{ fontSize: 12, padding: '5px 12px' }}
            onClick={() => void generate()}
            disabled={generating}
            data-testid="redirects-generate"
          >
            {generating ? tr('生成中...') : tr('生成映射')}
          </button>
          <button
            type="button"
            className="btn btn-ghost"
            style={{ fontSize: 12, padding: '5px 12px', border: '1px solid var(--color-border)' }}
            onClick={() => void preview()}
            data-testid="redirects-preview"
          >
            {tr('预览可修复项')}
          </button>
        </div>
      </div>

      {loading ? (
        <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('加载中…')}</div>
      ) : items.length === 0 ? (
        <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('暂无映射（模型同步后自动生成，或手动生成）')}</div>
      ) : (
        <div style={{ overflowX: 'auto', border: '1px solid var(--color-border-light)', borderRadius: 8 }}>
          <table className="table" style={{ minWidth: 640 }}>
            <thead>
              <tr>
                <th>{tr('标准名')}</th>
                <th>{tr('上游实际名')}</th>
                <th>{tr('账号 / 站点')}</th>
                <th>{tr('来源')}</th>
                <th style={{ textAlign: 'right' }}>{tr('操作')}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id} data-testid={`redirect-row-${item.id}`}>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{item.canonical}</td>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{item.actual}</td>
                  <td style={{ fontSize: 12 }}>
                    {item.siteName || '—'} / {item.username || `#${item.accountId}`}
                  </td>
                  <td>
                    <span className={`badge ${item.source === 'manual' ? 'badge-warning' : 'badge-muted'}`} style={{ fontSize: 11 }}>
                      {item.source === 'manual' ? tr('手动') : tr('自动')}
                    </span>
                  </td>
                  <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                    {item.source === 'sync' && (
                      <button
                        type="button"
                        className="btn btn-link btn-link-muted"
                        disabled={promotingId === item.id}
                        onClick={() => void promote(item)}
                      >
                        {promotingId === item.id ? <span className="spinner spinner-sm" /> : tr('转手动')}
                      </button>
                    )}
                    <button
                      type="button"
                      className="btn btn-link btn-link-danger"
                      disabled={deletingId === item.id}
                      onClick={() => void remove(item.id)}
                    >
                      {deletingId === item.id ? <span className="spinner spinner-sm" /> : tr('删除')}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {previewed && (
        <div style={{ marginTop: 14, padding: 12, border: '1px solid var(--color-border-light)', borderRadius: 8 }}>
          <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 8 }}>
            {tr('可修复的禁用模型')}（{fixCandidates.length}）
          </div>
          {fixCandidates.length === 0 ? (
            <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('没有可通过映射修复的禁用模型')}</div>
          ) : (
            <>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginBottom: 10 }}>
                {fixCandidates.map((c, idx) => (
                  <div key={`${c.siteId}-${c.modelName}-${idx}`} style={{ fontSize: 12, color: 'var(--color-text-secondary)' }}>
                    {tr('站点')} {c.siteName} · {c.modelName}{' '}
                    <span style={{ color: 'var(--color-text-muted)' }}>→ {c.actual} {tr('可用，将从禁用列表移除')}</span>
                  </div>
                ))}
              </div>
              <button
                type="button"
                className="btn btn-primary"
                style={{ fontSize: 12 }}
                disabled={applying}
                onClick={() => void apply()}
                data-testid="redirects-apply"
              >
                {applying ? tr('修复中...') : tr('确认修复')}
              </button>
            </>
          )}
        </div>
      )}

      <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginTop: 12, lineHeight: 1.6 }}>
        {tr('同步生成的映射不会覆盖手动映射；同一标准名只保留首个命中的实际名。修复操作仅移除「实际名已可用」的禁用条目，且需预览确认后执行。')}
      </div>
    </div>
  );
}
