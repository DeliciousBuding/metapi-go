package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	_ "time/tzdata"
)

// DownstreamAccessPolicy adds conjunctive restrictions to the existing key
// policy. A present empty channel list denies every channel; omission imposes
// no channel restriction. Model IDs are exact and checked after mapping.
type DownstreamAccessPolicy struct {
	AllowedUpstreamChannelIDs *[]int64                 `json:"allowedUpstreamChannelIds,omitempty"`
	ModelIDs                  []string                 `json:"modelIds,omitempty"`
	ModelMappings             []DownstreamModelMapping `json:"modelMappings,omitempty"`
	Quota                     *DownstreamQuota         `json:"quota,omitempty"`
	BlockReason               string                   `json:"blockReason,omitempty"`
}

type DownstreamModelMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type DownstreamQuota struct {
	Requests    *int64                `json:"requests,omitempty"`
	TotalTokens *int64                `json:"totalTokens,omitempty"`
	Cost        *float64              `json:"cost,omitempty"`
	Period      DownstreamQuotaPeriod `json:"period"`
	Timezone    string                `json:"timezone,omitempty"`
	// HistoryMissingBefore bounds an imported history gap. Rolling/calendar
	// windows recover once they no longer overlap the missing history.
	HistoryMissingBefore *int64 `json:"historyMissingBefore,omitempty"`
}

type DownstreamQuotaPeriod struct {
	Type             string                   `json:"type"`
	PastDuration     *DownstreamQuotaDuration `json:"pastDuration,omitempty"`
	CalendarDuration *DownstreamQuotaDuration `json:"calendarDuration,omitempty"`
}

type DownstreamQuotaDuration struct {
	Value int64  `json:"value,omitempty"`
	Unit  string `json:"unit"`
}

func ParseDownstreamAccessPolicy(raw string) (*DownstreamAccessPolicy, error) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" {
		return nil, nil
	}
	var policy DownstreamAccessPolicy
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&policy); err != nil {
		return nil, errors.New("invalid accessPolicy")
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return nil, errors.New("invalid accessPolicy trailing data")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &policy, nil
}

func (p *DownstreamAccessPolicy) Validate() error {
	if p == nil {
		return nil
	}
	if p.AllowedUpstreamChannelIDs != nil {
		for _, id := range *p.AllowedUpstreamChannelIDs {
			if id <= 0 {
				return errors.New("accessPolicy channel IDs must be positive")
			}
		}
	}
	for _, id := range p.ModelIDs {
		if strings.TrimSpace(id) == "" {
			return errors.New("accessPolicy modelIds must not be empty")
		}
	}
	for _, mapping := range p.ModelMappings {
		if mapping.From == "" || mapping.To == "" {
			return errors.New("accessPolicy model mapping must have from and to")
		}
		if _, err := compileDownstreamModelPattern(mapping.From); err != nil {
			return errors.New("accessPolicy has an unsupported model pattern")
		}
	}
	if q := p.Quota; q != nil {
		if (q.Requests != nil && *q.Requests < 0) || (q.TotalTokens != nil && *q.TotalTokens < 0) || (q.Cost != nil && (*q.Cost < 0 || math.IsNaN(*q.Cost) || math.IsInf(*q.Cost, 0))) {
			return errors.New("accessPolicy quota limits must be nonnegative")
		}
		if q.Requests == nil && q.TotalTokens == nil && q.Cost == nil {
			return errors.New("accessPolicy quota requires a limit")
		}
		if _, _, err := q.Window(time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func compileDownstreamModelPattern(pattern string) (*regexp.Regexp, error) {
	if pattern == "*" {
		return regexp.Compile("(?s)^.*$")
	}
	if !strings.ContainsAny(pattern, "*?+[]{}()^$.|\\") {
		return regexp.Compile("^" + regexp.QuoteMeta(pattern) + "$")
	}
	modifier := ""
	if strings.HasPrefix(pattern, "(?") {
		if end := strings.Index(pattern, ")"); end > 2 && !strings.ContainsAny(pattern[2:end], ":=!<") {
			modifier, pattern = pattern[:end+1], pattern[end+1:]
		}
	}
	pattern = strings.TrimSuffix(strings.TrimPrefix(pattern, "^"), "$")
	return regexp.Compile(modifier + "^(?:" + pattern + ")$")
}

func (p *DownstreamAccessPolicy) MapModel(model string) string {
	if p == nil {
		return model
	}
	for _, m := range p.ModelMappings {
		if re, err := compileDownstreamModelPattern(m.From); err == nil && re.MatchString(model) {
			return m.To
		}
	}
	return model
}

func (p *DownstreamAccessPolicy) AllowsModel(model string) bool {
	return p == nil || (p.BlockReason == "" && (len(p.ModelIDs) == 0 || slices.Contains(p.ModelIDs, p.MapModel(model))))
}

func (p *DownstreamAccessPolicy) AllowsUpstreamChannel(id int64) bool {
	return p == nil || (p.BlockReason == "" && (p.AllowedUpstreamChannelIDs == nil || slices.Contains(*p.AllowedUpstreamChannelIDs, id)))
}

// Window returns inclusive start and exclusive end in Unix milliseconds.
func (q *DownstreamQuota) Window(now time.Time) (int64, int64, error) {
	loc := time.UTC
	if q.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(q.Timezone)
		if err != nil {
			return 0, 0, errors.New("accessPolicy quota has an invalid timezone")
		}
	}
	end := now.UnixMilli() + 1
	switch q.Period.Type {
	case "all_time":
		return 0, end, nil
	case "past_duration":
		d := q.Period.PastDuration
		if d == nil || d.Value <= 0 {
			break
		}
		var unit int64
		switch d.Unit {
		case "minute":
			unit = 60000
		case "hour":
			unit = 3600000
		case "day":
			unit = 86400000
		default:
			return 0, 0, errors.New("accessPolicy quota has an invalid duration unit")
		}
		if d.Value > math.MaxInt64/unit {
			break
		}
		return now.UnixMilli() - d.Value*unit, end, nil
	case "calendar_duration":
		d := q.Period.CalendarDuration
		if d == nil {
			break
		}
		n := now.In(loc)
		var start, stop time.Time
		switch d.Unit {
		case "day":
			start = time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
			stop = start.AddDate(0, 0, 1)
		case "month":
			start = time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
			stop = start.AddDate(0, 1, 0)
		default:
			return 0, 0, errors.New("accessPolicy quota has an invalid calendar unit")
		}
		return start.UnixMilli(), stop.UnixMilli(), nil
	}
	return 0, 0, errors.New("accessPolicy quota has an invalid period")
}

func buildDownstreamQuotaUsageDDL(dialect string) string {
	pk := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if dialect == DialectPostgres {
		pk = "BIGSERIAL PRIMARY KEY"
	}
	return `CREATE TABLE IF NOT EXISTS downstream_quota_usage (
        id ` + pk + `,
		key_id BIGINT NOT NULL REFERENCES downstream_api_keys(id) ON DELETE CASCADE,
		event_key TEXT NOT NULL,
		occurred_at BIGINT NOT NULL,
		requests BIGINT NOT NULL DEFAULT 1,
		total_tokens BIGINT,
		cost DOUBLE PRECISION,
		UNIQUE (key_id,event_key)
	)`
}
