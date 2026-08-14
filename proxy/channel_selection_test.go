package proxy

import (
	"context"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Mock types ----

type mockRouter struct {
	selectChannel          func(ctx context.Context, requestedModel string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error)
	selectNextChannel      func(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error)
	selectPreferredChannel func(ctx context.Context, requestedModel string, preferredChannelID int64, policy routing.DownstreamRoutingPolicy, excludeChannelIDs []int64) (*routing.SelectedChannel, error)
}

func (m *mockRouter) SelectChannel(ctx context.Context, requestedModel string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	if m.selectChannel != nil {
		return m.selectChannel(ctx, requestedModel, policy)
	}
	return nil, nil
}

func (m *mockRouter) SelectNextChannel(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	if m.selectNextChannel != nil {
		return m.selectNextChannel(ctx, requestedModel, excludeChannelIDs, policy)
	}
	return nil, nil
}

func (m *mockRouter) SelectPreferredChannel(ctx context.Context, requestedModel string, preferredChannelID int64, policy routing.DownstreamRoutingPolicy, excludeChannelIDs []int64) (*routing.SelectedChannel, error) {
	if m.selectPreferredChannel != nil {
		return m.selectPreferredChannel(ctx, requestedModel, preferredChannelID, policy, excludeChannelIDs)
	}
	return nil, nil
}

func (m *mockRouter) RecordSuccess(ctx context.Context, channelID int64, latencyMs float64, cost float64, modelName *string, actualAccountID *int64) error {
	return nil
}
func (m *mockRouter) RecordFailure(ctx context.Context, channelID int64, failureCtx routing.SiteRuntimeFailureContext, actualAccountID *int64) error {
	return nil
}

type mockRouteRefresher struct{}

func (m *mockRouteRefresher) RefreshModelsAndRebuildRoutes(ctx context.Context) error {
	return nil
}

// ---- Helpers ----

func makeChannel(channelID, routeID, accountID int64) routing.SelectedChannel {
	return routing.SelectedChannel{
		ActualModel: "gpt-4",
		Channel: store.RouteChannel{
			ID:      channelID,
			RouteID: routeID,
		},
		Account: store.Account{
			ID: accountID,
		},
		Site: store.Site{
			ID:       1,
			Name:     "test-site",
			URL:      "https://api.example.com",
			Platform: "openai",
		},
	}
}

func setupCfg() {
	cfg := config.Load(map[string]string{
		"PORT": "8080",
	})
	config.Set(cfg)
}

// ---- Tests ----

func TestTesterHelpers(t *testing.T) {
	t.Run("IsLoopbackClientIP", func(t *testing.T) {
		tests := []struct {
			ip       string
			expected bool
		}{
			{"127.0.0.1", true},
			{"::1", true},
			{"::ffff:127.0.0.1", true},
			{"192.168.1.1", false},
			{"10.0.0.1", false},
			{"", false},
			{"::ffff:192.168.1.1", false},
		}
		for _, tt := range tests {
			if got := IsLoopbackClientIP(tt.ip); got != tt.expected {
				t.Errorf("IsLoopbackClientIP(%q) = %v, want %v", tt.ip, got, tt.expected)
			}
		}
	})

	t.Run("IsTrustedTesterRequest", func(t *testing.T) {
		t.Run("loopback with correct header", func(t *testing.T) {
			headers := map[string]string{"x-metapi-tester-request": "1"}
			if !IsTrustedTesterRequest(headers, "127.0.0.1") {
				t.Error("expected trusted tester request")
			}
		})

		t.Run("loopback without header", func(t *testing.T) {
			headers := map[string]string{}
			if IsTrustedTesterRequest(headers, "127.0.0.1") {
				t.Error("expected NOT trusted (no header)")
			}
		})

		t.Run("non-loopback with header", func(t *testing.T) {
			headers := map[string]string{"x-metapi-tester-request": "1"}
			if IsTrustedTesterRequest(headers, "192.168.1.1") {
				t.Error("expected NOT trusted (non-loopback)")
			}
		})

		t.Run("loopback with wrong header value", func(t *testing.T) {
			headers := map[string]string{"x-metapi-tester-request": "0"}
			if IsTrustedTesterRequest(headers, "127.0.0.1") {
				t.Error("expected NOT trusted (header != 1)")
			}
		})

		t.Run("case insensitive header", func(t *testing.T) {
			headers := map[string]string{"X-Metapi-Tester-Request": "1"}
			if !IsTrustedTesterRequest(headers, "127.0.0.1") {
				t.Error("expected trusted (case-insensitive)")
			}
		})
	})

	t.Run("GetTesterForcedChannelID", func(t *testing.T) {
		t.Run("valid forced channel", func(t *testing.T) {
			headers := map[string]string{
				"x-metapi-tester-request":        "1",
				"x-metapi-tester-forced-channel-id": "42",
			}
			id := GetTesterForcedChannelID(headers, "127.0.0.1")
			if id == nil || *id != 42 {
				t.Errorf("expected channel 42, got %v", id)
			}
		})

		t.Run("invalid channel ID", func(t *testing.T) {
			headers := map[string]string{
				"x-metapi-tester-request":        "1",
				"x-metapi-tester-forced-channel-id": "abc",
			}
			id := GetTesterForcedChannelID(headers, "127.0.0.1")
			if id != nil {
				t.Errorf("expected nil for invalid ID, got %v", *id)
			}
		})

		t.Run("zero channel ID", func(t *testing.T) {
			headers := map[string]string{
				"x-metapi-tester-request":        "1",
				"x-metapi-tester-forced-channel-id": "0",
			}
			id := GetTesterForcedChannelID(headers, "127.0.0.1")
			if id != nil {
				t.Errorf("expected nil for zero ID, got %v", *id)
			}
		})

		t.Run("negative channel ID", func(t *testing.T) {
			headers := map[string]string{
				"x-metapi-tester-request":        "1",
				"x-metapi-tester-forced-channel-id": "-1",
			}
			id := GetTesterForcedChannelID(headers, "127.0.0.1")
			if id != nil {
				t.Errorf("expected nil for negative ID, got %v", *id)
			}
		})

		t.Run("non-loopback", func(t *testing.T) {
			headers := map[string]string{
				"x-metapi-tester-request":        "1",
				"x-metapi-tester-forced-channel-id": "42",
			}
			id := GetTesterForcedChannelID(headers, "192.168.1.1")
			if id != nil {
				t.Error("expected nil for non-loopback IP")
			}
		})

		t.Run("missing tester request header", func(t *testing.T) {
			headers := map[string]string{
				"x-metapi-tester-forced-channel-id": "42",
			}
			id := GetTesterForcedChannelID(headers, "127.0.0.1")
			if id != nil {
				t.Error("expected nil when tester request header missing")
			}
		})
	})

	t.Run("BuildForcedChannelUnavailableMessage", func(t *testing.T) {
		channelID := int64(42)
		msg := BuildForcedChannelUnavailableMessage(&channelID)
		if msg == "" {
			t.Error("expected non-empty message")
		}

		msgNil := BuildForcedChannelUnavailableMessage(nil)
		if msgNil != "No available channels for this model" {
			t.Errorf("unexpected message for nil: %s", msgNil)
		}

		zeroID := int64(0)
		msgZero := BuildForcedChannelUnavailableMessage(&zeroID)
		if msgZero != "No available channels for this model" {
			t.Errorf("unexpected message for zero: %s", msgZero)
		}
	})

	t.Run("CanRetryChannelSelection", func(t *testing.T) {
		t.Run("no forced channel, retries remaining", func(t *testing.T) {
			if !CanRetryChannelSelection(0, 2, nil) {
				t.Error("expected can retry with retries remaining")
			}
		})

		t.Run("no forced channel, no retries remaining", func(t *testing.T) {
			if CanRetryChannelSelection(2, 2, nil) {
				t.Error("expected cannot retry (maxRetries reached)")
			}
		})

		t.Run("forced channel set", func(t *testing.T) {
			fc := int64(42)
			if CanRetryChannelSelection(0, 2, &fc) {
				t.Error("expected cannot retry with forced channel")
			}
		})

		t.Run("forced channel zero", func(t *testing.T) {
			fc := int64(0)
			if !CanRetryChannelSelection(0, 2, &fc) {
				t.Error("expected can retry with zero forced channel (treated as no forced)")
			}
		})
	})
}

func TestSelectProxyChannelForAttempt(t *testing.T) {
	setupCfg()
	ctx := context.Background()
	defaultPolicy := routing.EmptyDownstreamRoutingPolicy

	coord := NewProxyChannelCoordinator(config.Get())

	t.Run("tester forced channel - first attempt", func(t *testing.T) {
		forcedID := int64(42)
		router := &mockRouter{
			selectPreferredChannel: func(ctx context.Context, requestedModel string, preferredChannelID int64, policy routing.DownstreamRoutingPolicy, excludeChannelIDs []int64) (*routing.SelectedChannel, error) {
				if preferredChannelID == 42 {
					ch := makeChannel(42, 1, 100)
					return &ch, nil
				}
				return nil, nil
			},
		}

		refresher := &mockRouteRefresher{}
		selected, err := SelectProxyChannelForAttempt(ctx, router, coord, refresher, ChannelSelectionInput{
			RequestedModel:   "gpt-4",
			DownstreamPolicy: defaultPolicy,
			RetryCount:       0,
			ForcedChannelID:  &forcedID,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected == nil {
			t.Fatal("expected selected channel")
		}
		if selected.Channel.ID != 42 {
			t.Errorf("expected channel 42, got %d", selected.Channel.ID)
		}
	})

	t.Run("tester forced channel - retryCount > 0 returns nil immediately", func(t *testing.T) {
		forcedID := int64(42)
		router := &mockRouter{}
		refresher := &mockRouteRefresher{}
		selected, err := SelectProxyChannelForAttempt(ctx, router, coord, refresher, ChannelSelectionInput{
			RequestedModel:   "gpt-4",
			DownstreamPolicy: defaultPolicy,
			RetryCount:       1,
			ForcedChannelID:  &forcedID,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected != nil {
			t.Error("expected nil for forced channel on retry > 0")
		}
	})

	t.Run("normal selection - first attempt via SelectChannel", func(t *testing.T) {
		router := &mockRouter{
			selectChannel: func(ctx context.Context, requestedModel string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
				ch := makeChannel(99, 2, 200)
				return &ch, nil
			},
		}
		refresher := &mockRouteRefresher{}

		selected, err := SelectProxyChannelForAttempt(ctx, router, coord, refresher, ChannelSelectionInput{
			RequestedModel:   "gpt-4",
			DownstreamPolicy: defaultPolicy,
			RetryCount:       0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected == nil {
			t.Fatal("expected selected channel")
		}
		if selected.Channel.ID != 99 {
			t.Errorf("expected channel 99, got %d", selected.Channel.ID)
		}
	})

	t.Run("normal selection - retry via SelectNextChannel", func(t *testing.T) {
		exclude := []int64{1, 2}
		var capturedExclude []int64
		router := &mockRouter{
			selectNextChannel: func(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
				capturedExclude = excludeChannelIDs
				ch := makeChannel(100, 3, 300)
				return &ch, nil
			},
		}
		refresher := &mockRouteRefresher{}

		selected, err := SelectProxyChannelForAttempt(ctx, router, coord, refresher, ChannelSelectionInput{
			RequestedModel:    "gpt-4",
			DownstreamPolicy:  defaultPolicy,
			RetryCount:        1,
			ExcludeChannelIDs: exclude,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected == nil {
			t.Fatal("expected selected channel")
		}
		if selected.Channel.ID != 100 {
			t.Errorf("expected channel 100, got %d", selected.Channel.ID)
		}
		if len(capturedExclude) != 2 || capturedExclude[0] != 1 || capturedExclude[1] != 2 {
			t.Errorf("excluded channels not passed correctly: %v", capturedExclude)
		}
	})
}

func TestSelectProxyChannelForAttempt_RetrySelectionOnEmpty(t *testing.T) {
	setupCfg()
	ctx := context.Background()
	coord := NewProxyChannelCoordinator(config.Get())

	t.Run("first attempt retries selection once when empty", func(t *testing.T) {
		count := 0
		refresher := &mockRouteRefresher{}
		router := &mockRouter{
			selectChannel: func(ctx context.Context, requestedModel string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
				count++
				if count == 1 {
					return nil, nil
				}
				ch := makeChannel(101, 5, 500)
				return &ch, nil
			},
		}

		selected, err := SelectProxyChannelForAttempt(ctx, router, coord, refresher, ChannelSelectionInput{
			RequestedModel:   "gpt-4",
			DownstreamPolicy: routing.EmptyDownstreamRoutingPolicy,
			RetryCount:       0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected == nil {
			t.Fatal("expected selected channel on second selection attempt")
		}
		if selected.Channel.ID != 101 {
			t.Errorf("expected channel 101, got %d", selected.Channel.ID)
		}
		if count != 2 {
			t.Errorf("expected 2 select channel calls, got %d", count)
		}
	})

	t.Run("no retry selection on retry > 0", func(t *testing.T) {
		refresher := &mockRouteRefresher{}
		nextCalls := 0
		router := &mockRouter{
			selectNextChannel: func(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
				nextCalls++
				return nil, nil
			},
		}

		selected, _ := SelectProxyChannelForAttempt(ctx, router, coord, refresher, ChannelSelectionInput{
			RequestedModel:   "gpt-4",
			DownstreamPolicy: routing.EmptyDownstreamRoutingPolicy,
			RetryCount:       1,
		})
		if selected != nil {
			t.Error("expected nil selection on retry")
		}
		if nextCalls != 1 {
			t.Errorf("expected exactly 1 SelectNextChannel call, got %d", nextCalls)
		}
	})

	t.Run("nil refresher still selects on second attempt", func(t *testing.T) {
		count := 0
		router := &mockRouter{
			selectChannel: func(ctx context.Context, requestedModel string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
				count++
				if count == 1 {
					return nil, nil
				}
				ch := makeChannel(200, 6, 600)
				return &ch, nil
			},
		}

		selected, err := SelectProxyChannelForAttempt(ctx, router, coord, nil, ChannelSelectionInput{
			RequestedModel:   "gpt-4",
			DownstreamPolicy: routing.EmptyDownstreamRoutingPolicy,
			RetryCount:       0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected == nil {
			t.Fatal("expected selected channel even without refresher")
		}
	})
}

func TestSelectProxyChannelForAttempt_ErrorPropagation(t *testing.T) {
	setupCfg()
	ctx := context.Background()
	defaultPolicy := routing.EmptyDownstreamRoutingPolicy
	coord := NewProxyChannelCoordinator(config.Get())

	t.Run("nil selection with no available channels", func(t *testing.T) {
		router := &mockRouter{
			selectChannel: func(ctx context.Context, requestedModel string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
				return nil, nil
			},
		}

		selected, err := SelectProxyChannelForAttempt(ctx, router, coord, &mockRouteRefresher{}, ChannelSelectionInput{
			RequestedModel:   "gpt-4",
			DownstreamPolicy: defaultPolicy,
			RetryCount:       0,
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if selected != nil {
			t.Error("expected nil selected when all channels exhausted")
		}
	})
}
