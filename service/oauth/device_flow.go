package oauth

import (
	"errors"
	"sync"
	"time"
)

// DeviceFlowInstructions is the safe projection shown to the operator. It
// deliberately excludes device_code, which is only used by the server-side
// token poll.
type DeviceFlowInstructions struct {
	UserCode            string `json:"userCode"`
	VerificationURI     string `json:"verificationUri"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	ExpiresInSeconds    int    `json:"expiresInSeconds"`
}

// DeviceFlowPendingError marks RFC 8628 responses that must leave the OAuth
// session pending: authorization_pending and slow_down.
type DeviceFlowPendingError struct {
	Reason     string
	RetryAfter time.Duration
}

func (e *DeviceFlowPendingError) Error() string {
	if e == nil {
		return "device flow pending"
	}
	return e.Reason
}

func isDeviceFlowPending(err error) bool {
	var pending *DeviceFlowPendingError
	return errors.As(err, &pending)
}

type deviceFlowEntry struct {
	deviceCode      string
	userCode        string
	verificationURI string
	interval        time.Duration
	expiresAt       time.Time
}

var (
	deviceFlowsMu sync.Mutex
	deviceFlows   = make(map[string]deviceFlowEntry)
)

func storeDeviceFlow(state, deviceCode, userCode, verificationURI string, interval, ttl time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	deviceFlowsMu.Lock()
	defer deviceFlowsMu.Unlock()
	pruneDeviceFlowsLocked(time.Now())
	deviceFlows[state] = deviceFlowEntry{
		deviceCode:      deviceCode,
		userCode:        userCode,
		verificationURI: verificationURI,
		interval:        interval,
		expiresAt:       time.Now().Add(ttl),
	}
}

func lookupDeviceFlow(state string) (deviceFlowEntry, bool) {
	deviceFlowsMu.Lock()
	defer deviceFlowsMu.Unlock()
	pruneDeviceFlowsLocked(time.Now())
	entry, ok := deviceFlows[state]
	return entry, ok
}

func clearDeviceFlow(state string) {
	deviceFlowsMu.Lock()
	defer deviceFlowsMu.Unlock()
	delete(deviceFlows, state)
}

// slowDownDeviceFlow applies RFC 8628's +5s requirement and returns the next
// interval. Unknown states keep the default pacing.
func slowDownDeviceFlow(state string) time.Duration {
	deviceFlowsMu.Lock()
	defer deviceFlowsMu.Unlock()
	entry, ok := deviceFlows[state]
	if !ok {
		return 5 * time.Second
	}
	entry.interval += 5 * time.Second
	deviceFlows[state] = entry
	return entry.interval
}

func pruneDeviceFlowsLocked(now time.Time) {
	for state, entry := range deviceFlows {
		if !entry.expiresAt.After(now) {
			delete(deviceFlows, state)
		}
	}
}

func deviceFlowInstructions(state string) *DeviceFlowInstructions {
	entry, ok := lookupDeviceFlow(state)
	if !ok {
		return nil
	}
	return &DeviceFlowInstructions{
		UserCode:            entry.userCode,
		VerificationURI:     entry.verificationURI,
		PollIntervalSeconds: maxInt(int(entry.interval/time.Second), 1),
		ExpiresInSeconds:    maxInt(int(time.Until(entry.expiresAt).Round(time.Second)/time.Second), 1),
	}
}

func maxInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}
