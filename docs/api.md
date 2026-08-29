# Admin API Reference

**Last updated**: 2026-08-30
**Index**: the API reference is split by domain into [`api/`](api/) (one file per domain, table below).
Every original heading of the pre-split file is preserved below as a stub pointing to its new home, so existing deep links (`api.md#<anchor>`) keep resolving.

Base URL: `http://localhost:4000/api`. Authentication model, response formats, request-body rules, billing, security notes, CORS and trusted-IP conventions: [docs/api/conventions.md](api/conventions.md).

## Admin API Domains

| Domain | File |
| :--- | :--- |
| Conventions, auth model, response formats, billing, security, CORS & trusted IPs | [`conventions.md`](api/conventions.md) |
| Stats & dashboard + proxy debug traces | [`stats.md`](api/stats.md) |
| Routes, channels & route decision | [`routes.md`](api/routes.md) |
| Marketplace, price compare, probes & model redirects | [`models.md`](api/models.md) |
| Sites CRUD, detect/import/batch, probe & tags | [`sites.md`](api/sites.md) |
| Accounts, account tokens & rebind | [`accounts.md`](api/accounts.md) |
| Site announcements, operator banners & system events | [`announcements.md`](api/announcements.md) |
| Downstream API keys, scope contract & export | [`downstream-keys.md`](api/downstream-keys.md) |
| Runtime/database/backup/notifications/maintenance/auth settings | [`settings.md`](api/settings.md) |
| Check-in trigger, logs & schedule | [`checkin.md`](api/checkin.md) |
| Monitor session/config + update center (residual) | [`monitor.md`](api/monitor.md) |
| Resin/scheduler/rates/audit-logs/ops-ws, search, tasks & test surfaces | [`diagnostics.md`](api/diagnostics.md) |
| Admin session model (login/logout/session/ws-ticket) | [`auth.md`](api/auth.md) |
| OAuth providers, sessions, connections & route units | [`oauth.md`](api/oauth.md) |
| /health /ready /desktop/health & /about | [`health.md`](api/health.md) |
| Full registered /api route inventory | [`routes-inventory.md`](api/routes-inventory.md) |
| /v1/files & /v1/pricing | [`proxy.md`](api/proxy.md) |

### Sensitive operations (master-token re-confirmation)
→ Moved to [api/conventions.md#sensitive-operations-master-token-re-confirmation](api/conventions.md#sensitive-operations-master-token-re-confirmation)
## Response Format
→ Moved to [api/conventions.md#response-format](api/conventions.md#response-format)
### Success (2xx)
→ Moved to [api/conventions.md#success-2xx](api/conventions.md#success-2xx)
### Error (non-2xx)
→ Moved to [api/conventions.md#error-non-2xx](api/conventions.md#error-non-2xx)
#### errorCode convention and registry
→ Moved to [api/conventions.md#errorcode-convention-and-registry](api/conventions.md#errorcode-convention-and-registry)
## Request Body Rules
→ Moved to [api/conventions.md#request-body-rules](api/conventions.md#request-body-rules)
## Billing & Currency
→ Moved to [api/conventions.md#billing--currency](api/conventions.md#billing--currency)
## Stats & Dashboard
→ Moved to [api/stats.md#stats--dashboard](api/stats.md#stats--dashboard)
### GET /api/stats/dashboard
→ Moved to [api/stats.md#get-apistatsdashboard](api/stats.md#get-apistatsdashboard)
### GET /api/stats/proxy-logs
→ Moved to [api/stats.md#get-apistatsproxy-logs](api/stats.md#get-apistatsproxy-logs)
### GET /api/stats/proxy-logs/:id
→ Moved to [api/stats.md#get-apistatsproxy-logsid](api/stats.md#get-apistatsproxy-logsid)
### GET /api/stats/site-distribution
→ Moved to [api/stats.md#get-apistatssite-distribution](api/stats.md#get-apistatssite-distribution)
### GET /api/stats/site-trend
→ Moved to [api/stats.md#get-apistatssite-trend](api/stats.md#get-apistatssite-trend)
### GET /api/stats/model-by-site
→ Moved to [api/stats.md#get-apistatsmodel-by-site](api/stats.md#get-apistatsmodel-by-site)
### GET /api/stats/usage-heatmap
→ Moved to [api/stats.md#get-apistatsusage-heatmap](api/stats.md#get-apistatsusage-heatmap)
### GET /api/stats/slow-requests
→ Moved to [api/stats.md#get-apistatsslow-requests](api/stats.md#get-apistatsslow-requests)
### GET /api/stats/attention
→ Moved to [api/stats.md#get-apistatsattention](api/stats.md#get-apistatsattention)
### GET /api/stats/balance-history, GET /api/stats/balance-income-outcome
→ Moved to [api/stats.md#get-apistatsbalance-history-get-apistatsbalance-income-outcome](api/stats.md#get-apistatsbalance-history-get-apistatsbalance-income-outcome)
### GET /api/stats/latency-histogram, GET /api/stats/latency-trend
→ Moved to [api/stats.md#get-apistatslatency-histogram-get-apistatslatency-trend](api/stats.md#get-apistatslatency-histogram-get-apistatslatency-trend)
### GET /api/stats/model-cost-distribution
→ Moved to [api/stats.md#get-apistatsmodel-cost-distribution](api/stats.md#get-apistatsmodel-cost-distribution)
### GET /api/stats/model-prices
→ Moved to [api/stats.md#get-apistatsmodel-prices](api/stats.md#get-apistatsmodel-prices)
## Models & Routes
→ Moved to [api/routes.md#models--routes](api/routes.md#models--routes)
### GET /api/routes/lite
→ Moved to [api/routes.md#get-apirouteslite](api/routes.md#get-apirouteslite)
### GET /api/routes/summary
→ Moved to [api/routes.md#get-apiroutessummary](api/routes.md#get-apiroutessummary)
### GET /api/routes
→ Moved to [api/routes.md#get-apiroutes](api/routes.md#get-apiroutes)
### GET /api/routes/:id/channels
→ Moved to [api/routes.md#get-apiroutesidchannels](api/routes.md#get-apiroutesidchannels)
### GET /api/channels
→ Moved to [api/routes.md#get-apichannels](api/routes.md#get-apichannels)
### GET /api/channels/error-summary
→ Moved to [api/routes.md#get-apichannelserror-summary](api/routes.md#get-apichannelserror-summary)
### GET /api/channels/probe-history
→ Moved to [api/routes.md#get-apichannelsprobe-history](api/routes.md#get-apichannelsprobe-history)
### POST /api/routes
→ Moved to [api/routes.md#post-apiroutes](api/routes.md#post-apiroutes)
### PUT /api/routes/:id
→ Moved to [api/routes.md#put-apiroutesid](api/routes.md#put-apiroutesid)
### DELETE /api/routes/:id
→ Moved to [api/routes.md#delete-apiroutesid](api/routes.md#delete-apiroutesid)
### POST /api/routes/batch
→ Moved to [api/routes.md#post-apiroutesbatch](api/routes.md#post-apiroutesbatch)
### PUT /api/routes/reorder
→ Moved to [api/routes.md#put-apiroutesreorder](api/routes.md#put-apiroutesreorder)
### POST /api/routes/rebuild
→ Moved to [api/routes.md#post-apiroutesrebuild](api/routes.md#post-apiroutesrebuild)
### POST /api/routes/:id/cooldown/clear
→ Moved to [api/routes.md#post-apiroutesidcooldownclear](api/routes.md#post-apiroutesidcooldownclear)
### POST /api/routes/:id/channels/batch
→ Moved to [api/routes.md#post-apiroutesidchannelsbatch](api/routes.md#post-apiroutesidchannelsbatch)
### POST /api/routes/:id/channels
→ Moved to [api/routes.md#post-apiroutesidchannels](api/routes.md#post-apiroutesidchannels)
### PUT /api/channels/batch
→ Moved to [api/routes.md#put-apichannelsbatch](api/routes.md#put-apichannelsbatch)
### PUT /api/channels/:channelId
→ Moved to [api/routes.md#put-apichannelschannelid](api/routes.md#put-apichannelschannelid)
### DELETE /api/channels/:channelId
→ Moved to [api/routes.md#delete-apichannelschannelid](api/routes.md#delete-apichannelschannelid)
### POST /api/admin/test-channel
→ Moved to [api/routes.md#post-apiadmintest-channel](api/routes.md#post-apiadmintest-channel)
## Route Decision
→ Moved to [api/routes.md#route-decision](api/routes.md#route-decision)
### GET /api/routes/decision
→ Moved to [api/routes.md#get-apiroutesdecision](api/routes.md#get-apiroutesdecision)
### POST /api/routes/decision/batch
→ Moved to [api/routes.md#post-apiroutesdecisionbatch](api/routes.md#post-apiroutesdecisionbatch)
### POST /api/routes/decision/by-route/batch
→ Moved to [api/routes.md#post-apiroutesdecisionby-routebatch](api/routes.md#post-apiroutesdecisionby-routebatch)
### POST /api/routes/decision/route-wide/batch
→ Moved to [api/routes.md#post-apiroutesdecisionroute-widebatch](api/routes.md#post-apiroutesdecisionroute-widebatch)
### POST /api/routes/decision/refresh
→ Moved to [api/routes.md#post-apiroutesdecisionrefresh](api/routes.md#post-apiroutesdecisionrefresh)
## Model Marketplace & Probing
→ Moved to [api/models.md#model-marketplace--probing](api/models.md#model-marketplace--probing)
### GET /api/models/marketplace
→ Moved to [api/models.md#get-apimodelsmarketplace](api/models.md#get-apimodelsmarketplace)
### GET /api/models/price-compare
→ Moved to [api/models.md#get-apimodelsprice-compare](api/models.md#get-apimodelsprice-compare)
### GET /api/models/token-candidates
→ Moved to [api/models.md#get-apimodelstoken-candidates](api/models.md#get-apimodelstoken-candidates)
### POST /api/models/check/:accountId
→ Moved to [api/models.md#post-apimodelscheckaccountid](api/models.md#post-apimodelscheckaccountid)
### POST /api/models/probe
→ Moved to [api/models.md#post-apimodelsprobe](api/models.md#post-apimodelsprobe)
### POST /api/models/verify-batch, GET /api/models/verify-history
→ Moved to [api/models.md#post-apimodelsverify-batch-get-apimodelsverify-history](api/models.md#post-apimodelsverify-batch-get-apimodelsverify-history)
## Model Redirects
→ Moved to [api/models.md#model-redirects](api/models.md#model-redirects)
### GET /api/model-redirects, PUT /api/model-redirects/{id}, DELETE /api/model-redirects/{id}
→ Moved to [api/models.md#get-apimodel-redirects-put-apimodel-redirectsid-delete-apimodel-redirectsid](api/models.md#get-apimodel-redirects-put-apimodel-redirectsid-delete-apimodel-redirectsid)
### POST /api/model-redirects/generate, POST /api/model-redirects/apply
→ Moved to [api/models.md#post-apimodel-redirectsgenerate-post-apimodel-redirectsapply](api/models.md#post-apimodel-redirectsgenerate-post-apimodel-redirectsapply)
## Proxy Debug
→ Moved to [api/stats.md#proxy-debug](api/stats.md#proxy-debug)
### GET /api/stats/proxy-debug/traces
→ Moved to [api/stats.md#get-apistatsproxy-debugtraces](api/stats.md#get-apistatsproxy-debugtraces)
### GET /api/stats/proxy-debug/traces/:id
→ Moved to [api/stats.md#get-apistatsproxy-debugtracesid](api/stats.md#get-apistatsproxy-debugtracesid)
## Sites
→ Moved to [api/sites.md#sites](api/sites.md#sites)
### GET /api/sites, POST /api/sites
→ Moved to [api/sites.md#get-apisites-post-apisites](api/sites.md#get-apisites-post-apisites)
### GET /api/sites/:id, PUT /api/sites/:id, DELETE /api/sites/:id
→ Moved to [api/sites.md#get-apisitesid-put-apisitesid-delete-apisitesid](api/sites.md#get-apisitesid-put-apisitesid-delete-apisitesid)
### POST /api/sites/detect
→ Moved to [api/sites.md#post-apisitesdetect](api/sites.md#post-apisitesdetect)
### POST /api/sites/import
→ Moved to [api/sites.md#post-apisitesimport](api/sites.md#post-apisitesimport)
### POST /api/sites/batch
→ Moved to [api/sites.md#post-apisitesbatch](api/sites.md#post-apisitesbatch)
### POST /api/sites/{id}/probe-now, GET /api/sites/{id}/probe-stream
→ Moved to [api/sites.md#post-apisitesidprobe-now-get-apisitesidprobe-stream](api/sites.md#post-apisitesidprobe-now-get-apisitesidprobe-stream)
### GET /api/sites/{id}/available-models, GET /api/sites/{id}/disabled-models, PUT /api/sites/{id}/disabled-models
→ Moved to [api/sites.md#get-apisitesidavailable-models-get-apisitesiddisabled-models-put-apisitesiddisabled-models](api/sites.md#get-apisitesidavailable-models-get-apisitesiddisabled-models-put-apisitesiddisabled-models)
### PUT /api/sites/{id}/tags, PUT /api/accounts/{id}/tags, GET /api/tags
→ Moved to [api/sites.md#put-apisitesidtags-put-apiaccountsidtags-get-apitags](api/sites.md#put-apisitesidtags-put-apiaccountsidtags-get-apitags)
## Accounts
→ Moved to [api/accounts.md#accounts](api/accounts.md#accounts)
### GET /api/accounts, POST /api/accounts
→ Moved to [api/accounts.md#get-apiaccounts-post-apiaccounts](api/accounts.md#get-apiaccounts-post-apiaccounts)
### GET /api/accounts/:id, PUT /api/accounts/:id, DELETE /api/accounts/:id
→ Moved to [api/accounts.md#get-apiaccountsid-put-apiaccountsid-delete-apiaccountsid](api/accounts.md#get-apiaccountsid-put-apiaccountsid-delete-apiaccountsid)
### GET /api/accounts/probe-history
→ Moved to [api/accounts.md#get-apiaccountsprobe-history](api/accounts.md#get-apiaccountsprobe-history)
### POST /api/accounts/login
→ Moved to [api/accounts.md#post-apiaccountslogin](api/accounts.md#post-apiaccountslogin)
### POST /api/accounts/verify-token
→ Moved to [api/accounts.md#post-apiaccountsverify-token](api/accounts.md#post-apiaccountsverify-token)
### POST /api/accounts/batch, POST /api/accounts/health/refresh
→ Moved to [api/accounts.md#post-apiaccountsbatch-post-apiaccountshealthrefresh](api/accounts.md#post-apiaccountsbatch-post-apiaccountshealthrefresh)
### POST /api/accounts/{id}/balance
→ Moved to [api/accounts.md#post-apiaccountsidbalance](api/accounts.md#post-apiaccountsidbalance)
### GET /api/accounts/{id}/models, POST /api/accounts/{id}/models/manual
→ Moved to [api/accounts.md#get-apiaccountsidmodels-post-apiaccountsidmodelsmanual](api/accounts.md#get-apiaccountsidmodels-post-apiaccountsidmodelsmanual)
### POST /api/accounts/{id}/rebind-session
→ Moved to [api/accounts.md#post-apiaccountsidrebind-session](api/accounts.md#post-apiaccountsidrebind-session)
## Account Tokens
→ Moved to [api/accounts.md#account-tokens](api/accounts.md#account-tokens)
### GET /api/account-tokens, POST /api/account-tokens
→ Moved to [api/accounts.md#get-apiaccount-tokens-post-apiaccount-tokens](api/accounts.md#get-apiaccount-tokens-post-apiaccount-tokens)
### GET /api/account-tokens/:id, PUT /api/account-tokens/:id, DELETE /api/account-tokens/:id
→ Moved to [api/accounts.md#get-apiaccount-tokensid-put-apiaccount-tokensid-delete-apiaccount-tokensid](api/accounts.md#get-apiaccount-tokensid-put-apiaccount-tokensid-delete-apiaccount-tokensid)
### GET /api/account-tokens/{id}/value
→ Moved to [api/accounts.md#get-apiaccount-tokensidvalue](api/accounts.md#get-apiaccount-tokensidvalue)
### GET /api/account-tokens/groups/{accountId}, GET /api/account-tokens/account/{accountId}/default
→ Moved to [api/accounts.md#get-apiaccount-tokensgroupsaccountid-get-apiaccount-tokensaccountaccountiddefault](api/accounts.md#get-apiaccount-tokensgroupsaccountid-get-apiaccount-tokensaccountaccountiddefault)
### POST /api/account-tokens/batch, POST /api/account-tokens/{id}/default
→ Moved to [api/accounts.md#post-apiaccount-tokensbatch-post-apiaccount-tokensiddefault](api/accounts.md#post-apiaccount-tokensbatch-post-apiaccount-tokensiddefault)
### POST /api/account-tokens/sync/{accountId}, POST /api/account-tokens/sync-all
→ Moved to [api/accounts.md#post-apiaccount-tokenssyncaccountid-post-apiaccount-tokenssync-all](api/accounts.md#post-apiaccount-tokenssyncaccountid-post-apiaccount-tokenssync-all)
## Site Announcements
→ Moved to [api/announcements.md#site-announcements](api/announcements.md#site-announcements)
### GET /api/site-announcements
→ Moved to [api/announcements.md#get-apisite-announcements](api/announcements.md#get-apisite-announcements)
### POST /api/site-announcements/{id}/read
→ Moved to [api/announcements.md#post-apisite-announcementsidread](api/announcements.md#post-apisite-announcementsidread)
### POST /api/site-announcements/read-all
→ Moved to [api/announcements.md#post-apisite-announcementsread-all](api/announcements.md#post-apisite-announcementsread-all)
### POST /api/site-announcements/sync
→ Moved to [api/announcements.md#post-apisite-announcementssync](api/announcements.md#post-apisite-announcementssync)
### DELETE /api/site-announcements
→ Moved to [api/announcements.md#delete-apisite-announcements](api/announcements.md#delete-apisite-announcements)
## Announcements
→ Moved to [api/announcements.md#announcements](api/announcements.md#announcements)
### GET /api/announcements, GET /api/announcements/active
→ Moved to [api/announcements.md#get-apiannouncements-get-apiannouncementsactive](api/announcements.md#get-apiannouncements-get-apiannouncementsactive)
### POST /api/announcements, PUT /api/announcements/{id}, DELETE /api/announcements/{id}
→ Moved to [api/announcements.md#post-apiannouncements-put-apiannouncementsid-delete-apiannouncementsid](api/announcements.md#post-apiannouncements-put-apiannouncementsid-delete-apiannouncementsid)
### POST /api/announcements/{id}/dismiss
→ Moved to [api/announcements.md#post-apiannouncementsiddismiss](api/announcements.md#post-apiannouncementsiddismiss)
## Events
→ Moved to [api/announcements.md#events](api/announcements.md#events)
### GET /api/events
→ Moved to [api/announcements.md#get-apievents](api/announcements.md#get-apievents)
### GET /api/events/count
→ Moved to [api/announcements.md#get-apieventscount](api/announcements.md#get-apieventscount)
### POST /api/events/read-all
→ Moved to [api/announcements.md#post-apieventsread-all](api/announcements.md#post-apieventsread-all)
### POST /api/events/{id}/read
→ Moved to [api/announcements.md#post-apieventsidread](api/announcements.md#post-apieventsidread)
### DELETE /api/events
→ Moved to [api/announcements.md#delete-apievents](api/announcements.md#delete-apievents)
## Downstream API Keys
→ Moved to [api/downstream-keys.md#downstream-api-keys](api/downstream-keys.md#downstream-api-keys)
### GET /api/downstream-keys
→ Moved to [api/downstream-keys.md#get-apidownstream-keys](api/downstream-keys.md#get-apidownstream-keys)
### GET /api/downstream-keys/summary
→ Moved to [api/downstream-keys.md#get-apidownstream-keyssummary](api/downstream-keys.md#get-apidownstream-keyssummary)
### GET /api/downstream-keys/:id/overview
→ Moved to [api/downstream-keys.md#get-apidownstream-keysidoverview](api/downstream-keys.md#get-apidownstream-keysidoverview)
### GET /api/downstream-keys/:id/trend
→ Moved to [api/downstream-keys.md#get-apidownstream-keysidtrend](api/downstream-keys.md#get-apidownstream-keysidtrend)
### POST /api/downstream-keys
→ Moved to [api/downstream-keys.md#post-apidownstream-keys](api/downstream-keys.md#post-apidownstream-keys)
### Credential & site scope (downstream keys)
→ Moved to [api/downstream-keys.md#credential--site-scope-downstream-keys](api/downstream-keys.md#credential--site-scope-downstream-keys)
### PUT /api/downstream-keys/:id
→ Moved to [api/downstream-keys.md#put-apidownstream-keysid](api/downstream-keys.md#put-apidownstream-keysid)
### DELETE /api/downstream-keys/:id
→ Moved to [api/downstream-keys.md#delete-apidownstream-keysid](api/downstream-keys.md#delete-apidownstream-keysid)
### POST /api/downstream-keys/:id/reset-usage
→ Moved to [api/downstream-keys.md#post-apidownstream-keysidreset-usage](api/downstream-keys.md#post-apidownstream-keysidreset-usage)
### POST /api/downstream-keys/batch
→ Moved to [api/downstream-keys.md#post-apidownstream-keysbatch](api/downstream-keys.md#post-apidownstream-keysbatch)
## Settings
→ Moved to [api/settings.md#settings](api/settings.md#settings)
### GET /api/settings/runtime
→ Moved to [api/settings.md#get-apisettingsruntime](api/settings.md#get-apisettingsruntime)
### PUT /api/settings/runtime
→ Moved to [api/settings.md#put-apisettingsruntime](api/settings.md#put-apisettingsruntime)
### GET /api/settings/migration/preview
→ Moved to [api/settings.md#get-apisettingsmigrationpreview](api/settings.md#get-apisettingsmigrationpreview)
### POST /api/settings/migration/apply
→ Moved to [api/settings.md#post-apisettingsmigrationapply](api/settings.md#post-apisettingsmigrationapply)
### GET /api/settings/brand-list
→ Moved to [api/settings.md#get-apisettingsbrand-list](api/settings.md#get-apisettingsbrand-list)
### POST /api/settings/system-proxy/test
→ Moved to [api/settings.md#post-apisettingssystem-proxytest](api/settings.md#post-apisettingssystem-proxytest)
## Settings - Database
→ Moved to [api/settings.md#settings---database](api/settings.md#settings---database)
### GET /api/settings/database/runtime
→ Moved to [api/settings.md#get-apisettingsdatabaseruntime](api/settings.md#get-apisettingsdatabaseruntime)
### PUT /api/settings/database/runtime
→ Moved to [api/settings.md#put-apisettingsdatabaseruntime](api/settings.md#put-apisettingsdatabaseruntime)
### POST /api/settings/database/test-connection
→ Moved to [api/settings.md#post-apisettingsdatabasetest-connection](api/settings.md#post-apisettingsdatabasetest-connection)
### POST /api/settings/database/migrate
→ Moved to [api/settings.md#post-apisettingsdatabasemigrate](api/settings.md#post-apisettingsdatabasemigrate)
## Settings - Backup
→ Moved to [api/settings.md#settings---backup](api/settings.md#settings---backup)
### GET /api/settings/backup/export
→ Moved to [api/settings.md#get-apisettingsbackupexport](api/settings.md#get-apisettingsbackupexport)
### POST /api/settings/backup/import
→ Moved to [api/settings.md#post-apisettingsbackupimport](api/settings.md#post-apisettingsbackupimport)
### GET /api/settings/backup/webdav
→ Moved to [api/settings.md#get-apisettingsbackupwebdav](api/settings.md#get-apisettingsbackupwebdav)
### PUT /api/settings/backup/webdav
→ Moved to [api/settings.md#put-apisettingsbackupwebdav](api/settings.md#put-apisettingsbackupwebdav)
### POST /api/settings/backup/webdav/export
→ Moved to [api/settings.md#post-apisettingsbackupwebdavexport](api/settings.md#post-apisettingsbackupwebdavexport)
### POST /api/settings/backup/webdav/import
→ Moved to [api/settings.md#post-apisettingsbackupwebdavimport](api/settings.md#post-apisettingsbackupwebdavimport)
### POST /api/settings/backup/import/preview
→ Moved to [api/settings.md#post-apisettingsbackupimportpreview](api/settings.md#post-apisettingsbackupimportpreview)
## Settings - Notifications
→ Moved to [api/settings.md#settings---notifications](api/settings.md#settings---notifications)
### POST /api/settings/notify/test
→ Moved to [api/settings.md#post-apisettingsnotifytest](api/settings.md#post-apisettingsnotifytest)
## Settings - Maintenance
→ Moved to [api/settings.md#settings---maintenance](api/settings.md#settings---maintenance)
### POST /api/settings/maintenance/clear-cache
→ Moved to [api/settings.md#post-apisettingsmaintenanceclear-cache](api/settings.md#post-apisettingsmaintenanceclear-cache)
### POST /api/settings/maintenance/clear-usage
→ Moved to [api/settings.md#post-apisettingsmaintenanceclear-usage](api/settings.md#post-apisettingsmaintenanceclear-usage)
### POST /api/settings/maintenance/factory-reset
→ Moved to [api/settings.md#post-apisettingsmaintenancefactory-reset](api/settings.md#post-apisettingsmaintenancefactory-reset)
## Checkin
→ Moved to [api/checkin.md#checkin](api/checkin.md#checkin)
### POST /api/checkin/trigger
→ Moved to [api/checkin.md#post-apicheckintrigger](api/checkin.md#post-apicheckintrigger)
### POST /api/checkin/trigger/{id}
→ Moved to [api/checkin.md#post-apicheckintriggerid](api/checkin.md#post-apicheckintriggerid)
### GET /api/checkin/logs
→ Moved to [api/checkin.md#get-apicheckinlogs](api/checkin.md#get-apicheckinlogs)
### PUT /api/checkin/schedule
→ Moved to [api/checkin.md#put-apicheckinschedule](api/checkin.md#put-apicheckinschedule)
## Update Center
→ Moved to [api/monitor.md#update-center](api/monitor.md#update-center)
### GET /api/update-center/status
→ Moved to [api/monitor.md#get-apiupdate-centerstatus](api/monitor.md#get-apiupdate-centerstatus)
### POST /api/update-center/check
→ Moved to [api/monitor.md#post-apiupdate-centercheck](api/monitor.md#post-apiupdate-centercheck)
### PUT /api/update-center/config
→ Moved to [api/monitor.md#put-apiupdate-centerconfig](api/monitor.md#put-apiupdate-centerconfig)
### POST /api/update-center/deploy
→ Moved to [api/monitor.md#post-apiupdate-centerdeploy](api/monitor.md#post-apiupdate-centerdeploy)
### POST /api/update-center/rollback
→ Moved to [api/monitor.md#post-apiupdate-centerrollback](api/monitor.md#post-apiupdate-centerrollback)
### GET /api/update-center/tasks/:id/stream
→ Moved to [api/monitor.md#get-apiupdate-centertasksidstream](api/monitor.md#get-apiupdate-centertasksidstream)
## Monitor
→ Moved to [api/monitor.md#monitor](api/monitor.md#monitor)
### GET /api/monitor/health
→ Moved to [api/monitor.md#get-apimonitorhealth](api/monitor.md#get-apimonitorhealth)
### GET /api/monitor/config
→ Moved to [api/monitor.md#get-apimonitorconfig](api/monitor.md#get-apimonitorconfig)
### PUT /api/monitor/config
→ Moved to [api/monitor.md#put-apimonitorconfig](api/monitor.md#put-apimonitorconfig)
### POST /api/monitor/session
→ Moved to [api/monitor.md#post-apimonitorsession](api/monitor.md#post-apimonitorsession)
### DELETE /api/monitor/session
→ Moved to [api/monitor.md#delete-apimonitorsession](api/monitor.md#delete-apimonitorsession)
## Admin Diagnostics & Observability
→ Moved to [api/diagnostics.md#admin-diagnostics--observability](api/diagnostics.md#admin-diagnostics--observability)
### GET /api/admin/resin/status
→ Moved to [api/diagnostics.md#get-apiadminresinstatus](api/diagnostics.md#get-apiadminresinstatus)
### GET /api/scheduler/status
→ Moved to [api/diagnostics.md#get-apischedulerstatus](api/diagnostics.md#get-apischedulerstatus)
### GET /api/models/rates
→ Moved to [api/diagnostics.md#get-apimodelsrates](api/diagnostics.md#get-apimodelsrates)
### PUT /api/models/rates
→ Moved to [api/diagnostics.md#put-apimodelsrates](api/diagnostics.md#put-apimodelsrates)
### GET /api/admin/audit-logs
→ Moved to [api/diagnostics.md#get-apiadminaudit-logs](api/diagnostics.md#get-apiadminaudit-logs)
### GET /api/debug/vars
→ Moved to [api/diagnostics.md#get-apidebugvars](api/diagnostics.md#get-apidebugvars)
### GET /api/admin/ops/ws
→ Moved to [api/diagnostics.md#get-apiadminopsws](api/diagnostics.md#get-apiadminopsws)
## Search
→ Moved to [api/diagnostics.md#search](api/diagnostics.md#search)
### POST /api/search
→ Moved to [api/diagnostics.md#post-apisearch](api/diagnostics.md#post-apisearch)
## Tasks
→ Moved to [api/diagnostics.md#tasks](api/diagnostics.md#tasks)
### GET /api/tasks
→ Moved to [api/diagnostics.md#get-apitasks](api/diagnostics.md#get-apitasks)
### GET /api/tasks/:id
→ Moved to [api/diagnostics.md#get-apitasksid](api/diagnostics.md#get-apitasksid)
## Test
→ Moved to [api/diagnostics.md#test](api/diagnostics.md#test)
### POST /api/test/proxy, POST /api/test/chat
→ Moved to [api/diagnostics.md#post-apitestproxy-post-apitestchat](api/diagnostics.md#post-apitestproxy-post-apitestchat)
### POST /api/test/chat/stream, POST /api/test/proxy/stream, POST /api/test/chat/jobs, POST /api/test/proxy/jobs, GET /api/test/chat/jobs/{jobId}, GET /api/test/proxy/jobs/{jobId}, DELETE /api/test/chat/jobs/{jobId}, DELETE /api/test/proxy/jobs/{jobId}
→ Moved to [api/diagnostics.md#post-apitestchatstream-post-apitestproxystream-post-apitestchatjobs-post-apitestproxyjobs-get-apitestchatjobsjobid-get-apitestproxyjobsjobid-delete-apitestchatjobsjobid-delete-apitestproxyjobsjobid](api/diagnostics.md#post-apitestchatstream-post-apitestproxystream-post-apitestchatjobs-post-apitestproxyjobs-get-apitestchatjobsjobid-get-apitestproxyjobsjobid-delete-apitestchatjobsjobid-delete-apitestproxyjobsjobid)
### POST /api/debug/channel-probe
→ Moved to [api/diagnostics.md#post-apidebugchannel-probe](api/diagnostics.md#post-apidebugchannel-probe)
## Admin Session (#1034 session model)
→ Moved to [api/auth.md#admin-session-1034-session-model](api/auth.md#admin-session-1034-session-model)
### POST /api/auth/login
→ Moved to [api/auth.md#post-apiauthlogin](api/auth.md#post-apiauthlogin)
### GET /api/auth/session
→ Moved to [api/auth.md#get-apiauthsession](api/auth.md#get-apiauthsession)
### POST /api/auth/logout
→ Moved to [api/auth.md#post-apiauthlogout](api/auth.md#post-apiauthlogout)
### POST /api/auth/ws-ticket
→ Moved to [api/auth.md#post-apiauthws-ticket](api/auth.md#post-apiauthws-ticket)
## OAuth
→ Moved to [api/oauth.md#oauth](api/oauth.md#oauth)
### GET /api/oauth/providers
→ Moved to [api/oauth.md#get-apioauthproviders](api/oauth.md#get-apioauthproviders)
### POST /api/oauth/providers/{provider}/start
→ Moved to [api/oauth.md#post-apioauthprovidersproviderstart](api/oauth.md#post-apioauthprovidersproviderstart)
### GET /api/oauth/callback/{provider}
→ Moved to [api/oauth.md#get-apioauthcallbackprovider](api/oauth.md#get-apioauthcallbackprovider)
### GET /api/oauth/connections
→ Moved to [api/oauth.md#get-apioauthconnections](api/oauth.md#get-apioauthconnections)
### GET /api/oauth/sessions/{state}, POST /api/oauth/sessions/{state}/manual-callback
→ Moved to [api/oauth.md#get-apioauthsessionsstate-post-apioauthsessionsstatemanual-callback](api/oauth.md#get-apioauthsessionsstate-post-apioauthsessionsstatemanual-callback)
### POST /api/oauth/connections/{accountId}/rebind, PATCH /api/oauth/connections/{accountId}/proxy, DELETE /api/oauth/connections/{accountId}
→ Moved to [api/oauth.md#post-apioauthconnectionsaccountidrebind-patch-apioauthconnectionsaccountidproxy-delete-apioauthconnectionsaccountid](api/oauth.md#post-apioauthconnectionsaccountidrebind-patch-apioauthconnectionsaccountidproxy-delete-apioauthconnectionsaccountid)
### POST /api/oauth/connections/{accountId}/quota/refresh, POST /api/oauth/connections/quota/refresh-batch
→ Moved to [api/oauth.md#post-apioauthconnectionsaccountidquotarefresh-post-apioauthconnectionsquotarefresh-batch](api/oauth.md#post-apioauthconnectionsaccountidquotarefresh-post-apioauthconnectionsquotarefresh-batch)
### POST /api/oauth/import, POST /api/oauth/route-units, PATCH /api/oauth/route-units/{routeUnitId}, DELETE /api/oauth/route-units/{routeUnitId}
→ Moved to [api/oauth.md#post-apioauthimport-post-apioauthroute-units-patch-apioauthroute-unitsrouteunitid-delete-apioauthroute-unitsrouteunitid](api/oauth.md#post-apioauthimport-post-apioauthroute-units-patch-apioauthroute-unitsrouteunitid-delete-apioauthroute-unitsrouteunitid)
## Auth Settings
→ Moved to [api/settings.md#auth-settings](api/settings.md#auth-settings)
### GET /api/settings/auth/info
→ Moved to [api/settings.md#get-apisettingsauthinfo](api/settings.md#get-apisettingsauthinfo)
### POST /api/settings/auth/change
→ Moved to [api/settings.md#post-apisettingsauthchange](api/settings.md#post-apisettingsauthchange)
## Health
→ Moved to [api/health.md#health](api/health.md#health)
### GET /health
→ Moved to [api/health.md#get-health](api/health.md#get-health)
### GET /ready
→ Moved to [api/health.md#get-ready](api/health.md#get-ready)
### GET /api/desktop/health
→ Moved to [api/health.md#get-apidesktophealth](api/health.md#get-apidesktophealth)
## About
→ Moved to [api/health.md#about](api/health.md#about)
### GET /api/about
→ Moved to [api/health.md#get-apiabout](api/health.md#get-apiabout)
## Security Notes
→ Moved to [api/conventions.md#security-notes](api/conventions.md#security-notes)
### LDOH monitor proxy
→ Moved to [api/conventions.md#ldoh-monitor-proxy](api/conventions.md#ldoh-monitor-proxy)
### WebDAV backup SSRF hardening
→ Moved to [api/conventions.md#webdav-backup-ssrf-hardening](api/conventions.md#webdav-backup-ssrf-hardening)
## Browser CORS
→ Moved to [api/conventions.md#browser-cors](api/conventions.md#browser-cors)
## Trusted Client IPs
→ Moved to [api/conventions.md#trusted-client-ips](api/conventions.md#trusted-client-ips)
### GET /api/downstream-keys/:id/export
→ Moved to [api/downstream-keys.md#get-apidownstream-keysidexport](api/downstream-keys.md#get-apidownstream-keysidexport)
## Proxy files (`/v1/files`)
→ Moved to [api/proxy.md#proxy-files-v1files](api/proxy.md#proxy-files-v1files)
## Downstream Pricing (`/v1/pricing`)
→ Moved to [api/proxy.md#downstream-pricing-v1pricing](api/proxy.md#downstream-pricing-v1pricing)
### GET /v1/pricing
→ Moved to [api/proxy.md#get-v1pricing](api/proxy.md#get-v1pricing)
## Admin Route Inventory
→ Moved to [api/routes-inventory.md#admin-route-inventory](api/routes-inventory.md#admin-route-inventory)
### GET
→ Moved to [api/routes-inventory.md#get](api/routes-inventory.md#get)
### POST
→ Moved to [api/routes-inventory.md#post](api/routes-inventory.md#post)
### PUT
→ Moved to [api/routes-inventory.md#put](api/routes-inventory.md#put)
### PATCH
→ Moved to [api/routes-inventory.md#patch](api/routes-inventory.md#patch)
### DELETE
→ Moved to [api/routes-inventory.md#delete](api/routes-inventory.md#delete)
