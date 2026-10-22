package store

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// UserRequestFilter searches retained public identifiers, never private
// upstream reasons or correlation metadata. Both rows and totals use it.
type UserRequestFilter struct {
	Search     string
	Status     string
	QuotaState string
}

func (f UserRequestFilter) Validate() error {
	if utf8.RuneCountInString(f.Search) > 200 {
		return errors.New("request search exceeds 200 characters")
	}
	switch f.Status {
	case "", "completed", "upstream_failed", "output_interrupted", "client_cancelled", "rejected_no_route", "rejected_policy", "gateway_failed":
	default:
		return errors.New("invalid request status")
	}
	switch f.QuotaState {
	case "", "pending", "settled", "released", "unknown", "missing":
	default:
		return errors.New("invalid quota state")
	}
	return nil
}

func (f UserRequestFilter) where(userID int64) (string, []any, error) {
	if err := f.Validate(); err != nil {
		return "", nil, err
	}
	conditions, args := []string{"r.owner_user_id=?"}, []any{userID}
	if search := strings.TrimSpace(f.Search); search != "" {
		// A single-character escape works identically in both SQL dialects.
		like := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(search)) + "%"
		conditions = append(conditions, `(LOWER(r.id) LIKE ? ESCAPE '!' OR LOWER(COALESCE(r.model_requested,'')) LIKE ? ESCAPE '!')`)
		args = append(args, like, like)
	}
	if f.Status != "" {
		conditions = append(conditions, "r.terminal_status=?")
		args = append(args, f.Status)
	}
	if f.QuotaState == "missing" {
		conditions = append(conditions, "q.request_id IS NULL")
	} else if f.QuotaState != "" {
		conditions = append(conditions, "q.state=?")
		args = append(args, f.QuotaState)
	}
	return " WHERE " + strings.Join(conditions, " AND "), args, nil
}
