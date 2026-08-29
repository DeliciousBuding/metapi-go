import re

# move ModelAvailabilityProbeEnabled Config -> RuntimeSettings
src = open('config/config.go', encoding='utf-8').read()
src = src.replace('''	// Model Probe (4 fields)
	ModelAvailabilityProbeEnabled     bool
	ModelAvailabilityProbeIntervalMs  int''',
'''	// Model Probe (3 fields; ModelAvailabilityProbeEnabled is runtime-mutable
	// and lives in RuntimeSettings)
	ModelAvailabilityProbeIntervalMs  int''', 1)
src = src.replace('\tcfg.ModelAvailabilityProbeEnabled = parseBoolean(get("MODEL_AVAILABILITY_PROBE_ENABLED"), false)\n',
                  '\trt.ModelAvailabilityProbeEnabled = parseBoolean(get("MODEL_AVAILABILITY_PROBE_ENABLED"), false)\n', 1)
open('config/config.go','w',encoding='utf-8').write(src)

rt = open('config/runtime_settings.go', encoding='utf-8').read()
rt = rt.replace('''	RoutingWeights RoutingWeights''',
'''	// Model availability probe kill switch (settings toggle hot-applies the
	// running ticker, scheduler/model_probe.go SetEnabled).
	ModelAvailabilityProbeEnabled bool

	RoutingWeights RoutingWeights''', 1)
open('config/runtime_settings.go','w',encoding='utf-8').write(rt)

# scheduler/daily_summary.go: notify via runtime snapshot
src = open('scheduler/daily_summary.go', encoding='utf-8').read()
src = src.replace('_, err = notifypkg.SendNotification(s.cfg, title, message, string(notifypkg.LevelInfo),',
                  '_, err = notifypkg.SendNotification(config.RuntimeSafe(), title, message, string(notifypkg.LevelInfo),', 1)
open('scheduler/daily_summary.go','w',encoding='utf-8').write(src)
print("probe field moved")
