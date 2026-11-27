package schedulespec

import (
	"fmt"
	"strings"
)

// WindowBounds parses an inclusive daily window into minutes since midnight.
func WindowBounds(start, end string) (int, int, error) {
	startMin, err := parseHHMM(start)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid window start %q: %w", start, err)
	}
	endMin, err := parseHHMM(end)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid window end %q: %w", end, err)
	}
	if startMin > endMin {
		return 0, 0, fmt.Errorf("window start %s is after end %s", start, end)
	}

	return startMin, endMin, nil
}

// parseHHMM parses "HH:mm" (24h) into minutes since midnight. Zero-padded
// hours/minutes are expected but single digits are tolerated.
func parseHHMM(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("expected HH:mm, got %q", raw)
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, fmt.Errorf("bad hour %q", parts[0])
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, fmt.Errorf("bad minute %q", parts[1])
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("out of range %q", raw)
	}
	return h*60 + m, nil
}
