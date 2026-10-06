-- Legacy-shaped sites table translated from store/testdata/ts-source/hub.db.
-- Keep the TS-era columns needed by the current read path, but leave out
-- additive Go columns so first server startup must upgrade this PG table.
CREATE TABLE sites (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    platform TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    api_key TEXT,
    created_at TEXT,
    updated_at TEXT,
    is_pinned BOOLEAN DEFAULT FALSE,
    sort_order INTEGER DEFAULT 0,
    proxy_url TEXT,
    use_system_proxy BOOLEAN DEFAULT FALSE,
    custom_headers TEXT,
    external_checkin_url TEXT,
    global_weight DOUBLE PRECISION DEFAULT 1,
    post_refresh_probe_enabled BOOLEAN DEFAULT FALSE,
    post_refresh_probe_model TEXT DEFAULT '',
    post_refresh_probe_scope TEXT DEFAULT 'single',
    post_refresh_probe_latency_threshold_ms INTEGER DEFAULT 0,
    CONSTRAINT sites_platform_url_unique UNIQUE (platform, url)
);

-- An explicit ID also exercises sequence reconciliation on takeover.
INSERT INTO sites (id, name, url, platform, created_at, updated_at)
VALUES (41, 'pg-legacy-site', 'https://legacy.example.invalid', 'openai',
        '2026-01-01 12:34:56', '2026-01-01 12:34:56');
