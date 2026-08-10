import React, { useCallback, useEffect, useState } from 'react';

import { api, type Announcement } from '../../api.js';
import { useToast } from '../../components/Toast.js';
import { tr } from '../../i18n.js';

/**
 * H1: product risk banners — admin authoring surface.
 * Operator-written severity-ranked announcements surface on the Dashboard as
 * dismissible banners (替代邮件群发). Content edits reset dismissals so a new
 * revision is seen again.
 */

const SEVERITY_LABEL: Record<Announcement['severity'], string> = {
  critical: tr('严重'),
  warning: tr('警告'),
  info: tr('信息'),
};

const SEVERITY_CLASS: Record<Announcement['severity'], string> = {
  critical: 'badge-danger',
  warning: 'badge-warning',
  info: 'badge-info',
};

type Draft = {
  title: string;
  message: string;
  severity: Announcement['severity'];
  link: string;
  enabled: boolean;
};

const EMPTY_DRAFT: Draft = { title: '', message: '', severity: 'info', link: '', enabled: true };

export default function AnnouncementsSection() {
  const toast = useToast();
  const [items, setItems] = useState<Announcement[]>([]);
  const [loading, setLoading] = useState(true);
  const [formOpen, setFormOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [saving, setSaving] = useState(false);
  const [deletingId, setDeletingId] = useState<number | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await api.getAnnouncements();
      setItems(Array.isArray(res?.items) ? res.items : []);
    } catch (error: any) {
      toast.error(error?.message || tr('加载公告失败'));
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void load();
  }, [load]);

  const startCreate = () => {
    setFormOpen(true);
    setEditingId(null);
    setDraft(EMPTY_DRAFT);
  };

  const startEdit = (item: Announcement) => {
    setFormOpen(true);
    setEditingId(item.id);
    setDraft({
      title: item.title,
      message: item.message,
      severity: item.severity,
      link: item.link ?? '',
      enabled: item.enabled,
    });
  };

  const closeForm = () => {
    setFormOpen(false);
    setEditingId(null);
    setDraft(EMPTY_DRAFT);
  };

  const save = async () => {
    if (!draft.title.trim() || !draft.message.trim()) {
      toast.error(tr('标题和内容不能为空'));
      return;
    }
    setSaving(true);
    try {
      const payload = {
        title: draft.title.trim(),
        message: draft.message.trim(),
        severity: draft.severity,
        link: draft.link.trim() || null,
        enabled: draft.enabled,
      };
      if (editingId == null) {
        await api.createAnnouncement(payload);
        toast.success(tr('公告已发布'));
      } else {
        const res = await api.updateAnnouncement(editingId, payload);
        toast.success(res.revision ? tr('公告已更新（已重置关闭状态）') : tr('公告已更新'));
      }
      closeForm();
      await load();
    } catch (error: any) {
      toast.error(error?.message || tr('保存公告失败'));
    } finally {
      setSaving(false);
    }
  };

  const remove = async (id: number) => {
    setDeletingId(id);
    try {
      await api.deleteAnnouncement(id);
      toast.success(tr('公告已删除'));
      await load();
    } catch (error: any) {
      toast.error(error?.message || tr('删除公告失败'));
    } finally {
      setDeletingId(null);
    }
  };

  const inputStyle: React.CSSProperties = {
    width: '100%',
    height: 34,
    padding: '0 10px',
    borderRadius: 8,
    border: '1px solid var(--color-border)',
    background: 'var(--color-bg)',
    color: 'var(--color-text)',
    fontSize: 13,
  };

  return (
    <div className="card" style={{ padding: 20 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
        <div>
          <div style={{ fontWeight: 600, fontSize: 14 }}>{tr('产品公告')}</div>
          <div style={{ fontSize: 12, color: 'var(--color-text-muted)', marginTop: 2 }}>
            {tr('在 Dashboard 顶部显示的风险横幅，按严重程度分级，可关闭')}
          </div>
        </div>
        <button
          type="button"
          className="btn btn-soft-primary"
          style={{ fontSize: 12, padding: '5px 12px' }}
          onClick={startCreate}
          data-testid="new-announcement"
        >
          {tr('+ 新建公告')}
        </button>
      </div>

      {loading ? (
        <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('加载中…')}</div>
      ) : items.length === 0 && !formOpen ? (
        <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('暂无公告')}</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {items.map((item) => (
            <div
              key={item.id}
              data-testid={`announcement-row-${item.id}`}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                padding: '8px 12px',
                border: '1px solid var(--color-border-light)',
                borderRadius: 8,
                background: 'var(--color-bg)',
              }}
            >
              <span className={`badge ${SEVERITY_CLASS[item.severity]}`} style={{ fontSize: 11, minWidth: 40, textAlign: 'center' }}>
                {SEVERITY_LABEL[item.severity]}
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 13, fontWeight: 600 }}>{item.title}</div>
                <div
                  style={{
                    fontSize: 12,
                    color: 'var(--color-text-muted)',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {item.message}
                </div>
              </div>
              <span className={`badge ${item.enabled ? 'badge-success' : 'badge-muted'}`} style={{ fontSize: 11 }}>
                {item.enabled ? tr('启用') : tr('停用')}
              </span>
              {item.dismissed ? (
                <span className="badge badge-muted" style={{ fontSize: 11 }} title={tr('该版本已被关闭，内容变更后会重新显示')}>
                  {tr('已关闭')}
                </span>
              ) : null}
              <button
                type="button"
                className="btn btn-link btn-link-primary"
                onClick={() => startEdit(item)}
              >
                {tr('编辑')}
              </button>
              <button
                type="button"
                className="btn btn-link btn-link-danger"
                disabled={deletingId === item.id}
                onClick={() => void remove(item.id)}
              >
                {deletingId === item.id ? <span className="spinner spinner-sm" /> : tr('删除')}
              </button>
            </div>
          ))}
        </div>
      )}

      {formOpen ? (
        <div
          style={{
            marginTop: 14,
            padding: 12,
            border: '1px solid var(--color-border-light)',
            borderRadius: 8,
            display: 'flex',
            flexDirection: 'column',
            gap: 10,
          }}
          data-testid="announcement-form"
        >
          <div style={{ fontSize: 13, fontWeight: 600 }}>
            {editingId == null ? tr('新建公告') : tr('编辑公告')}
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
            <input
              value={draft.title}
              onChange={(e) => setDraft((prev) => ({ ...prev, title: e.target.value }))}
              placeholder={tr('标题')}
              style={inputStyle}
              data-testid="announcement-title"
            />
            <select
              value={draft.severity}
              onChange={(e) =>
                setDraft((prev) => ({ ...prev, severity: e.target.value as Announcement['severity'] }))
              }
              style={inputStyle}
              data-testid="announcement-severity"
            >
              <option value="info">{tr('信息')}</option>
              <option value="warning">{tr('警告')}</option>
              <option value="critical">{tr('严重')}</option>
            </select>
          </div>
          <textarea
            value={draft.message}
            onChange={(e) => setDraft((prev) => ({ ...prev, message: e.target.value }))}
            placeholder={tr('内容')}
            rows={3}
            style={{ ...inputStyle, height: 'auto', padding: '8px 10px', resize: 'vertical' }}
            data-testid="announcement-message"
          />
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
            <input
              value={draft.link}
              onChange={(e) => setDraft((prev) => ({ ...prev, link: e.target.value }))}
              placeholder={tr('详情链接（可选）')}
              style={{ ...inputStyle, flex: 1, minWidth: 200 }}
              data-testid="announcement-link"
            />
            <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'var(--color-text-secondary)' }}>
              <input
                type="checkbox"
                checked={draft.enabled}
                onChange={(e) => setDraft((prev) => ({ ...prev, enabled: e.target.checked }))}
              />
              {tr('启用')}
            </label>
          </div>
          <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
            <button
              type="button"
              className="btn btn-ghost"
              style={{ border: '1px solid var(--color-border)', fontSize: 12 }}
              onClick={closeForm}
            >
              {tr('取消')}
            </button>
            <button
              type="button"
              className="btn btn-primary"
              style={{ fontSize: 12 }}
              disabled={saving}
              onClick={() => void save()}
              data-testid="announcement-save"
            >
              {saving ? tr('保存中...') : tr('保存')}
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
