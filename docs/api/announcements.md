# Announcements & Events

> **Index**: back to [API Reference](../api.md). This file is the Site announcements, operator banners & system events domain split out of the pre-`docs/api/` `docs/api.md`.

## Site Announcements

### GET /api/site-announcements

List site announcements.

### POST /api/site-announcements/{id}/read

Mark one site announcement as read.

### POST /api/site-announcements/read-all

Mark all site announcements as read.

### POST /api/site-announcements/sync

Sync site announcements from configured sites.

### DELETE /api/site-announcements

Delete all site announcements.

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
