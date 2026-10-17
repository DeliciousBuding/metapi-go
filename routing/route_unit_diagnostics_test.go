package routing

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestOAuthMemberDiagnosticsMatchExecutionFilters(t *testing.T) {
	model, future, past := "pool-model", "2999-01-01T00:00:00Z", "2000-01-01T00:00:00Z"
	pool := RouteChannelCandidate{
		Channel:   store.RouteChannel{ID: 1, Enabled: true, SourceModel: &model},
		RouteUnit: &OAuthRouteUnitSummary{ID: 7, Name: "Team", Enabled: true},
	}
	for i := int64(1); i <= 7; i++ {
		pool.RouteUnitMembers = append(pool.RouteUnitMembers, makeMember(i, "active", "active", "fixture-access-private", "fixture-api-private"))
	}
	pool.RouteUnitMembers[1].Member.CooldownUntil = &future
	pool.RouteUnitMembers[2].Account.Status = "disabled"
	pool.RouteUnitMembers[3].Site.Status = "disabled"
	pool.RouteUnitMembers[4].Account.AccessToken, pool.RouteUnitMembers[4].Account.APIToken = "", nil
	pool.RouteUnitMembers[5].Member.CooldownUntil = &past
	pool.RouteUnitMembers[6].Member.LastFailAt = &future // Soft recent-failure preference is not a hard blocker.
	rawReason := "fixture-raw-private"
	pool.RouteUnitMembers[1].Member.CooldownReason = &rawReason
	before, _ := json.Marshal(pool)
	selector := &ChannelSelector{}
	for _, tc := range []struct {
		name   string
		policy DownstreamRoutingPolicy
		want   []string
	}{
		{"protocol", DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat}, []string{"eligible", "cooldown", "account_inactive", "site_disabled", "token_unavailable", "eligible", "eligible"}},
		{"no-protocol", EmptyDownstreamRoutingPolicy, []string{"notEvaluated", "cooldown", "account_inactive", "site_disabled", "token_unavailable", "notEvaluated", "notEvaluated"}},
		{"site-policy", DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat, AllowedSiteIDs: []int64{10}}, []string{"eligible", "cooldown,site_not_allowed", "account_inactive,site_not_allowed", "site_disabled,site_not_allowed", "token_unavailable,site_not_allowed", "site_not_allowed", "site_not_allowed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := explainSelectionFromMatch(selector, &RouteMatch{Route: store.TokenRoute{ID: 1}, Channels: []RouteChannelCandidate{pool}}, model, nil, tc.policy, 60)
			candidate := d.Candidates[0]
			members := candidate.OAuthMembers
			if len(members) != len(tc.want) {
				t.Fatalf("members: %+v", members)
			}
			var diagnosticsAllowed []int64
			for i, member := range members {
				var codes []string
				for _, b := range member.Blockers {
					codes = append(codes, b.Code)
				}
				got := strings.Join(codes, ",")
				if len(codes) == 0 {
					got = member.Eligibility
					diagnosticsAllowed = append(diagnosticsAllowed, member.MemberID)
				}
				if got != tc.want[i] {
					t.Fatalf("member %d: %s, want %s", i, got, tc.want[i])
				}
				if len(codes) > 0 && member.Eligibility != "blocked" {
					t.Fatalf("blocked member: %+v", member)
				}
			}
			var executionAllowed []int64
			filtered := selector.filterRouteUnitMembersByPolicy(pool, tc.policy)
			for _, m := range getEligibleRouteUnitMembers(filtered, model, "2026-10-18T00:00:00Z") {
				executionAllowed = append(executionAllowed, m.Member.ID)
			}
			if !reflect.DeepEqual(diagnosticsAllowed, executionAllowed) {
				t.Fatalf("diagnostics=%v execution=%v", diagnosticsAllowed, executionAllowed)
			}
			if candidate.AccountID != 0 || candidate.ActualModel != "" || d.SelectedAccountID != nil {
				t.Fatal("preview selected a member")
			}
			encoded, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"fixture-access-private", "fixture-api-private", "fixture-raw-private"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("private field in diagnostics: %s", secret)
				}
			}
		})
	}
	after, _ := json.Marshal(pool)
	if string(before) != string(after) {
		t.Fatal("diagnostics changed cached members")
	}
}
