package routing

// First-principles invariants for the channel-selection core. The checked-in
// golden files pin the exact scores of one hand-built fixture set; the targets
// below instead assert the properties that must hold for *any* candidate pool,
// so they keep holding as weights, pricing and health inputs vary:
//
//   - the weighted algorithm always returns a valid probability distribution
//     over the candidates and picks a candidate that is a member of the pool;
//   - the stable_first strategy always selects a member and never a nil slot;
//   - round-robin ordering is a permutation of the input (no candidate added or
//     dropped);
//   - least-busy selection returns a pool member, or nil only for an empty pool.
//
//	go test ./routing -run '^$' -fuzz FuzzWeightedSelectionDistribution -fuzztime=60s

import (
	"math"
	"math/rand"
	"testing"
)

// fuzzCandidates deterministically derives a small candidate pool from the
// fuzz input so one input always maps to one pool.
func fuzzCandidates(input []byte) []RouteChannelCandidate {
	var seed int64
	for _, b := range input {
		seed = seed*131 + int64(b)
	}
	r := rand.New(rand.NewSource(seed))
	n := 1 + r.Intn(6)
	providers := []string{"", "openai", "claude"}
	candidates := make([]RouteChannelCandidate, 0, n)
	for i := 0; i < n; i++ {
		channelID := int64(i + 1)
		siteID := int64(10 + r.Intn(3))
		accountID := int64(100 + i)
		candidates = append(candidates, buildTestCandidate(
			channelID, siteID, accountID,
			int64(r.Intn(20)),           // weight
			int64(r.Intn(3)),            // priority
			int64(r.Intn(50)),           // successCount
			int64(r.Intn(10)),           // failCount
			r.Float64()*0.01,            // totalCost
			0.5+r.Float64()*1.5,         // siteGlobalWeight
			ptrFloat(0.001+r.Float64()), // unitCost
			r.Float64()*500,             // balance
			&providers[r.Intn(len(providers))],
		))
	}
	return candidates
}

func candidateIndex(candidates []RouteChannelCandidate, selected *RouteChannelCandidate) int {
	if selected == nil {
		return -1
	}
	for i := range candidates {
		if candidates[i].Channel.ID == selected.Channel.ID &&
			candidates[i].Site.ID == selected.Site.ID &&
			candidates[i].Account.ID == selected.Account.ID {
			return i
		}
	}
	return -1
}

func FuzzWeightedSelectionDistribution(f *testing.F) {
	f.Add([]byte{1, 2, 3})
	f.Add([]byte{5, 0, 9, 255, 7, 1})
	f.Fuzz(func(t *testing.T, input []byte) {
		candidates := fuzzCandidates(input)
		ResetSiteRuntimeHealthState()

		for _, mode := range []WeightedSelectionMode{WeightedMode, StableFirstMode} {
			res := CalculateWeightedSelection(
				candidates, staticModel("fuzz-model"), defaultRoutingWeights(),
				map[int64]float64{}, 1.0, nil, 0, mode, "fuzz", nil, 1,
			)
			if res.Selected == nil {
				t.Fatalf("mode %s: nil selection for %d candidates", mode, len(candidates))
			}
			if candidateIndex(candidates, res.Selected) < 0 {
				t.Fatalf("mode %s: selected channel %d is not a pool member", mode, res.Selected.Channel.ID)
			}
			if len(res.Details) != len(candidates) {
				t.Fatalf("mode %s: %d probability rows for %d candidates", mode, len(res.Details), len(candidates))
			}
			sum := 0.0
			for _, d := range res.Details {
				if d.Probability < 0 || d.Probability > 1 {
					t.Fatalf("mode %s: probability out of range: %v", mode, d.Probability)
				}
				sum += d.Probability
			}
			if sum > 0 && math.Abs(sum-1) > 1e-6 {
				t.Fatalf("mode %s: probabilities do not sum to 1: %v", mode, sum)
			}
		}
	})
}

func FuzzRoundRobinPermutation(f *testing.F) {
	f.Add([]byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, input []byte) {
		candidates := fuzzCandidates(input)
		ordered := GetRoundRobinCandidates(candidates)
		if len(ordered) != len(candidates) {
			t.Fatalf("round-robin changed the pool size: %d -> %d", len(candidates), len(ordered))
		}
		counts := map[int64]int{}
		for _, c := range candidates {
			counts[c.Channel.ID]++
		}
		for _, c := range ordered {
			counts[c.Channel.ID]--
			if counts[c.Channel.ID] < 0 {
				t.Fatalf("round-robin introduced channel %d", c.Channel.ID)
			}
		}
		for id, n := range counts {
			if n != 0 {
				t.Fatalf("round-robin dropped channel %d", id)
			}
		}
	})
}

func FuzzLeastBusyCandidateIsMember(f *testing.F) {
	f.Add([]byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, input []byte) {
		candidates := fuzzCandidates(input)
		ResetSiteRuntimeHealthState()
		got := SelectLeastBusyCandidate(candidates, nil)
		if len(candidates) == 0 {
			if got != nil {
				t.Fatal("empty pool returned a candidate")
			}
			return
		}
		if candidateIndex(candidates, got) < 0 {
			t.Fatal("least-busy returned a non-member")
		}
	})
}
