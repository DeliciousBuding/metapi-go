#!/usr/bin/env python3
"""Seed deterministic demo data into a metapi server's SQLite DB so screenshot
evidence (ui-screenshots CI job, local design review) shows a lived-in UI
instead of empty states.

Usage:
    python3 scripts/e2e/seed-demo-data.py /path/to/data/hub.db

The server must have been started at least once against that DATA_DIR so the
schema exists; it can keep running while the seed executes (per-request reads).
The script is idempotent: it wipes and reseeds the demo tables on every run.
Timestamps are relative to "now" (RFC3339 UTC, matching the server's own
writes) so dashboards/checkin/proxy-log pages always look fresh; the row
*shape* is fixed via a constant random seed.

Demo-only data: fake example.com hosts and non-functional tokens. Never point
this at a real deployment.
"""
import json
import random
import sqlite3
import sys
from datetime import datetime, timedelta, timezone

if len(sys.argv) != 2:
    sys.exit(__doc__)
DB = sys.argv[1]
random.seed(20260913)
NOW = datetime.now(timezone.utc)

def ts(dt): return dt.astimezone(timezone.utc).strftime('%Y-%m-%dT%H:%M:%S.') + f"{dt.microsecond//1000:03d}Z"

db = sqlite3.connect(DB)
c = db.cursor()
for t in ['sites','accounts','account_tokens','token_routes','route_channels','downstream_api_keys','proxy_logs','checkin_logs','site_day_usage','site_hour_usage','model_day_usage','model_availability','events','balance_history']:
    c.execute(f'DELETE FROM {t}')

# ── sites ──
sites = [
    (1, 'NewAPI 主站', 'https://api-primary.example.com', 'new-api', 'active', 1, 0),
    (2, 'Sub2API 备用', 'https://sub-backup.example.com', 'sub2api', 'active', 0, 1),
    (3, 'CLIProxy 实验', 'https://cli-lab.example.com', 'cliproxyapi', 'disabled', 0, 2),
]
for sid, name, url, platform, status, pinned, order in sites:
    c.execute("INSERT INTO sites (id,name,url,external_checkin_url,platform,proxy_url,use_system_proxy,custom_headers,custom_headers_override_request_headers,browser_ua,cf_clearance,resin_enabled,use_utls,status,is_pinned,sort_order,global_weight,api_key,max_concurrency,post_refresh_probe_enabled,created_at,updated_at,tags) VALUES (?,?,?,'',?,'',0,'',0,'','',0,0,?,?,?,1,'',0,0,?,?,'')",
              (sid, name, url, platform, status, pinned, order, ts(NOW - timedelta(days=30)), ts(NOW)))

# ── accounts ──
accounts = [
    (1, 1, 'team-prod@corp.dev', 'sess_live_a1', 128.45, 61.20, 0, 'active', 1),
    (2, 1, 'team-staging@corp.dev', 'sess_live_a2', 42.10, 12.66, 0, 'active', 0),
    (3, 2, 'backup-pool-01', 'sess_live_b1', 300.00, 4.50, 0, 'active', 0),
    (4, 2, 'backup-pool-02', 'sess_live_b2', 0.00, 96.30, 0, 'active', 0),
    (5, 3, 'lab-trial', 'sess_live_c1', 5.00, 0.00, 0, 'disabled', 0),
    (6, 1, 'partner-readonly', 'sess_live_a3', 18.90, 2.10, 0, 'active', 0),
]
for aid, sid, uname, tok, bal, used, quota, status, pinned in accounts:
    c.execute("INSERT INTO accounts (id,site_id,username,access_token,api_token,balance,balance_used,quota,unit_cost,value_score,status,is_pinned,sort_order,checkin_enabled,last_checkin_at,last_balance_refresh,oauth_provider,oauth_account_key,oauth_project_id,extra_config,created_at,updated_at,tags,remark) VALUES (?,?,?,?,'',?,?,?,NULL,?,?,?,?,1,?,?,  '','','','',?,?,'','')",
              (aid, sid, uname, tok, bal, used, quota, round(bal/max(used,0.01),2), status, pinned, 0,
               ts(NOW - timedelta(hours=random.randint(2, 20))), ts(NOW - timedelta(minutes=random.randint(5, 90))),
               ts(NOW - timedelta(days=25)), ts(NOW)))

# ── account tokens ──
tid = 0
for aid in [1, 2, 3, 4]:
    for i, name in enumerate(['default', 'ci-runner']):
        tid += 1
        c.execute("INSERT INTO account_tokens (id,account_id,name,token,token_group,value_status,source,enabled,is_default,created_at,updated_at) VALUES (?,?,?,?,'','ready','auto',1,?,?,?)",
                  (tid, aid, name, f'sk-upstream-{aid}{i}x9f2d7c', 1 if i == 0 else 0, ts(NOW - timedelta(days=20)), ts(NOW)))

# ── token routes ──
routes = [
    (1, 'gpt-4o-mini', 'GPT-4o Mini', 'auto'),
    (2, 'claude-3-5-sonnet-*', 'Claude 3.5 Sonnet', 'pattern'),
    (3, 'gpt-4o', 'GPT-4o', 'exact'),
]
for rid, pattern, dname, mode in routes:
    c.execute("INSERT INTO token_routes (id,model_pattern,display_name,display_icon,route_mode,model_mapping,decision_snapshot,decision_refreshed_at,routing_strategy,context_length,sort_order,enabled,created_at,updated_at) VALUES (?,?,?,'',?,'','',?,'weighted',0,?,1,?,?)",
              (rid, pattern, dname, 'pattern' if mode != 'exact' else 'exact', ts(NOW - timedelta(days=2)), rid, ts(NOW - timedelta(days=18)), ts(NOW)))

# ── route channels (route → account/token) ──
channels = [
    (1, 1, 1, 1, 'gpt-4o-mini', 0, 60, 214, 3, 402110, 3.2140),
    (2, 1, 2, 3, 'gpt-4o-mini', 1, 40, 88, 9, 197502, 1.4801),
    (3, 2, 3, 5, 'claude-3-5-sonnet-20241022', 0, 100, 56, 1, 290344, 8.7712),
    (4, 3, 1, 2, 'gpt-4o', 0, 70, 41, 2, 510922, 12.0551),
    (5, 3, 4, 7, 'gpt-4o', 1, 30, 12, 5, 98012, 2.3310),
]
for cid, rid, aid, tk, model, prio, weight, succ, fail, lat, cost in channels:
    c.execute("INSERT INTO route_channels (id,route_id,account_id,token_id,oauth_route_unit_id,source_model,priority,weight,enabled,manual_override,success_count,fail_count,total_latency_ms,total_cost,last_used_at,last_selected_at,last_fail_at,consecutive_fail_count,cooldown_level,cooldown_until,cooldown_reason_code,cooldown_reason,cooldown_reason_at) VALUES (?,?,?,?,0,?,?,?,1,0,?,?,?,?,?,?,?,0,0,'','','','')",
              (cid, rid, aid, tk, model, prio, weight, succ, fail, lat, cost,
               ts(NOW - timedelta(minutes=random.randint(1, 40))), ts(NOW - timedelta(minutes=random.randint(1, 40))),
               ts(NOW - timedelta(hours=random.randint(2, 30))) if fail else ''))

# ── downstream api keys ──
keys = [
    (1, '生产 Web 应用', 'sk-metapi-prod-7f2a9c1e', 'web', 12.0551, 4021),
    (2, 'CI 冒烟', 'sk-metapi-ci-3b8d4e5f', 'ci', 0.8123, 388),
]
for kid, name, key, group, cost, reqs in keys:
    c.execute("INSERT INTO downstream_api_keys (id,name,key,description,group_name,tags,enabled,expires_at,max_cost,used_cost,max_requests,used_requests,supported_models,allowed_route_ids,site_weight_multipliers,excluded_site_ids,excluded_credential_refs,allowed_site_ids,allowed_credential_refs,key_weight,proxy_url,max_rpm,max_tpm,ip_allowlist,ip_blocklist,last_used_at,created_at,updated_at) VALUES (?,?,?,'',?,'',1,'',0,?,0,?,'','','','','','','',1,'',0,0,'','',?,?,?)",
              (kid, name, key, group, cost, reqs, ts(NOW - timedelta(minutes=random.randint(1, 30))), ts(NOW - timedelta(days=15)), ts(NOW)))

# ── proxy logs (~80 over 48h) ──
models = [(1, 1, 'gpt-4o-mini', 'gpt-4o-mini', 0.0004), (1, 2, 'gpt-4o-mini', 'gpt-4o-mini', 0.0004),
          (2, 3, 'claude-3-5-sonnet-*', 'claude-3-5-sonnet-20241022', 0.008), (3, 1, 'gpt-4o', 'gpt-4o', 0.006),
          (3, 4, 'gpt-4o', 'gpt-4o', 0.006)]
lid = 0
for i in range(80):
    rid, aid, mreq, mact, rate = random.choice(models)
    fail = random.random() < 0.09
    status = 'failed' if fail else 'success'
    http = random.choice([500, 429, 502]) if fail else 200
    ptok = random.randint(120, 4200); ctok = 0 if fail else random.randint(40, 900)
    stream = 1 if random.random() < 0.6 else 0
    lat = random.randint(8000, 25000) if fail else random.randint(350, 4200)
    fb = random.randint(120, 900) if stream and not fail else 0
    created = NOW - timedelta(minutes=random.randint(3, 2880))
    lid += 1
    chan = [ch for ch in channels if ch[1] == rid and ch[2] == aid][0][0]
    c.execute("INSERT INTO proxy_logs (id,route_id,channel_id,account_id,downstream_api_key_id,model_requested,model_actual,status,http_status,is_stream,first_byte_latency_ms,latency_ms,prompt_tokens,completion_tokens,total_tokens,estimated_cost,billing_details,client_family,client_app_id,client_app_name,client_confidence,error_message,retry_count,request_id,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'','','','','',?,?,'req-'||printf('%024x',?),?)",
              (lid, rid, chan, aid, random.choice([1, 2]), mreq, mact, status, http, stream, fb, lat, ptok, ctok, ptok + ctok,
               round((ptok + ctok) * rate / 1000, 6),
               'upstream 5xx: connection reset by peer' if fail else '', random.choice([0, 0, 0, 1]), random.getrandbits(60), ts(created)))

# ── checkin logs ──
cid_ = 0
for day in range(6, -1, -1):
    for aid in [1, 2, 3]:
        cid_ += 1
        ok = not (day == 3 and aid == 2)
        c.execute("INSERT INTO checkin_logs (id,account_id,status,message,reward,failure_reason,created_at) VALUES (?,?,?,?,?,?,?)",
                  (cid_, aid, 'success' if ok else 'failed', '', '+$0.50' if ok else '', '' if ok else 'upstream timeout', ts(NOW - timedelta(days=day, hours=2))))

# ── usage aggregates (14d day + 24h hour) ──
uid = 0
for day in range(13, -1, -1):
    d = (NOW - timedelta(days=day)).strftime('%Y-%m-%d')
    for sid in [1, 2]:
        uid += 1
        calls = random.randint(40, 220); failed = int(calls * random.uniform(0.02, 0.1))
        c.execute("INSERT INTO site_day_usage (id,local_day,site_id,total_calls,success_calls,failed_calls,total_tokens,total_summary_spend,total_site_spend,total_latency_ms,latency_count,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)",
                  (uid, d, sid, calls, calls - failed, failed, calls * random.randint(800, 2400), round(calls * 0.021, 4), round(calls * 0.019, 4), calls * 1100, calls, ts(NOW), ts(NOW)))
    for model in ['gpt-4o-mini', 'gpt-4o', 'claude-3-5-sonnet-20241022']:
        uid += 1
        calls = random.randint(10, 120); failed = int(calls * random.uniform(0.0, 0.12))
        c.execute("INSERT INTO model_day_usage (id,local_day,site_id,model,total_calls,success_calls,failed_calls,total_tokens,total_spend,total_latency_ms,latency_count,created_at,updated_at) VALUES (?,?,1,?,?,?,?,?,?,?,?,?,?)",
                  (uid, d, model, calls, calls - failed, failed, calls * random.randint(600, 2000), round(calls * 0.015, 4), calls * 950, calls, ts(NOW), ts(NOW)))
for hour in range(23, -1, -1):
    b = (NOW - timedelta(hours=hour)).strftime('%Y-%m-%dT%H:00:00Z')
    for sid in [1, 2]:
        uid += 1
        calls = random.randint(0, 24); failed = int(calls * random.uniform(0, 0.15))
        c.execute("INSERT INTO site_hour_usage (id,bucket_start_utc,site_id,total_calls,success_calls,failed_calls,total_tokens,total_summary_spend,total_site_spend,total_latency_ms,latency_count,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)",
                  (uid, b, sid, calls, calls - failed, failed, calls * 1400, round(calls * 0.021, 4), round(calls * 0.019, 4), calls * 1200, calls, ts(NOW), ts(NOW)))

# ── model availability ──
mid = 0
for aid, models_list in [(1, ['gpt-4o', 'gpt-4o-mini']), (2, ['gpt-4o-mini']), (3, ['claude-3-5-sonnet-20241022']), (4, ['gpt-4o'])]:
    for m in models_list:
        mid += 1
        c.execute("INSERT INTO model_availability (id,account_id,model_name,available,is_manual,latency_ms,checked_at) VALUES (?,?,?,1,0,?,?)",
                  (mid, aid, m, random.randint(180, 900), ts(NOW - timedelta(minutes=random.randint(2, 50)))))

# ── events (attention feed) ──
c.execute("INSERT INTO events (id,type,title,message,level,read,related_id,related_type,created_at,title_key,params) VALUES (1,'balance','','账户 backup-pool-02 余额不足','warning',0,4,'account',?,'','{}')", (ts(NOW - timedelta(hours=3)),))
c.execute("INSERT INTO events (id,type,title,message,level,read,related_id,related_type,created_at,title_key,params) VALUES (2,'checkin','','账号 team-staging@corp.dev 连续签到失败','error',0,2,'account',?,'','{}')", (ts(NOW - timedelta(days=3)),))

# ── balance history (7d, account 1) ──
for day in range(6, -1, -1):
    c.execute("INSERT INTO balance_history (account_id,balance,balance_used,quota,local_day,captured_at,created_at) VALUES (1,?,?,0,?,?,?)",
                  (round(110 + (6 - day) * 3.1, 2), round(50 + (6 - day) * 1.9, 2),
                   (NOW - timedelta(days=day)).strftime('%Y-%m-%d'), ts(NOW - timedelta(days=day)), ts(NOW - timedelta(days=day))))

db.commit()
counts = {t: c.execute(f'select count(*) from {t}').fetchone()[0] for t in ['sites','accounts','account_tokens','token_routes','route_channels','downstream_api_keys','proxy_logs','checkin_logs','site_day_usage','site_hour_usage','model_day_usage','model_availability','events','balance_history']}
print(json.dumps(counts))
db.close()
