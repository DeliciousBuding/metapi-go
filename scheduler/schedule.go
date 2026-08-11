package scheduler

import (
	"fmt"
	"strconv"
	"strings"
)

// ScheduleSpecVersion is the current version of the semantic schedule spec.
// Version is embedded in persisted JSON so future format changes can be
// detected instead of silently misinterpreting old documents.
const ScheduleSpecVersion = 1

// ScheduleSpec is a versioned, JSON-serializable semantic description of a
// cron schedule. The original cron expression is preserved verbatim in Cron
// (including 5/6-field form and original whitespace) so that reverse mapping
// is lossless whenever the semantic fields are unchanged. Expressions that
// cannot be mapped without losing information become Type "custom" and keep
// the raw expression as the single source of truth.
type ScheduleSpec struct {
	Version     int    `json:"version"`
	Type        string `json:"type"` // daily | interval | window | custom
	Time        string `json:"time,omitempty"`
	EveryHours  int    `json:"everyHours,omitempty"`
	WindowStart string `json:"windowStart,omitempty"`
	WindowEnd   string `json:"windowEnd,omitempty"`
	Cron        string `json:"cron,omitempty"`
}

// CronToSchedule maps a cron expression to a semantic ScheduleSpec without
// modifying the expression. The original bytes are preserved in spec.Cron.
// Unparseable or unmappable expressions become Type "custom".
func CronToSchedule(expr string) ScheduleSpec {
	trimmed := strings.TrimSpace(expr)
	spec := ScheduleSpec{Version: ScheduleSpecVersion, Cron: trimmed}
	if trimmed == "" || !ValidateCronExpr(trimmed) {
		spec.Type = "custom"
		return spec
	}

	fields := strings.Fields(normalizeCronExpr(trimmed))
	if len(fields) != 6 {
		spec.Type = "custom"
		return spec
	}
	sec, min, hour, dom, month, dow := fields[0], fields[1], fields[2], fields[3], fields[4], fields[5]

	// Daily at a fixed HH:mm: second=0, dom/month/dow=*, minute/hour static.
	if sec == "0" && dom == "*" && month == "*" && dow == "*" &&
		isStaticField(min) && isStaticField(hour) {
		if h, err := strconv.Atoi(hour); err == nil {
			if m, err := strconv.Atoi(min); err == nil && h >= 0 && h <= 23 && m >= 0 && m <= 59 {
				spec.Type = "daily"
				spec.Time = fmt.Sprintf("%02d:%02d", h, m)
				return spec
			}
		}
	}

	// Interval every N hours at minute 0: second=0, minute=0, hour=*/N or *,
	// dom/month/dow=*.
	if sec == "0" && min == "0" && dom == "*" && month == "*" && dow == "*" && isStepField(hour) {
		if step, ok := parseStepField(hour); ok && step >= 1 && step <= 24 {
			spec.Type = "interval"
			spec.EveryHours = step
			return spec
		}
	}

	spec.Type = "custom"
	return spec
}

// ScheduleToCron converts a semantic ScheduleSpec back to a cron expression.
// When the spec's semantic fields still match the original expression it
// returns the original bytes unchanged; otherwise it generates a normalized
// expression. Type "window" has no deterministic cron (the runtime re-rolls a
// random time inside the window) and returns the preserved expression only.
func ScheduleToCron(spec ScheduleSpec) (string, error) {
	switch spec.Type {
	case "daily":
		if !validHHMM(spec.Time) {
			return "", fmt.Errorf("daily schedule requires a valid HH:mm time, got %q", spec.Time)
		}
		if spec.Cron != "" {
			if derived := CronToSchedule(spec.Cron); derived.Type == "daily" && derived.Time == spec.Time {
				return spec.Cron, nil
			}
		}
		h, m, err := splitHHMM(spec.Time)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d %d * * *", m, h), nil
	case "interval":
		if spec.EveryHours < 1 || spec.EveryHours > 24 {
			return "", fmt.Errorf("interval schedule requires everyHours in [1, 24], got %d", spec.EveryHours)
		}
		if spec.Cron != "" {
			if derived := CronToSchedule(spec.Cron); derived.Type == "interval" && derived.EveryHours == spec.EveryHours {
				return spec.Cron, nil
			}
		}
		return fmt.Sprintf("0 0 */%d * * *", spec.EveryHours), nil
	case "window":
		if spec.Cron != "" && ValidateCronExpr(spec.Cron) {
			return spec.Cron, nil
		}
		return "", nil
	case "custom", "":
		if spec.Cron != "" && ValidateCronExpr(spec.Cron) {
			return spec.Cron, nil
		}
		return "", fmt.Errorf("custom schedule requires a valid cron expression")
	default:
		return "", fmt.Errorf("unknown schedule type %q", spec.Type)
	}
}

// Validate checks that a ScheduleSpec is well-formed and self-consistent.
// Version must match ScheduleSpecVersion so stale documents cannot be applied
// silently.
func (s ScheduleSpec) Validate() error {
	if s.Version != ScheduleSpecVersion {
		return fmt.Errorf("unsupported schedule spec version %d (expected %d)", s.Version, ScheduleSpecVersion)
	}
	switch s.Type {
	case "daily":
		if !validHHMM(s.Time) {
			return fmt.Errorf("daily schedule requires a valid HH:mm time, got %q", s.Time)
		}
	case "interval":
		if s.EveryHours < 1 || s.EveryHours > 24 {
			return fmt.Errorf("interval schedule requires everyHours in [1, 24], got %d", s.EveryHours)
		}
	case "window":
		if _, err := RandomCronInWindow(s.WindowStart, s.WindowEnd); err != nil {
			return err
		}
	case "custom", "":
		if !ValidateCronExpr(s.Cron) {
			return fmt.Errorf("custom schedule requires a valid cron expression")
		}
	default:
		return fmt.Errorf("unknown schedule type %q", s.Type)
	}
	return nil
}

// validHHMM reports whether raw is a valid "HH:mm" (24h) time.
func validHHMM(raw string) bool {
	_, err := parseHHMM(raw)
	return err == nil
}

// splitHHMM parses "HH:mm" into hour and minute.
func splitHHMM(raw string) (int, int, error) {
	total, err := parseHHMM(raw)
	if err != nil {
		return 0, 0, err
	}
	return total / 60, total % 60, nil
}

// isStaticField reports whether a cron field is a single literal value
// (no wildcard, step, range, or list).
func isStaticField(field string) bool {
	if field == "" || strings.ContainsAny(field, "*,/-") {
		return false
	}
	return true
}

// isStepField reports whether a cron field is "*" or "*/N".
func isStepField(field string) bool {
	if field == "*" {
		return true
	}
	return strings.HasPrefix(field, "*/")
}

// parseStepField returns the step count for "*" (1) or "*/N" (N).
func parseStepField(field string) (int, bool) {
	if field == "*" {
		return 1, true
	}
	if strings.HasPrefix(field, "*/") {
		n, err := strconv.Atoi(strings.TrimPrefix(field, "*/"))
		if err != nil || n < 1 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
