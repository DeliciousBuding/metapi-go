# Announcements & Events

> **Index**: back to [API Reference](../api.md). This file is the Site announcements, operator banners & system events domain split out of the pre-`docs/api/` `docs/api.md`.

## Site Announcements

### GET /api/site-announcements

返回当前公告的 JSON 数组，按当前版本的 `firstSeenAt`、`id` 降序排列。
支持 `limit`、`offset`、`siteId`、`platform`、`read=true|false`、
`status=active|expired|dismissed`。筛选与分页作用于当前公告，历史版本不会占用列表行。
`revisionCount` 是历史版本数；点击公告行可就地展开 Markdown 正文与更新时间线。
历史接口在首次展开时请求，正文支持标题、链接、代码块与表格，HTML 不执行。

New API / DoneHub 的单条站点通知按站点合并，独立公告按上游公告 ID 区分。
标题、正文或级别变化会保存旧版本、重置已读和忽略状态，并生成更新通知；
重复抓取只刷新 `lastSeenAt`，上游时间、链接等元数据变化不产生新版本。
`firstSeenAt` 表示当前版本首次被观察的时间，`lastSeenAt` 表示最后被观察的时间。

### GET /api/site-announcements/{id}/history

返回公告版本数组：当前版本在前，历史版本按保存顺序倒序。
每个版本保留标题、正文、级别、来源及发现时间。内容恢复为旧值也会保留一次新更新。
公告不存在返回 404，历史读取或解析失败返回 500。

### POST /api/site-announcements/{id}/read

Mark one site announcement as read.

### POST /api/site-announcements/read-all

Mark all site announcements as read.

### POST /api/site-announcements/sync

Sync site announcements from configured sites.

### DELETE /api/site-announcements

删除全部当前公告及其历史版本。

---

## Announcements

Operator-authored severity-ranked product banners (替代邮件群发). The dashboard renders the `active` list; editing content resets the dismissal so a new revision surfaces again (dismiss-revision semantics).

### GET /api/announcements, GET /api/announcements/active

Admin list (all announcements with dismissal state) and dashboard list (enabled and not dismissed).

**Response**: `{ items: [{ id, title, message, severity, link, enabled, dismissed, dismissedAt, createdAt, updatedAt }] }` — `active` filters `enabled = TRUE` in SQL and drops dismissed rows in Go; both sort by severity (`critical` first) then updatedAt descending.

### POST /api/announcements, PUT /api/announcements/{id}, DELETE /api/announcements/{id}

Create, update, delete an announcement.

**Body**: `{ "title": "...", "message": "...", "severity": "warning", "link": "https://...", "enabled": true }` — `title`/`message` required; `severity` is `info` (default) | `warning` | `critical`.
**Response**: create `{ success, items }` (reloaded list); update `{ success, revision }` where `revision: true` when the content changed (dismissal reset); delete `{ "success": true }` (404 for unknown id).

### POST /api/announcements/{id}/dismiss

Record a dismissal for the current operator (upsert). `{ "success": true }`; 404 for unknown id.

---

## Events

### GET /api/events

List system events (paginated). **Query params**: `page`, `limit`, `level`.

### GET /api/events/count

Get count of unread events.

### POST /api/events/read-all

Mark all events as read.

### POST /api/events/{id}/read

Mark a single event as read.

### DELETE /api/events

Delete all system events. `{ "success": true }`.

---
