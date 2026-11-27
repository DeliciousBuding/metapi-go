package store

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// NormalizeTags preserves first-use order and case for site and account tags.
func NormalizeTags(list []string) []string {
	seen := make(map[string]struct{}, len(list))
	out := make([]string, 0, len(list))
	for _, t := range list {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// EncodeTags marshals a tag list to the stored JSON array text form.
// An empty list is persisted as an empty string.
func EncodeTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return ""
	}
	return string(b)
}

type TagUsage struct {
	Name     string `json:"name"`
	Accounts int    `json:"accounts"`
	Sites    int    `json:"sites"`
	Total    int    `json:"total"`
}

// TagUsage counts each tag once per row. Legacy malformed arrays are ignored.
func (db *DB) TagUsage() ([]TagUsage, error) {
	var rows []struct {
		Tags string
		Site bool
	}
	err := db.Select(&rows, `SELECT tags, FALSE AS site FROM accounts WHERE COALESCE(tags, '') <> ''
		UNION ALL SELECT tags, TRUE AS site FROM sites WHERE COALESCE(tags, '') <> ''`)
	if err != nil {
		return nil, err
	}
	counts := map[string]TagUsage{}
	for _, row := range rows {
		var tags []string
		if err := json.Unmarshal([]byte(row.Tags), &tags); err != nil {
			continue
		}
		for _, name := range NormalizeTags(tags) {
			c := counts[name]
			c.Name = name
			c.Total++
			if row.Site {
				c.Sites++
			} else {
				c.Accounts++
			}
			counts[name] = c
		}
	}
	items := make([]TagUsage, 0, len(counts))
	for _, c := range counts {
		items = append(items, c)
	}
	slices.SortFunc(items, func(a, b TagUsage) int {
		return cmp.Or(cmp.Compare(b.Total, a.Total), strings.Compare(a.Name, b.Name))
	})
	return items, nil
}

// SetTags preserves the legacy distinction between a failed lookup (not found)
// and a failed write. The table name can only come from the two tag owners.
func (db *DB) SetTags(owner string, id int64, tags []string) error {
	var table string
	switch owner {
	case "account":
		table = "accounts"
	case "site":
		table = "sites"
	default:
		return fmt.Errorf("unsupported tag owner %q", owner)
	}
	var exists int
	if err := db.Get(&exists, "SELECT COUNT(*) FROM "+table+" WHERE id = ?", id); err != nil || exists == 0 {
		return sql.ErrNoRows
	}
	_, err := db.Exec("UPDATE "+table+" SET tags = ? WHERE id = ?", EncodeTags(tags), id)
	return err
}
