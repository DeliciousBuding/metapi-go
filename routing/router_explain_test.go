package routing

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func explanationDirectCandidate() RouteChannelCandidate {
	return RouteChannelCandidate{
		Channel: store.RouteChannel{ID: -31, Enabled: true},
		Direct: &store.DirectUpstreamCandidate{
			ItemID: 31, GroupID: 21, GrantID: 17, ChannelID: 11, CredentialID: 13,
			ChannelName: "Primary", CredentialName: "Developer key", ModelName: "provider-model",
			ChannelEnabled: true, CredentialEnabled: true, GrantEnabled: true, ModelEnabled: true,
			Protocols: store.DirectProtocolChat, CredentialKind: store.DirectCredentialAPIKey,
			Credential: "fixture-private-value", CustomHeader: "fixture-private-header",
			BaseURL: "https://private-fixture.example", Endpoints: store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "https://fixture.example/v1/chat/completions", Auth: "bearer"}},
		},
	}
}

func TestExplanationDirectProtocolContextAndExecutionFailClosed(t *testing.T) {
	row := explanationDirectCandidate()
	selector := &ChannelSelector{}
	match := &RouteMatch{Route: store.TokenRoute{ID: 1, ModelPattern: "coding"}, Channels: []RouteChannelCandidate{row}}
	for _, tc := range []struct {
		name       string
		protocol   int
		conversion bool
		want       string
	}{
		{"missing", 0, false, "notEvaluated"},
		{"native-chat", store.DirectProtocolChat, false, "eligible"},
		{"native-responses-denied", store.DirectProtocolResponses, false, "blocked"},
		{"responses-conversion", store.DirectProtocolResponses, true, "eligible"},
		{"embeddings-never-converts", store.DirectProtocolEmbeddings, true, "blocked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := DownstreamRoutingPolicy{RequiredUpstreamProtocol: tc.protocol, AllowUpstreamProtocolConversion: tc.conversion}
			d := explainSelectionFromMatch(selector, match, "coding", nil, policy, 60)
			c := d.Candidates[0]
			if c.Eligibility != tc.want || c.Eligible != (tc.want == "eligible") {
				t.Fatalf("candidate status: %+v", c)
			}
			if tc.want == "blocked" && (len(c.Blockers) != 1 || c.Blockers[0].Code != "protocol_not_authorized") {
				t.Fatalf("missing protocol blocker: %+v", c)
			}
			if tc.protocol == 0 {
				if len(c.Blockers) != 0 || strings.Contains(c.Reason, "not authorized") {
					t.Fatalf("missing context misreported as denial: %+v", c)
				}
				if len(selector.getCandidateEligibilityReasons(row, "coding", false, nil, "", policy)) == 0 {
					t.Fatal("execution must still fail closed without protocol")
				}
			}
			if c.CandidateRef.Kind != "direct" || c.CandidateRef.ID != 31 || c.CandidateRef.GrantID != 17 || c.CandidateRef.ChannelID != 11 || c.CandidateRef.CredentialID != 13 || c.ActualModel != "provider-model" || c.Username != "" || c.SiteName != "Primary" {
				t.Fatalf("unsafe or incorrect direct identity: %+v", c)
			}
			if c.ProbabilityEvaluated || d.SelectedChannelID != nil || d.SelectedAccountID != nil {
				t.Fatal("preview pretended to select a winner")
			}
			encoded, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{"fixture-private-value", "fixture-private-header", "private-fixture.example", "account-0"} {
				if strings.Contains(string(encoded), private) {
					t.Fatalf("private runtime field escaped: %s", private)
				}
			}
		})
	}
}

func TestExplanationKnownBlockerSurvivesMissingProtocol(t *testing.T) {
	row := explanationDirectCandidate()
	row.Direct.GrantEnabled = false
	d := explainSelectionFromMatch(&ChannelSelector{}, &RouteMatch{Route: store.TokenRoute{ID: 1}, Channels: []RouteChannelCandidate{row}}, "coding", nil, EmptyDownstreamRoutingPolicy, 60)
	c := d.Candidates[0]
	if c.Eligibility != "blocked" || len(c.Blockers) != 1 || c.Blockers[0].Code != "disabled" {
		t.Fatalf("lost known blocker: %+v", c)
	}
}

func TestExplanationAccountAndOAuthIdentityWithoutMemberSelection(t *testing.T) {
	model, user, credential := "gpt-test", "alice", "fixture-account-private"
	unitID := int64(7)
	account := RouteChannelCandidate{
		Channel: store.RouteChannel{ID: 5, AccountID: 3, SourceModel: &model, Enabled: true},
		Account: store.Account{ID: 3, Username: &user, Status: "active", APIToken: &credential},
		Site:    store.Site{ID: 2, Name: "Site A", Status: "active"},
	}
	unit := RouteChannelCandidate{
		Channel:          store.RouteChannel{ID: 9, OAuthRouteUnitID: &unitID, SourceModel: &model, Enabled: true},
		RouteUnit:        &OAuthRouteUnitSummary{ID: 7, Name: "Team pool", Enabled: true},
		RouteUnitMembers: []OAuthRouteUnitMemberCandidate{{Account: store.Account{ID: 12, Status: "active", AccessToken: credential}}},
	}
	d := explainSelectionFromMatch(&ChannelSelector{}, &RouteMatch{Route: store.TokenRoute{ID: 1, ModelPattern: model}, Channels: []RouteChannelCandidate{account, unit}}, model, nil, DownstreamRoutingPolicy{RequiredUpstreamProtocol: store.DirectProtocolChat}, 60)
	if d.Candidates[0].CandidateRef.Kind != "account" || d.Candidates[0].CandidateRef.AccountID != 3 || d.Candidates[0].ActualModel != model {
		t.Fatalf("account: %+v", d.Candidates[0])
	}
	if d.Candidates[1].CandidateRef.Kind != "oauth_unit" || d.Candidates[1].CandidateRef.ID != 7 || d.Candidates[1].Label != "Team pool" || d.Candidates[1].ActualModel != "" || d.Candidates[1].AccountID != 0 {
		t.Fatalf("OAuth: %+v", d.Candidates[1])
	}
}
