src = open('service/daily/daily_summary.go', encoding='utf-8').read()
src = src.replace('func SendDailySummary(cfg *config.Config, db *sqlx.DB) {', 'func SendDailySummary(cfg *config.RuntimeSettings, db *sqlx.DB) {', 1)
open('service/daily/daily_summary.go','w',encoding='utf-8').write(src)
print("daily fixed")
