package config_test

import (
	"crypto/subtle"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// TestRuntimeWriteRace_TornReadReproduction reproduces the Wave 18
// concurrency-audit finding: config.cfgMu only guards pointer publication,
// while runtime writers (handler/admin/settings_apply.go, auth_settings.go,
// scheduler write-backs) mutate fields of the shared *config.Config and
// hot-path readers (auth/downstream.go, auth/admin.go,
// routing/status_ranges.go) dereference those fields lock-free.
//
// Before the fix this test FAILS under -race (DATA RACE on two-word string
// values and slice headers = torn reads). After the fix the same access
// shape goes through the atomic RuntimeSettings snapshot (UpdateRuntime for
// writers, Runtime() for readers) and passes.
func TestRuntimeWriteRace_TornReadReproduction(t *testing.T) {
	cfg := config.Load(map[string]string{
		"AUTH_TOKEN":  "admin-token",
		"PROXY_TOKEN": "sk-initial-token",
	})
	config.Set(cfg)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer: mirrors what handler/admin/settings_apply.go does today
	// (h.cfg.ProxyToken = token, h.cfg.ProxyRetryStatusRanges = spec, ...).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			cfg.ProxyToken = "sk-token-" + strconv.Itoa(i)
			cfg.AuthToken = "admin-token-" + strconv.Itoa(i)
			cfg.ProxyRetryStatusRanges = "500-599"
			cfg.AdminIpAllowlist = []string{"10.0.0.1"}
			cfg.NotifyCooldownSec = i % 100
			cfg.RoutingWeights.CostWeight = 0.4
		}
	}()

	// Readers: mirror auth/downstream.go:161 (every proxy request),
	// auth/admin.go:102 (every admin request), routing/status_ranges.go
	// ActiveRetryStatusRanges spec read, and the AdminAuth allowlist parse.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				c := config.Get()
				_ = subtle.ConstantTimeCompare([]byte("probe-token"), []byte(c.ProxyToken))
				_ = subtle.ConstantTimeCompare([]byte("probe-token"), []byte(c.AuthToken))
				for _, ip := range c.AdminIpAllowlist {
					_ = ip
				}
				_ = c.NotifyCooldownSec
				_ = c.RoutingWeights.CostWeight
				_ = c.ProxyRetryStatusRanges
			}
		}()
	}

	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()
}
