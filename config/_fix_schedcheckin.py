src = open('scheduler/checkin.go', encoding='utf-8').read()

# 1. constructor mode
src = src.replace('''	return &CheckinScheduler{
		cfg:              cfg,
		mode:             cfg.CheckinScheduleMode,''',
'''	return &CheckinScheduler{
		cfg:              cfg,
		mode:             config.Runtime().CheckinScheduleMode,''', 1)

# 2. Start block
src = src.replace('''	activeCron := resolveCronSetting("checkin_cron", s.cfg.CheckinCron)
	activeMode := resolveCheckinScheduleMode(s.cfg)
	activeIntervalHours := config.ClampInt(
		resolvePositiveIntegerSetting("checkin_interval_hours", s.cfg.CheckinIntervalHours),
		1, 24,
	)

	// E1: window bounds (HH:mm) hydrate from settings with env defaults.
	s.cfg.CheckinWindowStart = resolveStringSetting("checkin_window_start", s.cfg.CheckinWindowStart)
	s.cfg.CheckinWindowEnd = resolveStringSetting("checkin_window_end", s.cfg.CheckinWindowEnd)

	s.cfg.CheckinCron = activeCron
	s.cfg.CheckinScheduleMode = activeMode
	s.cfg.CheckinIntervalHours = activeIntervalHours
	s.mode = activeMode

	// #1027: global check-in kill switch (env CHECKIN_ENABLED or the
	// checkin_enabled runtime setting). Default true keeps historical
	// behavior; when disabled no schedule is armed at all.
	enabled := resolveBooleanSetting("checkin_enabled", !s.cfg.CheckinDisabled)
	s.cfg.CheckinDisabled = !enabled
	if s.cfg.CheckinDisabled {
		slog.Info("checkin scheduler disabled (checkinEnabled=false)")
		return nil
	}''',
'''	rt := config.Runtime()
	activeCron := resolveCronSetting("checkin_cron", rt.CheckinCron)
	activeMode := resolveCheckinScheduleMode(rt)
	activeIntervalHours := config.ClampInt(
		resolvePositiveIntegerSetting("checkin_interval_hours", rt.CheckinIntervalHours),
		1, 24,
	)

	// E1: window bounds (HH:mm) hydrate from settings with env defaults.
	windowStart := resolveStringSetting("checkin_window_start", rt.CheckinWindowStart)
	windowEnd := resolveStringSetting("checkin_window_end", rt.CheckinWindowEnd)

	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.CheckinCron = activeCron
		r.CheckinScheduleMode = activeMode
		r.CheckinIntervalHours = activeIntervalHours
		r.CheckinWindowStart = windowStart
		r.CheckinWindowEnd = windowEnd
	})
	s.mode = activeMode

	// #1027: global check-in kill switch (env CHECKIN_ENABLED or the
	// checkin_enabled runtime setting). Default true keeps historical
	// behavior; when disabled no schedule is armed at all.
	enabled := resolveBooleanSetting("checkin_enabled", !rt.CheckinDisabled)
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CheckinDisabled = !enabled })
	if !enabled {
		slog.Info("checkin scheduler disabled (checkinEnabled=false)")
		return nil
	}''', 1)

# 3. startLocked window mode
src = src.replace('''	if s.mode == "window" {
		expr, err := RandomCronInWindow(s.cfg.CheckinWindowStart, s.cfg.CheckinWindowEnd)
		if err != nil {
			slog.Error("checkin: invalid window bounds, falling back to cron", "error", err, "start", s.cfg.CheckinWindowStart, "end", s.cfg.CheckinWindowEnd)
			s.mode = "cron"
		} else {
			s.cfg.CheckinCron = expr
			s.cronRunner = newCronRunner()
			_, err := s.cronRunner.addJob(expr, s.runCronJob)
			if err != nil {
				slog.Error("checkin: failed to add window cron job", "error", err)
				return
			}
			s.cronRunner.start()
			slog.Info("checkin: window mode armed", "cron", expr, "windowStart", s.cfg.CheckinWindowStart, "windowEnd", s.cfg.CheckinWindowEnd)
			s.maybeCatchUpCheckin()
			return
		}
	}

	// Cron mode
	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(s.cfg.CheckinCron, s.runCronJob)''',
'''	wrt := config.Runtime()
	if s.mode == "window" {
		expr, err := RandomCronInWindow(wrt.CheckinWindowStart, wrt.CheckinWindowEnd)
		if err != nil {
			slog.Error("checkin: invalid window bounds, falling back to cron", "error", err, "start", wrt.CheckinWindowStart, "end", wrt.CheckinWindowEnd)
			s.mode = "cron"
		} else {
			config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CheckinCron = expr })
			s.cronRunner = newCronRunner()
			_, err := s.cronRunner.addJob(expr, s.runCronJob)
			if err != nil {
				slog.Error("checkin: failed to add window cron job", "error", err)
				return
			}
			s.cronRunner.start()
			slog.Info("checkin: window mode armed", "cron", expr, "windowStart", wrt.CheckinWindowStart, "windowEnd", wrt.CheckinWindowEnd)
			s.maybeCatchUpCheckin()
			return
		}
	}

	// Cron mode
	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(config.Runtime().CheckinCron, s.runCronJob)''', 1)

# 4. maybeCatchUpCheckin
src = src.replace('''	if shouldCatchUpCheckin(now, s.cfg.CheckinCron, ranToday > 0, enabled) {
		slog.Info("checkin: missed today's scheduled time, compensating with immediate run", "spec", s.cfg.CheckinCron)''',
'''	if shouldCatchUpCheckin(now, config.Runtime().CheckinCron, ranToday > 0, enabled) {
		slog.Info("checkin: missed today's scheduled time, compensating with immediate run", "spec", config.Runtime().CheckinCron)''', 1)

# 5. UpdateCheckinSchedule
src = src.replace('''	s.mode = mode
	if mode == "cron" {
		s.cfg.CheckinCron = cronExpr
	}
	if mode == "window" {
		s.cfg.CheckinWindowStart = windowStart
		s.cfg.CheckinWindowEnd = windowEnd
	}
	s.cfg.CheckinScheduleMode = mode
	s.cfg.CheckinIntervalHours = config.ClampInt(intervalHours, 1, 24)
	// #1027: a schedule update while check-in is globally disabled must
	// persist the new schedule without silently re-arming the runner.
	if s.cfg.CheckinDisabled {
		return nil
	}''',
'''	s.mode = mode
	clampedHours := config.ClampInt(intervalHours, 1, 24)
	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		if mode == "cron" {
			r.CheckinCron = cronExpr
		}
		if mode == "window" {
			r.CheckinWindowStart = windowStart
			r.CheckinWindowEnd = windowEnd
		}
		r.CheckinScheduleMode = mode
		r.CheckinIntervalHours = clampedHours
	})
	// #1027: a schedule update while check-in is globally disabled must
	// persist the new schedule without silently re-arming the runner.
	if config.Runtime().CheckinDisabled {
		return nil
	}''', 1)

# 6. SetEnabled
src = src.replace('''	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.CheckinDisabled = !enabled
	if !enabled {''',
'''	s.mu.Lock()
	defer s.mu.Unlock()
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CheckinDisabled = !enabled })
	if !enabled {''', 1)

# 7. filterDue interval read
src = src.replace('intervalHours := config.ClampInt(s.cfg.CheckinIntervalHours, 1, 24)',
                  'intervalHours := config.ClampInt(config.Runtime().CheckinIntervalHours, 1, 24)', 1)

open('scheduler/checkin.go','w',encoding='utf-8').write(src)
print("checkin.go updated")
