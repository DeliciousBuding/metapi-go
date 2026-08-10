import React, { useEffect, useState } from 'react';
import { api, type RuntimeSettingsPayload } from '../api.js';
import { useToast } from '../components/Toast.js';
import { tr } from '../i18n.js';

type RuntimeSettings = {
    webhookUrl: string;
    barkUrl: string;
    webhookEnabled: boolean;
    barkEnabled: boolean;
    serverChanEnabled: boolean;
    telegramEnabled: boolean;
    telegramApiBaseUrl: string;
    telegramChatId: string;
    telegramUseSystemProxy: boolean;
    telegramMessageThreadId: string;
    smtpEnabled: boolean;
    smtpHost: string;
    smtpPort: number;
    smtpSecure: boolean;
    smtpUser: string;
    smtpPassMasked?: string;
    smtpFrom: string;
    smtpTo: string;
    serverChanKeyMasked?: string;
    telegramBotTokenMasked?: string;
    // extended channels
    feishuEnabled: boolean;
    feishuWebhook: string;
    feishuSecretMasked?: string;
    dingtalkEnabled: boolean;
    dingtalkWebhook: string;
    dingtalkSecretMasked?: string;
    wecomEnabled: boolean;
    wecomWebhook: string;
    ntfyEnabled: boolean;
    ntfyUrl: string;
    ntfyTopic: string;
    ntfyTokenMasked?: string;
    notifyTaskToggles?: Record<string, boolean>;
    notifyCooldownSec: number;
};

export default function NotificationSettings() {
    const [runtime, setRuntime] = useState<RuntimeSettings>({
        webhookUrl: '',
        barkUrl: '',
        webhookEnabled: true,
        barkEnabled: true,
        serverChanEnabled: false,
        telegramEnabled: false,
        telegramApiBaseUrl: 'https://api.telegram.org',
        telegramChatId: '',
        telegramUseSystemProxy: false,
        telegramMessageThreadId: '',
        smtpEnabled: false,
        smtpHost: '',
        smtpPort: 587,
        smtpSecure: false,
        smtpUser: '',
        smtpFrom: '',
        smtpTo: '',
        feishuEnabled: false,
        feishuWebhook: '',
        dingtalkEnabled: false,
        dingtalkWebhook: '',
        wecomEnabled: false,
        wecomWebhook: '',
        ntfyEnabled: false,
        ntfyUrl: '',
        ntfyTopic: '',
        notifyTaskToggles: {},
        notifyCooldownSec: 300,
    });

    const [serverChanKey, setServerChanKey] = useState('');
    const [telegramBotToken, setTelegramBotToken] = useState('');
    const [smtpPass, setSmtpPass] = useState('');
    // secret inputs (only sent when user types a new value)
    const [feishuSecret, setFeishuSecret] = useState('');
    const [dingtalkSecret, setDingtalkSecret] = useState('');
    const [ntfyToken, setNtfyToken] = useState('');
    const [loading, setLoading] = useState(true);
    const [savingNotify, setSavingNotify] = useState(false);
    const [testingNotify, setTestingNotify] = useState(false);
    const toast = useToast();

    const inputStyle: React.CSSProperties = {
        width: '100%',
        padding: '10px 14px',
        border: '1px solid var(--color-border)',
        borderRadius: 'var(--radius-sm)',
        fontSize: 13,
        outline: 'none',
        background: 'var(--color-bg)',
        color: 'var(--color-text-primary)',
        transition: 'border-color 0.2s',
    };

    const loadSettings = async () => {
        setLoading(true);
        try {
            const runtimeInfo = await api.getRuntimeSettings();
            setRuntime({
                webhookUrl: runtimeInfo.webhookUrl || '',
                barkUrl: runtimeInfo.barkUrl || '',
                webhookEnabled: runtimeInfo.webhookEnabled ?? true,
                barkEnabled: runtimeInfo.barkEnabled ?? true,
                serverChanEnabled: !!runtimeInfo.serverChanEnabled,
                telegramEnabled: !!runtimeInfo.telegramEnabled,
                telegramApiBaseUrl: runtimeInfo.telegramApiBaseUrl || 'https://api.telegram.org',
                telegramChatId: runtimeInfo.telegramChatId || '',
                telegramUseSystemProxy: !!runtimeInfo.telegramUseSystemProxy,
                telegramMessageThreadId: runtimeInfo.telegramMessageThreadId || '',
                smtpEnabled: !!runtimeInfo.smtpEnabled,
                smtpHost: runtimeInfo.smtpHost || '',
                smtpPort: Number(runtimeInfo.smtpPort) || 587,
                smtpSecure: !!runtimeInfo.smtpSecure,
                smtpUser: runtimeInfo.smtpUser || '',
                smtpPassMasked: runtimeInfo.smtpPassMasked || '',
                smtpFrom: runtimeInfo.smtpFrom || '',
                smtpTo: runtimeInfo.smtpTo || '',
                serverChanKeyMasked: runtimeInfo.serverChanKeyMasked || '',
                telegramBotTokenMasked: runtimeInfo.telegramBotTokenMasked || '',
                feishuEnabled: !!runtimeInfo.feishuEnabled,
                feishuWebhook: runtimeInfo.feishuWebhook || '',
                feishuSecretMasked: runtimeInfo.feishuSecretMasked || '',
                dingtalkEnabled: !!runtimeInfo.dingtalkEnabled,
                dingtalkWebhook: runtimeInfo.dingtalkWebhook || '',
                dingtalkSecretMasked: runtimeInfo.dingtalkSecretMasked || '',
                wecomEnabled: !!runtimeInfo.wecomEnabled,
                wecomWebhook: runtimeInfo.wecomWebhook || '',
                ntfyEnabled: !!runtimeInfo.ntfyEnabled,
                ntfyUrl: runtimeInfo.ntfyUrl || '',
                ntfyTopic: runtimeInfo.ntfyTopic || '',
                ntfyTokenMasked: runtimeInfo.ntfyTokenMasked || '',
                notifyTaskToggles: (runtimeInfo.notifyTaskToggles as Record<string, boolean>) || {},
                notifyCooldownSec: Number.isFinite(Number(runtimeInfo.notifyCooldownSec))
                    ? Math.max(0, Math.trunc(Number(runtimeInfo.notifyCooldownSec)))
                    : 300,
            });
        } catch (err: any) {
            toast.error(err?.message || '加载通知设置失败');
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadSettings();
    }, []);

    const saveNotify = async () => {
        // 启用但凭据空的渠道 —— 拦截（2026-08-01 生产发现：三渠道 enabled 但
        // 凭据空，签到失败/token 过期告警静默丢失——让正确状态成为唯一可保存状态）
        const missingCreds: string[] = [];
        if (runtime.webhookEnabled && !runtime.webhookUrl.trim()) missingCreds.push('Webhook URL');
        if (runtime.barkEnabled && !runtime.barkUrl.trim()) missingCreds.push('Bark URL');
        if (runtime.serverChanEnabled && !serverChanKey.trim() && !runtime.serverChanKeyMasked) missingCreds.push('Server酱 SendKey');
        if (runtime.telegramEnabled && !telegramBotToken.trim() && !runtime.telegramBotTokenMasked) missingCreds.push('Telegram Bot Token');
        if (runtime.smtpEnabled && (!runtime.smtpHost.trim() || !runtime.smtpFrom.trim() || !runtime.smtpTo.trim())) missingCreds.push('SMTP 服务器 / 发件人 / 收件人');
        if (runtime.feishuEnabled && !runtime.feishuWebhook.trim()) missingCreds.push('飞书 Webhook');
        if (runtime.dingtalkEnabled && !runtime.dingtalkWebhook.trim()) missingCreds.push('钉钉 Webhook');
        if (runtime.wecomEnabled && !runtime.wecomWebhook.trim()) missingCreds.push('企业微信 Webhook');
        if (runtime.ntfyEnabled && (!runtime.ntfyUrl.trim() || !runtime.ntfyTopic.trim())) missingCreds.push('Ntfy URL / Topic');
        if (missingCreds.length > 0) {
            toast.error(`已启用但缺少配置：${missingCreds.join('、')}`);
            return;
        }
        setSavingNotify(true);
        try {
            const payload: RuntimeSettingsPayload = {
                webhookUrl: runtime.webhookUrl,
                barkUrl: runtime.barkUrl,
                webhookEnabled: runtime.webhookEnabled,
                barkEnabled: runtime.barkEnabled,
                serverChanEnabled: runtime.serverChanEnabled,
                telegramEnabled: runtime.telegramEnabled,
                telegramApiBaseUrl: runtime.telegramApiBaseUrl,
                telegramChatId: runtime.telegramChatId,
                telegramUseSystemProxy: runtime.telegramUseSystemProxy,
                telegramMessageThreadId: runtime.telegramMessageThreadId,
                smtpEnabled: runtime.smtpEnabled,
                smtpHost: runtime.smtpHost,
                smtpPort: runtime.smtpPort,
                smtpSecure: runtime.smtpSecure,
                smtpUser: runtime.smtpUser,
                smtpFrom: runtime.smtpFrom,
                smtpTo: runtime.smtpTo,
                feishuEnabled: runtime.feishuEnabled,
                feishuWebhook: runtime.feishuWebhook,
                dingtalkEnabled: runtime.dingtalkEnabled,
                dingtalkWebhook: runtime.dingtalkWebhook,
                wecomEnabled: runtime.wecomEnabled,
                wecomWebhook: runtime.wecomWebhook,
                ntfyEnabled: runtime.ntfyEnabled,
                ntfyUrl: runtime.ntfyUrl,
                ntfyTopic: runtime.ntfyTopic,
                notifyTaskToggles: runtime.notifyTaskToggles,
                notifyCooldownSec: Math.max(0, Math.trunc(Number(runtime.notifyCooldownSec) || 0)),
            };
            if (serverChanKey.trim()) payload.serverChanKey = serverChanKey.trim();
            if (telegramBotToken.trim()) payload.telegramBotToken = telegramBotToken.trim();
            if (smtpPass.trim()) payload.smtpPass = smtpPass.trim();
            // secrets only sent when user types a fresh value
            if (feishuSecret.trim()) payload.feishuSecret = feishuSecret.trim();
            if (dingtalkSecret.trim()) payload.dingtalkSecret = dingtalkSecret.trim();
            if (ntfyToken.trim()) payload.ntfyToken = ntfyToken.trim();

            const res = await api.updateRuntimeSettings(payload);
            setRuntime((prev) => ({
                ...prev,
                serverChanKeyMasked: res.serverChanKeyMasked || prev.serverChanKeyMasked,
                telegramBotTokenMasked: res.telegramBotTokenMasked || prev.telegramBotTokenMasked,
                smtpPassMasked: res.smtpPassMasked || prev.smtpPassMasked,
                feishuSecretMasked: res.feishuSecretMasked || prev.feishuSecretMasked,
                dingtalkSecretMasked: res.dingtalkSecretMasked || prev.dingtalkSecretMasked,
                ntfyTokenMasked: res.ntfyTokenMasked || prev.ntfyTokenMasked,
            }));
            setServerChanKey('');
            setTelegramBotToken('');
            setSmtpPass('');
            setFeishuSecret('');
            setDingtalkSecret('');
            setNtfyToken('');
            toast.success('通知设置已保存');
        } catch (err: any) {
            toast.error(err?.message || '保存失败');
        } finally {
            setSavingNotify(false);
        }
    };

    const testNotify = async () => {
        setTestingNotify(true);
        try {
            const res = await api.testNotification();
            toast.success(res?.message || '测试通知已发送');
        } catch (err: any) {
            toast.error(err?.message || '触发测试通知失败');
        } finally {
            setTestingNotify(false);
        }
    };

    if (loading) {
        return (
            <div className="animate-fade-in">
                <div className="skeleton" style={{ width: 220, height: 28, marginBottom: 20 }} />
                <div className="skeleton" style={{ width: '100%', height: 320, borderRadius: 'var(--radius-sm)' }} />
            </div>
        );
    }

    return (
        <div className="animate-fade-in" style={{ paddingBottom: 40 }}>
            {/* 头部标题与操作 */}
            <div className="page-header">
                <h2 className="page-title">{tr('通知设置')}</h2>
                <div className="page-actions">
                    <button onClick={testNotify} disabled={testingNotify} className="btn btn-success">
                        {testingNotify ? <><span className="spinner spinner-sm" /> 发送中...</> : '发送测试通知'}
                    </button>
                    <button onClick={saveNotify} disabled={savingNotify} className="btn btn-primary">
                        {savingNotify ? <><span className="spinner spinner-sm" /> 保存中...</> : '保存通知设置'}
                    </button>
                </div>
            </div>

            <div style={{ maxWidth: 860, display: 'flex', flexDirection: 'column', gap: 20 }}>

                <div className="card animate-slide-up stagger-1" style={{ padding: 20 }}>
                    <div style={{ fontWeight: 600, fontSize: 15, marginBottom: 8 }}>{tr('告警去噪与冷静期')}</div>
                    <div style={{ fontSize: 12, color: 'var(--color-text-muted)', marginBottom: 12 }}>
                        相同告警在冷静期内不会重复推送；冷静期结束后会自动合并重复条数。
                    </div>
                    <div style={{ maxWidth: 260 }}>
                        <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>{tr('冷静期（秒）')}</div>
                        <input
                            type="number"
                            min={0}
                            value={runtime.notifyCooldownSec}
                            onChange={(e) => setRuntime((prev) => ({
                                ...prev,
                                notifyCooldownSec: Math.max(0, Math.trunc(Number(e.target.value) || 0)),
                            }))}
                            style={inputStyle}
                        />
                    </div>
                </div>

                {/* 卡片：Webhook & Bark */}
                <div className="card animate-slide-up stagger-2" style={{ padding: 24, border: (runtime.webhookEnabled || runtime.barkEnabled) ? '1px solid var(--color-primary)' : '1px solid var(--color-border-light)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--color-primary-light)', color: 'var(--color-primary)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                                <svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" /></svg>
                            </div>
                            <div>
                                <div style={{ fontWeight: 600, fontSize: 15 }}>Webhook & Bark</div>
                                <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>通过 HTTP URL 推送消息通知（自动识别企业微信、飞书格式）</div>
                            </div>
                        </div>

                        <div style={{ display: 'flex', gap: 16 }}>
                            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
                                <span style={{ fontSize: 13, fontWeight: 500, color: runtime.webhookEnabled ? 'var(--color-primary)' : 'var(--color-text-muted)' }}>启用 Webhook</span>
                                <input
                                    type="checkbox"
                                    style={{ width: 16, height: 16, cursor: 'pointer' }}
                                    checked={runtime.webhookEnabled}
                                    onChange={(e) => setRuntime((prev) => ({ ...prev, webhookEnabled: e.target.checked }))}
                                />
                            </label>
                            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
                                <span style={{ fontSize: 13, fontWeight: 500, color: runtime.barkEnabled ? 'var(--color-primary)' : 'var(--color-text-muted)' }}>启用 Bark</span>
                                <input
                                    type="checkbox"
                                    style={{ width: 16, height: 16, cursor: 'pointer' }}
                                    checked={runtime.barkEnabled}
                                    onChange={(e) => setRuntime((prev) => ({ ...prev, barkEnabled: e.target.checked }))}
                                />
                            </label>
                        </div>
                    </div>

                    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                        <div style={{ opacity: runtime.webhookEnabled ? 1 : 0.6, transition: 'opacity 0.2s' }}>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>Webhook URL</div>
                            <input
                                value={runtime.webhookUrl}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, webhookUrl: e.target.value }))}
                                placeholder="https://your-webhook-url (可选)"
                                style={inputStyle}
                                disabled={!runtime.webhookEnabled}
                            />
                        </div>
                        <div style={{ opacity: runtime.barkEnabled ? 1 : 0.6, transition: 'opacity 0.2s' }}>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>Bark URL</div>
                            <input
                                value={runtime.barkUrl}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, barkUrl: e.target.value }))}
                                placeholder="https://api.day.app/your_key (可选)"
                                style={inputStyle}
                                disabled={!runtime.barkEnabled}
                            />
                        </div>
                    </div>
                </div>

                {/* 卡片：Server酱 */}
                <div className="card animate-slide-up stagger-3" style={{ padding: 24, border: runtime.serverChanEnabled ? '1px solid var(--color-primary)' : '1px solid var(--color-border-light)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--color-warning-soft)', color: 'var(--color-warning)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                                <svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" /></svg>
                            </div>
                            <div>
                                <div style={{ fontWeight: 600, fontSize: 15 }}>Server酱 (SendKey)</div>
                                <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('微信推送消息支持')}</div>
                            </div>
                        </div>

                        <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
                            <span style={{ fontSize: 13, fontWeight: 500, color: runtime.serverChanEnabled ? 'var(--color-primary)' : 'var(--color-text-muted)' }}>启用 Server酱</span>
                            <input
                                type="checkbox"
                                style={{ width: 16, height: 16, cursor: 'pointer' }}
                                checked={runtime.serverChanEnabled}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, serverChanEnabled: e.target.checked }))}
                            />
                        </label>
                    </div>

                    <div style={{ opacity: runtime.serverChanEnabled ? 1 : 0.6, transition: 'opacity 0.2s' }}>
                        <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 10 }}>
                            <span style={{ fontSize: 13, color: 'var(--color-text-secondary)' }}>{tr('当前配置: ')}</span>
                            <code style={{ padding: '4px 10px', background: 'var(--color-bg)', borderRadius: 'var(--radius-sm)', fontSize: 13, fontFamily: 'var(--font-mono)', color: 'var(--color-text-secondary)', border: '1px solid var(--color-border-light)' }}>
                                {runtime.serverChanKeyMasked || tr('未设置')}
                            </code>
                        </div>
                        <input
                            type="password"
                            value={serverChanKey}
                            onChange={(e) => setServerChanKey(e.target.value)}
                            placeholder="输入新的 Server酱 Key（留空则不改）"
                            style={inputStyle}
                            disabled={!runtime.serverChanEnabled}
                        />
                    </div>
                </div>

                {/* 卡片：Telegram */}
                <div className="card animate-slide-up stagger-4" style={{ padding: 24, border: runtime.telegramEnabled ? '1px solid var(--color-primary)' : '1px solid var(--color-border-light)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--color-primary-light)', color: 'var(--color-primary)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                                <svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 11l18-8-6 18-3-7-9-3z" /></svg>
                            </div>
                            <div>
                                <div style={{ fontWeight: 600, fontSize: 15 }}>Telegram Bot</div>
                                <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>通过 Telegram 机器人推送消息通知</div>
                            </div>
                        </div>

                        <div style={{ display: 'flex', gap: 16 }}>
                            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
                                <span style={{ fontSize: 13, fontWeight: 500, color: runtime.telegramUseSystemProxy ? 'var(--color-primary)' : 'var(--color-text-muted)' }}>{tr('使用系统代理')}</span>
                                <input
                                    type="checkbox"
                                    style={{ width: 16, height: 16, cursor: 'pointer' }}
                                    checked={runtime.telegramUseSystemProxy}
                                    onChange={(e) => setRuntime((prev) => ({ ...prev, telegramUseSystemProxy: e.target.checked }))}
                                />
                            </label>
                            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
                                <span style={{ fontSize: 13, fontWeight: 500, color: runtime.telegramEnabled ? 'var(--color-primary)' : 'var(--color-text-muted)' }}>启用 Telegram</span>
                                <input
                                    type="checkbox"
                                    style={{ width: 16, height: 16, cursor: 'pointer' }}
                                    checked={runtime.telegramEnabled}
                                    onChange={(e) => setRuntime((prev) => ({ ...prev, telegramEnabled: e.target.checked }))}
                                />
                            </label>
                        </div>
                    </div>

                    <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)', gap: '16px 20px', opacity: runtime.telegramEnabled ? 1 : 0.6, transition: 'opacity 0.2s' }}>
                        <div style={{ gridColumn: '1 / -1' }}>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>Telegram API Base URL</div>
                            <input
                                value={runtime.telegramApiBaseUrl}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, telegramApiBaseUrl: e.target.value }))}
                                placeholder="例如: https://your-proxy.example.com"
                                style={inputStyle}
                                disabled={!runtime.telegramEnabled}
                            />
                            <div style={{ marginTop: 8, fontSize: 12, color: 'var(--color-text-muted)' }}>
                                留空或使用默认值时直连官方 Telegram API；如需国内反代，可填写反代前缀。
                            </div>
                        </div>
                        <div>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>Telegram Chat ID</div>
                            <input
                                value={runtime.telegramChatId}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, telegramChatId: e.target.value }))}
                                placeholder="例如: -1001234567890 或 @your_channel"
                                style={inputStyle}
                                disabled={!runtime.telegramEnabled}
                            />
                        </div>
                        <div>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>Telegram Topic ID</div>
                            <input
                                value={runtime.telegramMessageThreadId}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, telegramMessageThreadId: e.target.value }))}
                                placeholder="例如: 77"
                                style={inputStyle}
                                disabled={!runtime.telegramEnabled}
                            />
                        </div>
                        <div>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>
                                Telegram Bot Token
                                {runtime.telegramBotTokenMasked && <span style={{ color: 'var(--color-primary)', marginLeft: 8, fontSize: 12 }}>(当前已设置)</span>}
                            </div>
                            <input
                                type="password"
                                value={telegramBotToken}
                                onChange={(e) => setTelegramBotToken(e.target.value)}
                                placeholder="输入新的 Bot Token（留空则不改）"
                                style={inputStyle}
                                disabled={!runtime.telegramEnabled}
                            />
                        </div>
                    </div>
                </div>

                {/* 卡片：SMTP 邮件设置 */}
                <div className="card animate-slide-up stagger-4" style={{ padding: 24, border: runtime.smtpEnabled ? '1px solid var(--color-primary)' : '1px solid var(--color-border-light)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--color-primary-light)', color: 'var(--color-primary)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                                <svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>
                            </div>
                            <div>
                                <div style={{ fontWeight: 600, fontSize: 15 }}>邮件服务 (SMTP)</div>
                                <div style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>{tr('通过电子邮件推送提醒')}</div>
                            </div>
                        </div>

                        <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
                            <span style={{ fontSize: 13, fontWeight: 500, color: runtime.smtpEnabled ? 'var(--color-primary)' : 'var(--color-text-muted)' }}>启用 SMTP</span>
                            <input
                                type="checkbox"
                                style={{ width: 16, height: 16, cursor: 'pointer' }}
                                checked={runtime.smtpEnabled}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, smtpEnabled: e.target.checked }))}
                            />
                        </label>
                    </div>

                    <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)', gap: '16px 20px', opacity: runtime.smtpEnabled ? 1 : 0.6, transition: 'opacity 0.2s' }}>
                        {/* Host */}
                        <div>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>SMTP 服务器</div>
                            <input
                                value={runtime.smtpHost}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, smtpHost: e.target.value }))}
                                placeholder="例如: smtp.qq.com"
                                style={inputStyle}
                                disabled={!runtime.smtpEnabled}
                            />
                        </div>
                        {/* Port & Secure */}
                        <div style={{ display: 'flex', gap: 16, alignItems: 'flex-end' }}>
                            <div style={{ flex: 1 }}>
                                <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>{tr('端口')}</div>
                                <input
                                    type="number"
                                    min={1}
                                    value={runtime.smtpPort}
                                    onChange={(e) => setRuntime((prev) => ({ ...prev, smtpPort: Number(e.target.value) || 0 }))}
                                    style={inputStyle}
                                    disabled={!runtime.smtpEnabled}
                                />
                            </div>
                            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: 'var(--color-text-secondary)', paddingBottom: 12 }}>
                                <input
                                    type="checkbox"
                                    checked={runtime.smtpSecure}
                                    onChange={(e) => setRuntime((prev) => ({ ...prev, smtpSecure: e.target.checked }))}
                                    disabled={!runtime.smtpEnabled}
                                />
                                启用 TLS/SSL
                            </label>
                        </div>
                        {/* User */}
                        <div>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>{tr('账号用户')}</div>
                            <input
                                value={runtime.smtpUser}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, smtpUser: e.target.value }))}
                                placeholder="SMTP 用户名"
                                style={inputStyle}
                                disabled={!runtime.smtpEnabled}
                            />
                        </div>
                        {/* Pass */}
                        <div>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>
                                账号密码
                                {runtime.smtpPassMasked && <span style={{ color: 'var(--color-primary)', marginLeft: 8, fontSize: 12 }}>(当前已设置)</span>}
                            </div>
                            <input
                                type="password"
                                value={smtpPass}
                                onChange={(e) => setSmtpPass(e.target.value)}
                                placeholder="输入以更改密码..."
                                style={inputStyle}
                                disabled={!runtime.smtpEnabled}
                            />
                        </div>
                        {/* From */}
                        <div>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>{tr('发件人地址')}</div>
                            <input
                                value={runtime.smtpFrom}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, smtpFrom: e.target.value }))}
                                placeholder="例如: admin@example.com"
                                style={inputStyle}
                                disabled={!runtime.smtpEnabled}
                            />
                        </div>
                        {/* To */}
                        <div>
                            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>{tr('接收地址')}</div>
                            <input
                                value={runtime.smtpTo}
                                onChange={(e) => setRuntime((prev) => ({ ...prev, smtpTo: e.target.value }))}
                                placeholder="例如: target@example.com"
                                style={inputStyle}
                                disabled={!runtime.smtpEnabled}
                            />
                        </div>

                    </div>
                </div>

                {/* extended channels (Feishu/DingTalk/WeCom/Ntfy) + per-task mute */}
                <div className="card animate-slide-up stagger-5" style={{ padding: 24, border: (runtime.feishuEnabled || runtime.dingtalkEnabled || runtime.wecomEnabled || runtime.ntfyEnabled) ? '1px solid var(--color-primary)' : '1px solid var(--color-border-light)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
                        <div>
                            <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-text-primary)' }}>{tr('扩展渠道')}</div>
                            <div style={{ fontSize: 12, color: 'var(--color-text-secondary)', marginTop: 2 }}>飞书 / 钉钉 / 企业微信 / Ntfy（可选签名验证）</div>
                        </div>
                    </div>

                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
                        {/* Feishu */}
                        <div style={{ opacity: runtime.feishuEnabled ? 1 : 0.6, transition: 'opacity 0.2s' }}>
                            <label style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8, fontSize: 13, fontWeight: 500, color: 'var(--color-text-secondary)' }}>
                                <input type="checkbox" checked={runtime.feishuEnabled} onChange={(e) => setRuntime((prev) => ({ ...prev, feishuEnabled: e.target.checked }))} />{tr('飞书')}</label>
                            <input value={runtime.feishuWebhook} onChange={(e) => setRuntime((prev) => ({ ...prev, feishuWebhook: e.target.value }))} placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..." style={inputStyle} disabled={!runtime.feishuEnabled} />
                            <input value={feishuSecret} onChange={(e) => setFeishuSecret(e.target.value)} placeholder={runtime.feishuSecretMasked ? `已配置（${runtime.feishuSecretMasked}）` : '签名密钥（可选）'} style={{ ...inputStyle, marginTop: 8 }} disabled={!runtime.feishuEnabled} />
                        </div>
                        {/* DingTalk */}
                        <div style={{ opacity: runtime.dingtalkEnabled ? 1 : 0.6, transition: 'opacity 0.2s' }}>
                            <label style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8, fontSize: 13, fontWeight: 500, color: 'var(--color-text-secondary)' }}>
                                <input type="checkbox" checked={runtime.dingtalkEnabled} onChange={(e) => setRuntime((prev) => ({ ...prev, dingtalkEnabled: e.target.checked }))} />{tr('钉钉')}</label>
                            <input value={runtime.dingtalkWebhook} onChange={(e) => setRuntime((prev) => ({ ...prev, dingtalkWebhook: e.target.value }))} placeholder="https://oapi.dingtalk.com/robot/send?access_token=..." style={inputStyle} disabled={!runtime.dingtalkEnabled} />
                            <input value={dingtalkSecret} onChange={(e) => setDingtalkSecret(e.target.value)} placeholder={runtime.dingtalkSecretMasked ? `已配置（${runtime.dingtalkSecretMasked}）` : '加签密钥（可选）'} style={{ ...inputStyle, marginTop: 8 }} disabled={!runtime.dingtalkEnabled} />
                        </div>
                        {/* WeCom */}
                        <div style={{ opacity: runtime.wecomEnabled ? 1 : 0.6, transition: 'opacity 0.2s' }}>
                            <label style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8, fontSize: 13, fontWeight: 500, color: 'var(--color-text-secondary)' }}>
                                <input type="checkbox" checked={runtime.wecomEnabled} onChange={(e) => setRuntime((prev) => ({ ...prev, wecomEnabled: e.target.checked }))} />{tr('企业微信')}</label>
                            <input value={runtime.wecomWebhook} onChange={(e) => setRuntime((prev) => ({ ...prev, wecomWebhook: e.target.value }))} placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..." style={inputStyle} disabled={!runtime.wecomEnabled} />
                        </div>
                        {/* Ntfy */}
                        <div style={{ opacity: runtime.ntfyEnabled ? 1 : 0.6, transition: 'opacity 0.2s' }}>
                            <label style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8, fontSize: 13, fontWeight: 500, color: 'var(--color-text-secondary)' }}>
                                <input type="checkbox" checked={runtime.ntfyEnabled} onChange={(e) => setRuntime((prev) => ({ ...prev, ntfyEnabled: e.target.checked }))} />
                                Ntfy
                            </label>
                            <input value={runtime.ntfyUrl} onChange={(e) => setRuntime((prev) => ({ ...prev, ntfyUrl: e.target.value }))} placeholder="https://ntfy.sh" style={inputStyle} disabled={!runtime.ntfyEnabled} />
                            <input value={runtime.ntfyTopic} onChange={(e) => setRuntime((prev) => ({ ...prev, ntfyTopic: e.target.value }))} placeholder="topic 名称" style={{ ...inputStyle, marginTop: 8 }} disabled={!runtime.ntfyEnabled} />
                            <input value={ntfyToken} onChange={(e) => setNtfyToken(e.target.value)} placeholder={runtime.ntfyTokenMasked ? `已配置（${runtime.ntfyTokenMasked}）` : '访问令牌（可选）'} style={{ ...inputStyle, marginTop: 8 }} disabled={!runtime.ntfyEnabled} />
                        </div>
                    </div>

                    {/* per-task mute toggles */}
                    <div style={{ marginTop: 20, paddingTop: 16, borderTop: '1px solid var(--color-border-light)' }}>
                        <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 8, color: 'var(--color-text-secondary)' }}>按告警类型静音（关闭后该类告警不再推送，但仍记入事件表）</div>
                        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 16 }}>
                            {[
                                { key: 'token_expired', label: 'Token 失效' },
                                { key: 'low_balance', label: '余额不足' },
                                { key: 'proxy_all_failed', label: '代理全部失败' },
                            ].map((task) => {
                                const enabled = runtime.notifyTaskToggles?.[task.key] ?? true;
                                return (
                                    <label key={task.key} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--color-text-secondary)' }}>
                                        <input
                                            type="checkbox"
                                            checked={enabled}
                                            onChange={(e) => setRuntime((prev) => ({
                                                ...prev,
                                                notifyTaskToggles: { ...(prev.notifyTaskToggles || {}), [task.key]: e.target.checked },
                                            }))}
                                        />
                                        {task.label}
                                    </label>
                                );
                            })}
                        </div>
                    </div>
                </div>

            </div>
        </div>
    );
}
