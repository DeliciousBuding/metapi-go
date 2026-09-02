package config

import "testing"

// TestLoadLogCleanupEnvIntent pins which env spellings count as an explicit ask
// for the log_cleanup retention regime. store.LoadRuntimeSettings ORs this bit
// into Config.LogCleanupConfigured, so it has to stay one-directional:
//
//   - a toggle set to true claims the regime (an env-only deployment has no
//     admin-saved log-cleanup rows to claim it with);
//   - LOG_CLEANUP_RETENTION_DAYS / LOG_CLEANUP_CRON do not, and neither does an
//     explicit false. Claiming the regime while both toggles are off would run
//     the new scheduler (which then skips for want of a target) and disable the
//     legacy PROXY_LOG_RETENTION_DAYS pruner, so nothing would ever be pruned.
func TestLoadLogCleanupEnvIntent(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "unset", env: nil, want: false},
		{
			name: "both toggles explicit false",
			env: map[string]string{
				"LOG_CLEANUP_USAGE_LOGS_ENABLED":   "false",
				"LOG_CLEANUP_PROGRAM_LOGS_ENABLED": "false",
			},
			want: false,
		},
		{name: "usage toggle true", env: map[string]string{"LOG_CLEANUP_USAGE_LOGS_ENABLED": "true"}, want: true},
		{name: "program toggle true", env: map[string]string{"LOG_CLEANUP_PROGRAM_LOGS_ENABLED": "true"}, want: true},
		{
			name: "truthy spelling",
			env:  map[string]string{"LOG_CLEANUP_USAGE_LOGS_ENABLED": "1"},
			want: true,
		},
		{
			name: "one toggle true, one false",
			env: map[string]string{
				"LOG_CLEANUP_USAGE_LOGS_ENABLED":   "true",
				"LOG_CLEANUP_PROGRAM_LOGS_ENABLED": "false",
			},
			want: true,
		},
		{
			name: "retention only",
			env:  map[string]string{"LOG_CLEANUP_RETENTION_DAYS": "7"},
			want: false,
		},
		{
			name: "cron only",
			env:  map[string]string{"LOG_CLEANUP_CRON": "30 4 * * *"},
			want: false,
		},
		{
			name: "retention and cron",
			env: map[string]string{
				"LOG_CLEANUP_RETENTION_DAYS": "7",
				"LOG_CLEANUP_CRON":           "30 4 * * *",
			},
			want: false,
		},
		{
			name: "unparsable toggle",
			env:  map[string]string{"LOG_CLEANUP_USAGE_LOGS_ENABLED": "maybe"},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, rt := Load(tc.env)
			if cfg.LogCleanupEnvEnabled != tc.want {
				t.Fatalf("LogCleanupEnvEnabled = %v, want %v", cfg.LogCleanupEnvEnabled, tc.want)
			}
			// The regime itself is resolved during settings hydration, never here.
			if cfg.LogCleanupConfigured {
				t.Error("LogCleanupConfigured = true straight out of Load; hydration owns that decision")
			}
			// The bit must never contradict the toggles it is derived from.
			if tc.want && !rt.LogCleanupUsageLogsEnabled && !rt.LogCleanupProgramLogsEnabled {
				t.Error("LogCleanupEnvEnabled = true but no toggle is enabled")
			}
		})
	}
}
