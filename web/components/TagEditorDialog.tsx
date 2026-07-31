import React, { useEffect, useMemo, useState } from 'react';
import CenteredModal from './CenteredModal.js';
import { tr } from '../i18n.js';
import { parseTags, tagColor } from '../pages/helpers/tags.js';

/* ------------------------------------------------------------------ */
/*  Props                                                              */
/* ------------------------------------------------------------------ */

interface TagEditorDialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  /** Current tags of the target row (JSON text or array). */
  initialTags: unknown;
  /** All tags in use (for quick-add chips). */
  allTags: string[];
  /** Persists the edited list; resolved toast message is shown by caller. */
  onSave: (tags: string[]) => Promise<void> | void;
}

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

/**
 * TagEditorDialog — shared tag editing for accounts/sites rows
 * (all-api-hub borrow I1). Free-text input (comma / newline separated) with
 * existing tags offered as quick-add chips.
 */
export default function TagEditorDialog({
  open,
  onClose,
  title,
  initialTags,
  allTags,
  onSave,
}: TagEditorDialogProps) {
  const [draft, setDraft] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setDraft(parseTags(initialTags).join(', '));
    setError(null);
  }, [open, initialTags]);

  const draftTags = useMemo(
    () => parseTags(draft),
    [draft],
  );

  const addTag = (tag: string) => {
    const next = Array.from(new Set([...draftTags, tag]));
    setDraft(next.join(', '));
  };

  const removeTag = (tag: string) => {
    setDraft(draftTags.filter((t) => t !== tag).join(', '));
  };

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      await onSave(draftTags);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const suggestions = allTags.filter((t) => !draftTags.includes(t)).slice(0, 12);

  return (
    <CenteredModal
      open={open}
      onClose={onClose}
      title={title}
      maxWidth={520}
      closeOnBackdrop
      closeOnEscape
      footer={
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button className="btn btn-ghost" onClick={onClose} style={{ border: '1px solid var(--color-border)' }}>
            {tr('取消')}
          </button>
          <button className="btn btn-primary" onClick={() => void save()} disabled={saving}>
            {saving ? tr('保存中...') : tr('保存')}
          </button>
        </div>
      }
    >
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginBottom: 12, minHeight: 28 }}>
        {draftTags.length === 0 && (
          <span style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('暂无标签')}</span>
        )}
        {draftTags.map((tag) => (
          <span
            key={tag}
            className="tag-chip"
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 4,
              padding: '2px 8px',
              borderRadius: 999,
              fontSize: 12,
              color: 'var(--color-text)',
              background: tagColor(tag),
            }}
          >
            {tag}
            <button
              type="button"
              aria-label={`${tr('移除标签')} ${tag}`}
              onClick={() => removeTag(tag)}
              style={{
                border: 'none',
                background: 'none',
                cursor: 'pointer',
                color: 'inherit',
                fontSize: 13,
                lineHeight: 1,
                padding: 0,
              }}
            >
              ×
            </button>
          </span>
        ))}
      </div>

      <input
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        placeholder={tr('输入标签，用逗号或换行分隔')}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            void save();
          }
        }}
        style={{
          width: '100%',
          height: 36,
          padding: '0 10px',
          borderRadius: 8,
          border: '1px solid var(--color-border)',
          background: 'var(--color-bg)',
          color: 'var(--color-text)',
          fontSize: 13,
        }}
      />

      {suggestions.length > 0 && (
        <div style={{ marginTop: 12 }}>
          <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginBottom: 6 }}>
            {tr('常用标签')}
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
            {suggestions.map((tag) => (
              <button
                key={tag}
                type="button"
                onClick={() => addTag(tag)}
                style={{
                  border: '1px solid var(--color-border-light)',
                  borderRadius: 999,
                  padding: '2px 10px',
                  fontSize: 12,
                  cursor: 'pointer',
                  background: 'var(--color-bg)',
                  color: 'var(--color-text-secondary)',
                }}
              >
                + {tag}
              </button>
            ))}
          </div>
        </div>
      )}

      {error && (
        <div style={{ fontSize: 12, color: 'var(--color-danger)', marginTop: 10 }}>{error}</div>
      )}
    </CenteredModal>
  );
}
