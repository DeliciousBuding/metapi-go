import re
src = open('handler/admin/checkin_schedule.go', encoding='utf-8').read()

src = src.replace('func applyCheckinScheduleSettings(db *sqlx.DB, cfg *config.Config, patch checkinSchedulePatch) (checkinScheduleState, error) {\n\tstate := resolveCheckinScheduleState(cfg, patch)',
                  'func applyCheckinScheduleSettings(db *sqlx.DB, rt *config.RuntimeSettings, patch checkinSchedulePatch) (checkinScheduleState, error) {\n\tstate := resolveCheckinScheduleState(rt, patch)', 1)

src = src.replace('''	cfg.CheckinScheduleMode = state.Mode
	cfg.CheckinCron = state.Cron
	cfg.CheckinIntervalHours = state.IntervalHours
	cfg.CheckinWindowStart = state.WindowStart
	cfg.CheckinWindowEnd = state.WindowEnd''',
'''	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.CheckinScheduleMode = state.Mode
		r.CheckinCron = state.Cron
		r.CheckinIntervalHours = state.IntervalHours
		r.CheckinWindowStart = state.WindowStart
		r.CheckinWindowEnd = state.WindowEnd
	})''', 1)

src = src.replace('func resolveCheckinScheduleState(cfg *config.Config, patch checkinSchedulePatch) checkinScheduleState {',
                  'func resolveCheckinScheduleState(rt *config.RuntimeSettings, patch checkinSchedulePatch) checkinScheduleState {', 1)
for f in ['CheckinScheduleMode','CheckinCron','CheckinIntervalHours','CheckinWindowStart','CheckinWindowEnd']:
    src = src.replace('cfg.'+f, 'rt.'+f)

src = src.replace('func scheduleSpecForCheckin(cfg *config.Config) scheduler.ScheduleSpec {',
                  'func scheduleSpecForCheckin(rt *config.RuntimeSettings) scheduler.ScheduleSpec {', 1)
src = src.replace('switch cfg.CheckinScheduleMode', 'switch rt.CheckinScheduleMode')
src = src.replace('WindowStart: cfg.CheckinWindowStart, WindowEnd: cfg.CheckinWindowEnd, Cron: cfg.CheckinCron',
                  'WindowStart: rt.CheckinWindowStart, WindowEnd: rt.CheckinWindowEnd, Cron: rt.CheckinCron')
src = src.replace('EveryHours: cfg.CheckinIntervalHours, Cron: cfg.CheckinCron',
                  'EveryHours: rt.CheckinIntervalHours, Cron: rt.CheckinCron')
src = src.replace('return scheduler.CronToSchedule(cfg.CheckinCron)', 'return scheduler.CronToSchedule(rt.CheckinCron)')
open('handler/admin/checkin_schedule.go','w',encoding='utf-8').write(src)
print("checkin_schedule.go updated")
