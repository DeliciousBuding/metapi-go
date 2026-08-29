# Check-in

> **Index**: back to [API Reference](../api.md). This file is the Check-in trigger, logs & schedule domain split out of the pre-`docs/api/` `docs/api.md`.

## Checkin

### POST /api/checkin/trigger

Trigger manual checkin for all accounts.

### POST /api/checkin/trigger/{id}

Trigger manual checkin for a single account.

### GET /api/checkin/logs

List checkin execution logs.

### PUT /api/checkin/schedule

Update checkin schedule (cron, interval, or random-window mode). The handler keeps the legacy fields and `checkin_schedule_v2` mirror synchronized.

---
