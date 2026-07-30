import { useState, useEffect, useRef, useCallback, useMemo, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api.js';
import { formatDateLocal, formatDateTimeMinuteLocal } from '../pages/helpers/checkinLogTime.js';
import { buildAccountFocusPath, buildSiteFocusPath, buildTokenFocusPath } from '../pages/helpers/navigationFocus.js';
import { useI18n } from '../i18n.js';
import { useAnimatedVisibility } from './useAnimatedVisibility.js';
import { useFocusTrap } from './useFocusTrap.js';

interface SiteResult {
  id: number;
  name: string;
  url: string;
}

interface AccountResult {
  id: number;
  username: string | null;
  status?: string | null;
  balance?: number | null;
  segment?: 'session' | 'apikey';
  site?: { name: string } | null;
}

interface AccountTokenResult {
  id: number;
  accountId: number;
  name: string;
  tokenGroup?: string | null;
  account?: {
    username?: string | null;
    segment?: 'session' | 'apikey';
  } | null;
  site?: { name: string } | null;
}

interface CheckinLogResult {
  id: number;
  accountId: number;
  message?: string | null;
  createdAt?: string | null;
  account?: { username?: string | null } | null;
}

interface ProxyLogResult {
  id: number;
  modelRequested?: string | null;
  status?: string | null;
  latencyMs?: number | null;
  createdAt?: string | null;
}

interface ModelSearchResult {
  name: string;
  accountCount: number;
  tokenCount: number;
  siteCount: number;
}

interface SearchResult {
  accounts: AccountResult[];
  accountTokens: AccountTokenResult[];
  sites: SiteResult[];
  checkinLogs: CheckinLogResult[];
  proxyLogs: ProxyLogResult[];
  models: ModelSearchResult[];
}

type SectionKey = 'models' | 'sites' | 'accounts' | 'accountTokens' | 'checkinLogs' | 'proxyLogs';

interface FlatItem {
  key: string;
  path: string;
  sectionKey: SectionKey;
  icon: ReactNode;
  title: string;
  meta: string;
}

// Section icons (kept out of render to keep the flat-map lean).
const ICON: Record<SectionKey, ReactNode> = {
  models: <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L4 12l5.75-5M14.25 7L20 12l-5.75 5M14 4l-4 16" /></svg>,
  sites: <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9" /></svg>,
  accounts: <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>,
  accountTokens: <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" /></svg>,
  checkinLogs: <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>,
  proxyLogs: <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2" /></svg>,
};

const SECTION_TITLE_KEY: Record<SectionKey, string> = {
  models: '模型广场',
  sites: '站点',
  accounts: '账号',
  accountTokens: '账号令牌',
  checkinLogs: '签到记录',
  proxyLogs: '使用日志',
};

export default function SearchModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useI18n();
  const presence = useAnimatedVisibility(open, 180);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult | null>(null);
  const [loading, setLoading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();
  const timerRef = useRef<number | undefined>(undefined);

  useFocusTrap(open && presence.shouldRender, panelRef);

  useEffect(() => {
    if (open) {
      setQuery('');
      setResults(null);
      // Input also receives focus via trap; keep explicit focus for input-first UX.
      setTimeout(() => inputRef.current?.focus(), 100);
    }
  }, [open]);

  const [activeIndex, setActiveIndex] = useState(-1);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);

  // Flat, ordered list of all result items (preserves section order:
  // models → sites → accounts → accountTokens → checkinLogs → proxyLogs)
  // so a single global index can drive keyboard navigation.
  const flat = useMemo<FlatItem[]>(() => {
    if (!results) return [];
    const out: FlatItem[] = [];
    for (const m of results.models) {
      out.push({
        key: `m-${m.name}`,
        path: `/models?q=${encodeURIComponent(m.name)}`,
        sectionKey: 'models',
        icon: ICON.models,
        title: m.name,
        meta: `${m.accountCount} ${t('个账号')} · ${m.tokenCount} ${t('个令牌')} · ${m.siteCount} ${t('个站点')}`,
      });
    }
    for (const s of results.sites) {
      out.push({ key: `s-${s.id}`, path: buildSiteFocusPath(s.id), sectionKey: 'sites', icon: ICON.sites, title: s.name, meta: s.url });
    }
    for (const a of results.accounts) {
      out.push({
        key: `a-${a.id}`,
        path: buildAccountFocusPath(a.id, { openRebind: a.status === 'expired', segment: a.segment }),
        sectionKey: 'accounts',
        icon: ICON.accounts,
        title: a.username?.trim() || (a.segment === 'apikey' ? t('API Key 连接') : `ID:${a.id}`),
        meta: `${a.site?.name || t('未关联站点')}${a.segment === 'apikey' ? ` · ${t('API Key 连接')}` : ''} · ${t('余额')} $${(a.balance || 0).toFixed(2)}`,
      });
    }
    for (const token of results.accountTokens) {
      out.push({
        key: `t-${token.id}`,
        path: buildTokenFocusPath(token.id),
        sectionKey: 'accountTokens',
        icon: ICON.accountTokens,
        title: token.name,
        meta: `${token.account?.username?.trim() || (token.account?.segment === 'apikey' ? t('API Key 连接') : t('未命名'))} · ${token.site?.name || t('未关联站点')}${token.tokenGroup ? ` · ${token.tokenGroup}` : ''}`,
      });
    }
    for (const l of results.checkinLogs) {
      out.push({
        key: `c-${l.id}`,
        path: '/checkin',
        sectionKey: 'checkinLogs',
        icon: ICON.checkinLogs,
        title: l.account?.username || `ID:${l.accountId}`,
        meta: `${l.message || '-'} · ${formatDateLocal(l.createdAt)}`,
      });
    }
    for (const l of results.proxyLogs) {
      out.push({
        key: `p-${l.id}`,
        path: '/logs',
        sectionKey: 'proxyLogs',
        icon: ICON.proxyLogs,
        title: l.modelRequested || '-',
        meta: `${l.status || '-'} · ${l.latencyMs || 0}ms · ${formatDateTimeMinuteLocal(l.createdAt)}`,
      });
    }
    return out;
  }, [results, t]);

  // Keep the active row scrolled into view as the user moves with arrow keys.
  useEffect(() => {
    const el = itemRefs.current[activeIndex];
    if (el && typeof el.scrollIntoView === 'function') {
      el.scrollIntoView({ block: 'nearest' });
    }
  }, [activeIndex]);

  const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    const n = flat.length;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (n) setActiveIndex(i => (i + 1) % n);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (n) setActiveIndex(i => (i <= 0 ? n - 1 : i - 1));
    } else if (e.key === 'Enter') {
      const item = flat[activeIndex] ?? (n === 1 ? flat[0] : null);
      if (item) {
        e.preventDefault();
        goTo(item.path);
      }
    }
  };

  const doSearch = useCallback(async (q: string) => {
    if (!q.trim()) {
      setResults(null);
      return;
    }

    setLoading(true);
    try {
      const res = await api.search(q);
      setResults({
        models: Array.isArray(res?.models) ? res.models : [],
        accounts: Array.isArray(res?.accounts) ? res.accounts : [],
        accountTokens: Array.isArray(res?.accountTokens) ? res.accountTokens : [],
        sites: Array.isArray(res?.sites) ? res.sites : [],
        checkinLogs: Array.isArray(res?.checkinLogs) ? res.checkinLogs : [],
        proxyLogs: Array.isArray(res?.proxyLogs) ? res.proxyLogs : [],
      });
    } catch {
      // ignore search errors in modal
    } finally {
      setLoading(false);
    }
  }, []);

  const handleInput = (val: string) => {
    setQuery(val);
    setActiveIndex(-1);
    clearTimeout(timerRef.current);
    timerRef.current = window.setTimeout(() => doSearch(val), 300);
  };

  const goTo = (path: string) => {
    onClose();
    navigate(path);
  };

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open) onClose();
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open, onClose]);

  if (!presence.shouldRender) return null;

  const hasResults = results && (
    results.models.length
    || results.accounts.length
    || results.accountTokens.length
    || results.sites.length
    || results.checkinLogs.length
    || results.proxyLogs.length
  );

  return (
    <div
      className={`modal-backdrop ${presence.isVisible ? '' : 'is-closing'}`.trim()}
      onClick={onClose}
    >
      <div
        ref={panelRef}
        className={`modal-content search-modal-content ${presence.isVisible ? '' : 'is-closing'}`.trim()}
        role="dialog"
        aria-modal="true"
        aria-label={t('搜索')}
        onClick={e => e.stopPropagation()}
      >
        <div className="search-modal-header">
          <svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="var(--color-text-muted)" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            ref={inputRef}
            value={query}
            onChange={e => handleInput(e.target.value)}
            onKeyDown={handleInputKeyDown}
            placeholder={t('搜索站点、账号、模型、日志...')}
            className="search-modal-input"
            aria-label={t('搜索')}
            role="combobox"
            aria-expanded={hasResults ? 'true' : 'false'}
            aria-controls="search-modal-listbox"
            aria-activedescendant={activeIndex >= 0 ? `search-item-${activeIndex}` : undefined}
          />
          {loading && <span className="spinner spinner-sm" aria-hidden="true" />}
          <kbd className="search-modal-kbd" aria-hidden="true">ESC</kbd>
          <button
            type="button"
            className="modal-close-button"
            onClick={onClose}
            aria-label={t('关闭')}
          >
            ×
          </button>
        </div>

        <div className="search-modal-body" id="search-modal-listbox" role="listbox" aria-label={t('搜索结果')}>
          {query && !loading && flat.length === 0 && (
            <div className="search-modal-empty">
              {t('没有找到匹配结果')}
            </div>
          )}

          {flat.map((item, idx) => {
            const prev = idx > 0 ? flat[idx - 1].sectionKey : null;
            const showTitle = prev !== item.sectionKey;
            return (
              <div key={item.key}>
                {showTitle && (
                  <div className="search-modal-section-title">{t(SECTION_TITLE_KEY[item.sectionKey])}</div>
                )}
                <button
                  ref={el => { itemRefs.current[idx] = el; }}
                  id={`search-item-${idx}`}
                  className="search-result-item"
                  data-active={idx === activeIndex ? 'true' : undefined}
                  aria-selected={idx === activeIndex}
                  onMouseEnter={() => setActiveIndex(idx)}
                  onClick={() => goTo(item.path)}
                >
                  {item.icon}
                  <div>
                    <div style={{ fontWeight: 500 }}>{item.title}</div>
                    <div className="search-result-meta">{item.meta}</div>
                  </div>
                </button>
              </div>
            );
          })}
        </div>

        <div style={{ padding: '8px 16px', borderTop: '1px solid var(--color-border-light)', fontSize: 11, color: 'var(--color-text-muted)', display: 'flex', gap: 12 }}>
          <span><kbd style={{ padding: '1px 4px', background: 'var(--color-bg)', border: '1px solid var(--color-border)', borderRadius: 3 }}>↑↓</kbd> {t('导航')}</span>
          <span><kbd style={{ padding: '1px 4px', background: 'var(--color-bg)', border: '1px solid var(--color-border)', borderRadius: 3 }}>Enter</kbd> {t('打开')}</span>
          <span><kbd style={{ padding: '1px 4px', background: 'var(--color-bg)', border: '1px solid var(--color-border)', borderRadius: 3 }}>Esc</kbd> {t('关闭')}</span>
        </div>
      </div>
    </div>
  );
}
