package config_test

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// C1 (Wave 21 config-race): the runtime-mutable settings live on an
// immutable RuntimeSettings snapshot published through an atomic.Pointer.
// Writers go through config.UpdateRuntime (copy-mutate-publish under a
// mutex); readers take one atomic snapshot via config.Runtime() and never
// lock. These tests pin that contract:
//
//  1. hot updates become visible without a restart (and never disturb
//     previously handed-out snapshots — copy-on-write),
//  2. under -race, N readers x M concurrent writers only ever observe
//     complete old/new snapshots — never a torn mix of two updates
//     (covers ProxyToken and ProxyRetryStatusRanges, the two hot-path
//     fields behind proxy auth and the retry status-range policy).

func publishRaceBaseline(t *testing.T) {
	t.Helper()
	config.SetRuntime(&config.RuntimeSettings{
		AuthToken:              "admin-token-100",
		ProxyToken:             "sk-gen-100",
		ProxyRetryStatusRanges: "100-100",
		NotifyCooldownSec:      100,
		AdminIpAllowlist:       []string{"10.0.0.1"},
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
}

func generationToken(g int) string  { return "sk-gen-" + strconv.Itoa(g) }
func generationRanges(g int) string { return strconv.Itoa(g) + "-" + strconv.Itoa(g) }

// TestUpdateRuntimeHotApply is the hot-update application test: a settings
// write publishes immediately, subsequent reads see it, and a snapshot
// handed out before the write keeps the old values (immutability).
func TestUpdateRuntimeHotApply(t *testing.T) {
	config.SetRuntime(&config.RuntimeSettings{
		AuthToken:              "admin-token-old",
		ProxyToken:             "sk-old-token",
		ProxyRetryStatusRanges: "500-500",
	})
	t.Cleanup(func() { config.SetRuntime(nil) })

	before := config.Runtime()

	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.ProxyToken = "sk-new-token"
		r.ProxyRetryStatusRanges = "500-599"
	})

	after := config.Runtime()
	if after.ProxyToken != "sk-new-token" {
		t.Fatalf("ProxyToken after hot update = %q, want sk-new-token", after.ProxyToken)
	}
	if after.ProxyRetryStatusRanges != "500-599" {
		t.Fatalf("ProxyRetryStatusRanges after hot update = %q, want 500-599", after.ProxyRetryStatusRanges)
	}
	// Untouched fields carry over from the previous snapshot.
	if after.AuthToken != "admin-token-old" {
		t.Fatalf("AuthToken = %q, want carry-over admin-token-old", after.AuthToken)
	}

	// Copy-on-write: the pre-update snapshot still observes the old values.
	if before.ProxyToken != "sk-old-token" {
		t.Fatalf("pre-update snapshot ProxyToken mutated: %q", before.ProxyToken)
	}
	if before == after {
		t.Fatal("UpdateRuntime must publish a fresh snapshot, not reuse the old pointer")
	}
}

// TestSetRuntimePublishesCopies pins the SetRuntime copy contract: mutating
// the caller's struct after publication must not leak into the snapshot.
func TestSetRuntimePublishesCopies(t *testing.T) {
	src := &config.RuntimeSettings{ProxyToken: "sk-published"}
	config.SetRuntime(src)
	t.Cleanup(func() { config.SetRuntime(nil) })

	src.ProxyToken = "sk-mutated-after-publish"

	if got := config.Runtime().ProxyToken; got != "sk-published" {
		t.Fatalf("published snapshot leaked caller mutation: %q", got)
	}
}

// TestRuntimeWriteRace_TornReadReproduction reproduces the Wave 18
// concurrency-audit finding against the FIXED access shape: writers publish
// correlated field tuples through UpdateRuntime while readers take atomic
// snapshots. Each published generation g carries ProxyToken "sk-gen-g",
// ProxyRetryStatusRanges "g-g" and NotifyCooldownSec g; a reader that ever
// observed fields from two different generations would have torn a read.
// Before the atomic-snapshot fix the same access shape on the shared
// *config.Config failed under -race (DATA RACE on two-word string values =
// torn reads); now it must pass.
func TestRuntimeWriteRace_TornReadReproduction(t *testing.T) {
	publishRaceBaseline(t)

	const (
		writers      = 3
		readers      = 4
		runFor       = 300 * time.Millisecond
		startGen     = 100
		allowlistLen = 1
	)

	var gen int64 = startGen
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writers: mirror handler/admin/settings_apply.go hot updates —
	// ProxyToken rotation, ProxyRetryStatusRanges spec changes, allowlist
	// and cooldown tweaks — all through the single UpdateRuntime gate.
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				g := int(atomicAddGeneration(&gen))
				token := generationToken(g)
				ranges := generationRanges(g)
				allowlist := []string{"10.0.0." + strconv.Itoa(g%250+1)}
				cooldown := g
				config.UpdateRuntime(func(r *config.RuntimeSettings) {
					r.ProxyToken = token
					r.ProxyRetryStatusRanges = ranges
					r.AdminIpAllowlist = allowlist
					r.NotifyCooldownSec = cooldown
				})
			}
		}()
	}

	// Readers: mirror auth/downstream.go (every proxy request),
	// routing/status_ranges.go spec reads and the AdminAuth allowlist
	// parse — lock-free snapshot reads only.
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				rt := config.Runtime()

				// ProxyToken carries the generation.
				if !strings.HasPrefix(rt.ProxyToken, "sk-gen-") {
					t.Errorf("torn read: ProxyToken %q has no generation prefix", rt.ProxyToken)
					return
				}
				g, err := strconv.Atoi(strings.TrimPrefix(rt.ProxyToken, "sk-gen-"))
				if err != nil {
					t.Errorf("torn read: ProxyToken %q generation does not parse: %v", rt.ProxyToken, err)
					return
				}
				// Every correlated field must come from the SAME generation:
				// a mix would mean the reader observed a half-applied update.
				if want := generationRanges(g); rt.ProxyRetryStatusRanges != want {
					t.Errorf("torn read: ProxyRetryStatusRanges %q does not match ProxyToken generation %d (want %q)", rt.ProxyRetryStatusRanges, g, want)
					return
				}
				if rt.NotifyCooldownSec != g {
					t.Errorf("torn read: NotifyCooldownSec %d does not match ProxyToken generation %d", rt.NotifyCooldownSec, g)
					return
				}
				if len(rt.AdminIpAllowlist) != allowlistLen {
					t.Errorf("torn read: AdminIpAllowlist length %d, want %d", len(rt.AdminIpAllowlist), allowlistLen)
					return
				}
			}
		}()
	}

	time.Sleep(runFor)
	close(stop)
	wg.Wait()

	// Progress sanity: the writers must actually have published new
	// generations, otherwise the test would pass vacuously.
	if final := atomicLoadGeneration(&gen); final <= startGen {
		t.Fatalf("no generations published during the race window (final %d)", final)
	}
}

// atomicAddGeneration / atomicLoadGeneration keep the shared generation
// counter race-free across the concurrent writer goroutines.
func atomicAddGeneration(p *int64) int64  { return atomic.AddInt64(p, 1) }
func atomicLoadGeneration(p *int64) int64 { return atomic.LoadInt64(p) }
