package platform

import (
	"fmt"
	"sort"
	"strings"
)

// modelFetchLadder owns one behaviour shared by every model-discovery ladder in
// this package: telling "the upstream answered and this account has no models"
// apart from "no rung of the ladder reached the upstream". Callers used to end
// their ladders with `[]string{}, nil` for both, and service/account_model_refresh
// only classifies a failure when err != nil, so every cause collapsed into the
// single code `empty_models`.
//
// The rungs are a preference order (the credential the operator configured first,
// dashboard/session fallbacks after), so the FIRST reason is kept: it is the one
// that explains the symptom. listAPIKeys keeps the last error instead, because its
// two endpoints are equivalent version-skew alternatives rather than a preference
// order — that difference is deliberate, do not "unify" the two.
//
// A nil ladder is valid and records nothing; VerifyToken passes nil because a
// verify rung failing is a signal to try the next credential shape, not a failure
// to report.
type modelFetchLadder struct {
	answered bool
	reason   string
}

// answer records that a rung read a real response, so an empty model list from it
// is a true statement about the account.
func (l *modelFetchLadder) answer() {
	if l != nil {
		l.answered = true
	}
}

// fail records why a rung produced nothing. Only the first reason is kept.
func (l *modelFetchLadder) fail(reason string) {
	if l == nil || l.reason != "" || strings.TrimSpace(reason) == "" {
		return
	}
	l.reason = strings.TrimSpace(reason)
}

// result is the end of the ladder: an answered empty list, or the reason no rung
// could answer.
func (l *modelFetchLadder) result() ([]string, error) {
	if l != nil && l.answered {
		return []string{}, nil
	}
	reason := "no model endpoint answered"
	if l != nil && l.reason != "" {
		reason = l.reason
	}
	return nil, fmt.Errorf("fetch models: %s", reason)
}

// extractModelIDsFromData extracts "data": [{"id": ...}] model ids from an
// OpenAI-compatible /v1/models response.
func extractModelIDsFromData(resp map[string]interface{}) []string {
	data, ok := resp["data"].([]interface{})
	if !ok {
		return nil
	}
	models := make([]string, 0, len(data))
	for _, item := range data {
		if m, ok := item.(map[string]interface{}); ok {
			if id, ok := m["id"].(string); ok && strings.TrimSpace(id) != "" {
				models = append(models, strings.TrimSpace(id))
			}
		}
	}
	return normalizeModelIDs(models)
}

// normalizeModelIDs dedups case-insensitively (preserving first-seen casing),
// strips a "models/" prefix, and sorts for deterministic output.
func normalizeModelIDs(models []string) []string {
	seen := make(map[string]bool, len(models))
	result := make([]string, 0, len(models))
	for _, m := range models {
		t := strings.TrimPrefix(strings.TrimSpace(m), "models/")
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, t)
	}
	sort.Strings(result)
	return result
}
