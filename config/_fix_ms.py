import re

# model_sync.go: cfg becomes unused -> drop field/param
src = open('scheduler/model_sync.go', encoding='utf-8').read()
src = src.replace('''type ModelSyncScheduler struct {
	cfg        *config.Config
	cronRunner *cronRunner
}

// NewModelSyncScheduler creates a new model sync scheduler.
func NewModelSyncScheduler(cfg *config.Config) *ModelSyncScheduler {
	return &ModelSyncScheduler{cfg: cfg}
}''',
'''type ModelSyncScheduler struct {
	cronRunner *cronRunner
}

// NewModelSyncScheduler creates a new model sync scheduler. The cron state
// lives in the runtime-settings snapshot (hot-updated by settings apply).
func NewModelSyncScheduler() *ModelSyncScheduler {
	return &ModelSyncScheduler{}
}''', 1)
src = src.replace('''	activeCron := resolveCronSetting("model_sync_cron", s.cfg.ModelSyncCron)
	s.cfg.ModelSyncCron = activeCron''',
'''	activeCron := resolveCronSetting("model_sync_cron", config.Runtime().ModelSyncCron)
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ModelSyncCron = activeCron })''', 1)
src = src.replace('	s.cfg.ModelSyncCron = cronExpr\n',
                  '	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ModelSyncCron = cronExpr })\n', 1)
open('scheduler/model_sync.go','w',encoding='utf-8').write(src)

# model_probe.go: enabled flag -> snapshot; interval/timeout/concurrency stay static
src = open('scheduler/model_probe.go', encoding='utf-8').read()
src = src.replace('if !s.cfg.ModelAvailabilityProbeEnabled {',
                  'if !config.Runtime().ModelAvailabilityProbeEnabled {', 1)
src = src.replace('	s.cfg.ModelAvailabilityProbeEnabled = enabled\n',
                  '	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ModelAvailabilityProbeEnabled = enabled })\n', 1)
open('scheduler/model_probe.go','w',encoding='utf-8').write(src)
print("model_sync + model_probe updated")
