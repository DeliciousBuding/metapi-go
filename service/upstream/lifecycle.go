// Package upstream owns transactional changes to the direct-upstream graph.
package upstream

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jmoiron/sqlx"
)

type Kind string

const (
	KindChannel    Kind = "channel"
	KindModel      Kind = "model"
	KindCredential Kind = "credential"
	KindGrant      Kind = "grant"
	KindGroup      Kind = "group"
	KindMember     Kind = "member"
	KindRoute      Kind = "route"
)

var (
	ErrNotFound             = errors.New("upstream entity not found")
	ErrConfirmationRequired = errors.New("related entities require cascade confirmation and expectedRevision")
	ErrRevisionMismatch     = errors.New("deletion impact changed; review a fresh preview")
)

type Reference struct {
	Kind Kind  `json:"kind"`
	ID   int64 `json:"id"`
}

type DeletionPreview struct {
	Kind             Kind           `json:"kind"`
	ID               int64          `json:"id"`
	Counts           map[string]int `json:"counts"`
	AffectedRouteIDs []int64        `json:"affectedRouteIds"`
	Revision         string         `json:"revision"`
	RequiresCascade  bool           `json:"requiresCascade"`
}

type DeleteOptions struct {
	Cascade          bool
	ExpectedRevision string
}

type DeletionResult struct {
	DeletionPreview
	OnlyDeletedRouteKeys []int64 `json:"downstreamKeysWithOnlyDeletedRoutes,omitempty"`
}

type entitySpec struct {
	kind         Kind
	table, label string
	aliases      []string
}

var deletionEntities = []entitySpec{
	{KindMember, "upstream_group_items", "members", []string{"group_items", "upstream_group_items"}},
	{KindGroup, "upstream_groups", "groups", []string{"groups", "upstream_groups"}},
	{KindGrant, "upstream_grants", "grants", []string{"channel_grants", "upstream_grants"}},
	{KindModel, "upstream_models", "models", []string{"channel_models", "upstream_models"}},
	{KindCredential, "upstream_credentials", "credentials", []string{"channel_keys", "upstream_credentials"}},
	{KindChannel, "upstream_channels", "channels", []string{"channels", "upstream_channels"}},
	{KindRoute, "token_routes", "routes", []string{"token_routes"}},
}

func KindForSourceEntity(entity string) (Kind, bool) {
	for _, spec := range deletionEntities {
		for _, alias := range spec.aliases {
			if entity == alias {
				return spec.kind, true
			}
		}
	}
	return "", false
}

type lifecycleNode struct {
	ID     int64  `db:"id"`
	Origin string `db:"origin_key"`
	Parent int64  `db:"parent"`
	Other  int64  `db:"other"`
}
type lifecyclePair struct {
	RouteID int64 `db:"route_id"`
	GroupID int64 `db:"group_id"`
}
type lifecycleMapping struct {
	Origin   string `db:"origin_key"`
	Entity   string `db:"entity_type"`
	SourceID int64  `db:"source_id"`
	TargetID int64  `db:"target_id"`
}

// DeletionPlan is a snapshot for one locked transaction, never another transaction.
type DeletionPlan struct {
	Preview       DeletionPreview
	ids           map[Kind]map[int64]bool
	nodes         map[Kind][]lifecycleNode
	mappings      []lifecycleMapping
	routeChannels []int64
	routeSources  []int64
	keys          []routeKeyChange
	activeGroups  []lifecycleNode
}

func (p *DeletionPlan) IncludesOtherOrigins(origin string) bool {
	for kind, nodes := range p.nodes {
		for _, node := range nodes {
			if p.ids[kind][node.ID] && node.Origin != "" && node.Origin != origin {
				return true
			}
		}
	}
	return false
}

func PreviewDeletion(ctx context.Context, db *sqlx.DB, kind Kind, id int64) (*DeletionPreview, error) {
	tx, err := db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	plan, err := PlanDeletionTx(ctx, tx, []Reference{{kind, id}})
	if err != nil {
		return nil, err
	}
	return &plan.Preview, nil
}

func Delete(ctx context.Context, db *sqlx.DB, kind Kind, id int64, options DeleteOptions) (*DeletionResult, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = LockLifecycleTx(ctx, tx); err != nil {
		return nil, err
	}
	plan, err := PlanDeletionTx(ctx, tx, []Reference{{kind, id}})
	if err != nil {
		return nil, err
	}
	result := &DeletionResult{DeletionPreview: plan.Preview}
	if plan.Preview.RequiresCascade && (!options.Cascade || options.ExpectedRevision == "") {
		return result, ErrConfirmationRequired
	}
	if options.ExpectedRevision != "" && options.ExpectedRevision != plan.Preview.Revision {
		return result, ErrRevisionMismatch
	}
	result.OnlyDeletedRouteKeys, err = ApplyDeletionTx(ctx, tx, plan)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// PlanDeletionTx follows relationships regardless of origin. An imported parent
// may have native children, and a grant may serve members in another origin.
func PlanDeletionTx(ctx context.Context, tx *sqlx.Tx, roots []Reference) (*DeletionPlan, error) {
	p := &DeletionPlan{ids: map[Kind]map[int64]bool{}, nodes: map[Kind][]lifecycleNode{}, Preview: DeletionPreview{Kind: "source", Counts: map[string]int{}, AffectedRouteIDs: []int64{}}}
	queries := map[Kind]string{
		KindChannel:    `SELECT id,origin_key,0 AS parent,0 AS other FROM upstream_channels`,
		KindModel:      `SELECT id,origin_key,channel_id AS parent,0 AS other FROM upstream_models`,
		KindCredential: `SELECT id,origin_key,channel_id AS parent,0 AS other FROM upstream_credentials`,
		KindGrant:      `SELECT id,origin_key,model_id AS parent,credential_id AS other FROM upstream_grants`,
		KindGroup:      `SELECT id,origin_key,active_item_id AS parent,0 AS other FROM upstream_groups`,
		KindMember:     `SELECT id,origin_key,group_id AS parent,grant_id AS other FROM upstream_group_items`,
		KindRoute:      `SELECT id,'' AS origin_key,0 AS parent,0 AS other FROM token_routes`,
	}
	all := map[Kind]map[int64]bool{}
	for _, spec := range deletionEntities {
		var rows []lifecycleNode
		if err := tx.SelectContext(ctx, &rows, queries[spec.kind]+" ORDER BY id"); err != nil {
			return nil, err
		}
		p.nodes[spec.kind] = rows
		p.ids[spec.kind] = map[int64]bool{}
		all[spec.kind] = map[int64]bool{}
		for _, row := range rows {
			all[spec.kind][row.ID] = true
		}
	}
	for _, root := range roots {
		if root.ID <= 0 || !all[root.Kind][root.ID] {
			return nil, ErrNotFound
		}
		p.ids[root.Kind][root.ID] = true
	}
	if len(roots) == 1 {
		p.Preview.Kind = roots[0].Kind
		p.Preview.ID = roots[0].ID
	}
	var pairs []lifecyclePair
	if err := tx.SelectContext(ctx, &pairs, `SELECT route_id,group_id FROM upstream_route_groups ORDER BY route_id`); err != nil {
		return nil, err
	}
	for _, pair := range pairs {
		if p.ids[KindRoute][pair.RouteID] || p.ids[KindGroup][pair.GroupID] {
			p.ids[KindRoute][pair.RouteID] = true
			p.ids[KindGroup][pair.GroupID] = true
		}
	}
	for _, kind := range []Kind{KindModel, KindCredential} {
		for _, n := range p.nodes[kind] {
			if p.ids[KindChannel][n.Parent] {
				p.ids[kind][n.ID] = true
			}
		}
	}
	for _, n := range p.nodes[KindGrant] {
		if p.ids[KindModel][n.Parent] || p.ids[KindCredential][n.Other] {
			p.ids[KindGrant][n.ID] = true
		}
	}
	affectedGroups := map[int64]bool{}
	for _, n := range p.nodes[KindMember] {
		if p.ids[KindGroup][n.Parent] || p.ids[KindGrant][n.Other] {
			p.ids[KindMember][n.ID] = true
		}
		if p.ids[KindMember][n.ID] {
			affectedGroups[n.Parent] = true
		}
	}
	affectedRoutes := map[int64]bool{}
	for id := range p.ids[KindRoute] {
		affectedRoutes[id] = true
	}
	for _, pair := range pairs {
		if affectedGroups[pair.GroupID] {
			affectedRoutes[pair.RouteID] = true
		}
	}
	for _, g := range p.nodes[KindGroup] {
		if p.ids[KindMember][g.Parent] && !p.ids[KindGroup][g.ID] {
			p.activeGroups = append(p.activeGroups, g)
		}
	}
	var routeChannels []struct {
		ID      int64 `db:"id"`
		RouteID int64 `db:"route_id"`
	}
	if err := tx.SelectContext(ctx, &routeChannels, `SELECT id,route_id FROM route_channels ORDER BY id`); err != nil {
		return nil, err
	}
	for _, row := range routeChannels {
		if p.ids[KindRoute][row.RouteID] {
			p.routeChannels = append(p.routeChannels, row.ID)
		}
	}
	var sources []struct {
		ID       int64 `db:"id"`
		GroupID  int64 `db:"group_route_id"`
		SourceID int64 `db:"source_route_id"`
	}
	if err := tx.SelectContext(ctx, &sources, `SELECT id,group_route_id,source_route_id FROM route_group_sources ORDER BY id`); err != nil {
		return nil, err
	}
	for _, row := range sources {
		if p.ids[KindRoute][row.GroupID] || p.ids[KindRoute][row.SourceID] {
			p.routeSources = append(p.routeSources, row.ID)
			affectedRoutes[row.GroupID] = true
		}
	}
	var mappings []lifecycleMapping
	if err := tx.SelectContext(ctx, &mappings, `SELECT origin_key,entity_type,source_id,target_id FROM external_source_ids ORDER BY origin_key,entity_type,source_id,target_id`); err != nil {
		return nil, err
	}
	for _, m := range mappings {
		if kind, ok := KindForSourceEntity(m.Entity); ok && p.ids[kind][m.TargetID] {
			p.mappings = append(p.mappings, m)
		}
	}
	var err error
	p.keys, err = loadRouteKeyChanges(ctx, tx, p.ids[KindRoute])
	if err != nil {
		return nil, err
	}
	deleted := map[Kind][]lifecycleNode{}
	total := 0
	for _, spec := range deletionEntities {
		p.Preview.Counts[spec.label] = len(p.ids[spec.kind])
		total += len(p.ids[spec.kind])
		for _, n := range p.nodes[spec.kind] {
			if p.ids[spec.kind][n.ID] {
				deleted[spec.kind] = append(deleted[spec.kind], n)
			}
		}
	}
	p.Preview.Counts["routeChannels"] = len(p.routeChannels)
	p.Preview.Counts["routeGroupSources"] = len(p.routeSources)
	p.Preview.Counts["downstreamKeys"] = len(p.keys)
	p.Preview.Counts["sourceMappings"] = len(p.mappings)
	p.Preview.AffectedRouteIDs = sortedIDs(affectedRoutes)
	p.Preview.RequiresCascade = total > len(roots) || len(p.routeChannels)+len(p.routeSources)+len(p.keys) > 0
	// Encode only identities and relationship/policy inputs, never credentials.
	encoded, err := json.Marshal(struct {
		Nodes                     map[Kind][]lifecycleNode
		Mappings                  []lifecycleMapping
		Routes, Channels, Sources []int64
		Keys                      []routeKeyChange
		Active                    []lifecycleNode
	}{deleted, p.mappings, p.Preview.AffectedRouteIDs, p.routeChannels, p.routeSources, p.keys, p.activeGroups})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	p.Preview.Revision = hex.EncodeToString(sum[:])
	return p, nil
}

func sortedIDs(ids map[int64]bool) []int64 {
	out := make([]int64, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ValidateSourceReferencesTx checks root ownership before replacement. Descendant
// origins are intentionally unrestricted, but must be present in its preview.
func ValidateSourceReferencesTx(ctx context.Context, tx *sqlx.Tx, origin string, roots []Reference) error {
	for _, root := range roots {
		var count int
		query := ""
		if root.Kind == KindRoute {
			query = `SELECT COUNT(*) FROM upstream_route_groups rg JOIN upstream_groups g ON g.id=rg.group_id WHERE rg.route_id=? AND g.origin_key=?`
		} else {
			for _, spec := range deletionEntities {
				if spec.kind == root.Kind {
					query = "SELECT COUNT(*) FROM " + spec.table + " WHERE id=? AND origin_key=?"
					break
				}
			}
		}
		if query == "" {
			return ErrNotFound
		}
		if err := tx.GetContext(ctx, &count, tx.Rebind(query), root.ID, origin); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("imported entity ownership changed; source replacement requires review")
		}
	}
	return nil
}

func deleteIDs(ctx context.Context, tx *sqlx.Tx, table string, ids []int64) error {
	for start := 0; start < len(ids); start += 400 {
		end := start + 400
		if end > len(ids) {
			end = len(ids)
		}
		query, args, err := sqlx.In("DELETE FROM "+table+" WHERE id IN (?)", ids[start:end])
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
			return err
		}
	}
	return nil
}

// ApplyDeletionTx is the sole graph deletion owner used by API deletion and
// source replacement. The caller owns locking, confirmation and commit.
func ApplyDeletionTx(ctx context.Context, tx *sqlx.Tx, p *DeletionPlan) ([]int64, error) {
	for _, g := range p.activeGroups {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE upstream_groups SET active_item_id=0 WHERE id=? AND active_item_id=?`), g.ID, g.Parent); err != nil {
			return nil, err
		}
	}
	if err := deleteIDs(ctx, tx, "route_group_sources", p.routeSources); err != nil {
		return nil, err
	}
	if err := deleteIDs(ctx, tx, "route_channels", p.routeChannels); err != nil {
		return nil, err
	}
	onlyDeleted, err := applyRouteKeyChanges(ctx, tx, p.keys)
	if err != nil {
		return nil, err
	}
	for _, spec := range deletionEntities {
		if err := deleteIDs(ctx, tx, spec.table, sortedIDs(p.ids[spec.kind])); err != nil {
			return nil, err
		}
	}
	for _, mapping := range p.mappings {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM external_source_ids WHERE origin_key=? AND entity_type=? AND source_id=? AND target_id=?`), mapping.Origin, mapping.Entity, mapping.SourceID, mapping.TargetID); err != nil {
			return nil, err
		}
	}
	return onlyDeleted, nil
}
