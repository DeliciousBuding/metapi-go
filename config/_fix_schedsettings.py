src = open('scheduler/settings.go', encoding='utf-8').read()
src = src.replace('func resolveCheckinScheduleMode(cfg *config.Config) string {',
                  'func resolveCheckinScheduleMode(rt *config.RuntimeSettings) string {', 1)
src = src.replace('return cfg.CheckinScheduleMode', 'return rt.CheckinScheduleMode')
open('scheduler/settings.go','w',encoding='utf-8').write(src)
print("scheduler/settings.go updated")
