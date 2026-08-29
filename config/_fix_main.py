src = open('cmd/server/main.go', encoding='utf-8').read()

src = src.replace('''	// ---- 1. Load config ----
	cfg := config.Load(env)
	config.Set(cfg)''',
'''	// ---- 1. Load config ----
	// Load splits env-driven values into the static Config (frozen at boot)
	// and the RuntimeSettings draft (published below, mutable afterwards only
	// through config.UpdateRuntime).
	cfg, rt := config.Load(env)''', 1)

src = src.replace('''	// ---- 1a. Validate config at startup ----
	errs := cfg.Validate()''',
'''	// ---- 1a. Validate config at startup ----
	errs := append(cfg.Validate(), rt.Validate()...)''', 1)

src = src.replace('''	if err := bootstrapRuntime(cfg); err != nil {
		slog.Error("startup bootstrap failed", "error", err)
		os.Exit(1)
	}''',
'''	if err := bootstrapRuntime(cfg, rt); err != nil {
		slog.Error("startup bootstrap failed", "error", err)
		os.Exit(1)
	}
	// Publish both singletons only after hydration: Config is frozen from
	// here on; RuntimeSettings stays hot-updatable via config.UpdateRuntime.
	config.Set(cfg)
	config.SetRuntime(rt)''', 1)

src = src.replace('app.ApplyCacheRatioOverrides(cfg)', 'app.ApplyCacheRatioOverrides()', 1)

src = src.replace('''func bootstrapRuntime(cfg *config.Config) (err error) {''',
                  'func bootstrapRuntime(cfg *config.Config, rt *config.RuntimeSettings) (err error) {', 1)
src = src.replace('if err := store.EnsureRuntimeDatabase(cfg); err != nil {',
                  'if err := store.EnsureRuntimeDatabase(cfg, rt); err != nil {', 1)
src = src.replace('if err := store.LoadRuntimeSettings(cfg); err != nil {',
                  'if err := store.LoadRuntimeSettings(cfg, rt); err != nil {', 1)

open('cmd/server/main.go','w',encoding='utf-8').write(src)
print("main.go updated")
