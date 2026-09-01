# 事件结构化（titleKey + params）设计

> 状态：实施中（2026-09-01）。对应 MASTER 开放项 F5。

## 问题

`events` 表当前存成品英文 `title` + `message`。事件在产生时写死文本，查看者
语言无法改变渲染——中文界面的管理员看到的程序日志/通知永远是英文，且
message 内嵌参数（账号名、站点名、数值）无法重建。

## 目标

新事件在产生时存**结构化标识 + 参数**，渲染时按查看者 locale 翻译；
历史行保持原文 fallback（零迁移，不重写已有行）。

## 架构（三层，单点收敛）

### 1. 后端事件注册表 `service/events/registry.go`

每个事件一个类型化定义（不是散落字符串）：

```go
type Definition struct {
    Key     string          // 稳定 slug：checkinSuccess / tokenExpired / ...
    TitleEn string          // 英文标题 fallback（历史消费方：通知/CSV/邮件）
    Params  []ParamSpec     // 参数 schema：name + 类型（string/int）
    Message string          // 英文 message 模板（{{name}} 插值）
}
```

- 注册表是**事件定义的唯一来源**；新增事件 = 新增一个条目 + 前端 locale 键。
- Key 命名与前端 `events.titles.*` locale 节一一对应（测试钉住两端一致）。

### 2. 统一写入入口 `service/events.WriteEvent`

```go
WriteEvent(ctx, db, Ref{Key, Params}, opts{Level, RelatedID, RelatedType})
```

- 从注册表取定义，按 schema 校验参数（类型/必填），渲染英文 fallback
  title/message 存入现有列（通知/CSV 等历史消费方不感知变化），
  同时写 `title_key` + `params`（JSON）。
- 渐近迁移：现有 `service.CreateEvent` 与 8 处手写 `INSERT INTO events`
  逐个迁移到 WriteEvent；迁移前的老行 `title_key` 为 NULL。

### 3. 前端双路径渲染

- **新行**（titleKey 非空）：`t('events.titles.' + key, params)` —
  i18next 原生插值，双语端到端，不依赖标题文本匹配。
- **历史行**（titleKey 为空）：现有 `eventTitleSlug(title)` 标题匹配
  fallback（`web/src/lib/event-titles.ts` 保留，作为历史反查表）。

## Schema（additive，零迁移）

```sql
ALTER TABLE events ADD COLUMN title_key TEXT NULL;
ALTER TABLE events ADD COLUMN params    TEXT NULL;  -- JSON object
```

- 老行两列 NULL；新行两列写入。无数据回填。
- 前端/API 对两列均容忍缺失。

## 一致性门禁

- 后端测试：枚举 registry 全部 Key。
- 前端测试：`i18n.exists('events.titles.' + key)` 对 registry 全部 Key 钉死
  （沿用动态 i18n 键的存在性测试模式）。
- 两测试从同一 Key 清单生成——Key 漂移在 CI 即断。

## 迁移批次

| 批次 | 事件 | 参数 |
|---|---|---|
| 1（本波） | checkin success / failed / skipped / cloudflare challenge | account, site, reward/reason |
| 2 | token expired / low balance / all proxies failed | account, site, model, reason |
| 3 | token sync ×2 / site disabled/enabled / admin token / runtime settings / disabled model repaired / backup keys | 按语义 |

## 验收

1. 新产生的签到事件在 zh-CN 界面渲染中文标题 + 参数插值；en 界面渲染英文。
2. 历史事件（title_key NULL）仍按原文 + 标题匹配渲染，视觉无变化。
3. 通知/CSV 等非 UI 消费方读到的 title/message 与迁移前一致（英文 fallback 不变）。
