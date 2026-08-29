src = open('scheduler/daily_summary.go', encoding='utf-8').read()
src = src.replace('''	cfg        *config.Config
''', '')
src = src.replace('''func NewDailySummaryScheduler(cfg *config.Config) *DailySummaryScheduler {
	return &DailySummaryScheduler{cfg: cfg}
}''', '''func NewDailySummaryScheduler() *DailySummaryScheduler {
	return &DailySummaryScheduler{}
}''', 1)
open('scheduler/daily_summary.go','w',encoding='utf-8').write(src)
print("daily scheduler cfg removed")
