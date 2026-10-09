package upstream

import (
	"context"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

type MemberCreate struct {
	GrantID       int64                     `json:"grantId"`
	Priority      int64                     `json:"priority"`
	Weight        *int64                    `json:"weight"`
	ProtocolOrder store.DirectProtocolOrder `json:"protocolOrder"`
}
type Member struct {
	ID             int64                     `db:"id" json:"id"`
	GroupID        int64                     `db:"group_id" json:"groupId"`
	GrantID        int64                     `db:"grant_id" json:"grantId"`
	Priority       int64                     `db:"priority" json:"priority"`
	Weight         int64                     `db:"weight" json:"weight"`
	ProtocolOrder  store.DirectProtocolOrder `db:"protocol_order" json:"protocolOrder"`
	Origin         string                    `db:"origin_key" json:"-"`
	Ownership      string                    `json:"ownership"`
	ModelName      string                    `db:"model_name" json:"modelName"`
	CredentialName string                    `db:"credential_name" json:"credentialName"`
	ChannelID      int64                     `db:"channel_id" json:"channelId"`
}
type Group struct {
	ID             int64    `db:"id" json:"id"`
	Name           string   `db:"name" json:"name"`
	Mode           string   `db:"mode" json:"mode"`
	Enabled        bool     `db:"enabled" json:"enabled"`
	ActiveMemberID int64    `db:"active_item_id" json:"activeMemberId"`
	Origin         string   `db:"origin_key" json:"-"`
	Ownership      string   `json:"ownership"`
	RouteID        int64    `db:"route_id" json:"routeId"`
	ModelPattern   string   `db:"model_pattern" json:"modelPattern"`
	DisplayName    string   `db:"display_name" json:"displayName"`
	Members        []Member `json:"members"`
}

const groupSelect = `SELECT g.id,g.name,g.mode,g.enabled,g.active_item_id,g.origin_key,COALESCE(rg.route_id,0) AS route_id,COALESCE(r.model_pattern,'') AS model_pattern,COALESCE(r.display_name,'') AS display_name FROM upstream_groups g LEFT JOIN upstream_route_groups rg ON rg.group_id=g.id LEFT JOIN token_routes r ON r.id=rg.route_id`
const memberSelect = `SELECT i.id,i.group_id,i.grant_id,i.priority,i.weight,i.protocol_order,i.origin_key,m.name AS model_name,k.name AS credential_name,m.channel_id FROM upstream_group_items i JOIN upstream_grants g ON g.id=i.grant_id JOIN upstream_models m ON m.id=g.model_id JOIN upstream_credentials k ON k.id=g.credential_id`

func ListGroups(ctx context.Context, db *sqlx.DB) ([]Group, error) {
	out := []Group{}
	members := []Member{}
	if err := db.SelectContext(ctx, &out, groupSelect+` ORDER BY g.name,g.id`); err != nil {
		return nil, err
	}
	if err := db.SelectContext(ctx, &members, memberSelect+` ORDER BY i.priority,i.id`); err != nil {
		return nil, err
	}
	byGroup := map[int64][]Member{}
	for _, m := range members {
		m.Ownership = Ownership(m.Origin)
		if m.ProtocolOrder == nil {
			m.ProtocolOrder = store.DirectProtocolOrder{}
		}
		byGroup[m.GroupID] = append(byGroup[m.GroupID], m)
	}
	for i := range out {
		out[i].Ownership = Ownership(out[i].Origin)
		out[i].Members = byGroup[out[i].ID]
		if out[i].Members == nil {
			out[i].Members = []Member{}
		}
	}
	return out, nil
}

type GroupCreate struct {
	Name          string `json:"name"`
	Mode          string `json:"mode"`
	Enabled       bool   `json:"enabled"`
	ActiveGrantID int64  `json:"activeGrantId"`
	Route         struct {
		ModelPattern    string `json:"modelPattern"`
		DisplayName     string `json:"displayName"`
		RoutingStrategy string `json:"routingStrategy"`
	} `json:"route"`
	Members []MemberCreate `json:"members"`
}
type GroupUpdate struct {
	Name           *string `json:"name"`
	Mode           *string `json:"mode"`
	Enabled        *bool   `json:"enabled"`
	ActiveMemberID *int64  `json:"activeMemberId"`
}

func CreateGroup(ctx context.Context, db *sqlx.DB, in GroupCreate) (Group, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Route.ModelPattern = strings.TrimSpace(in.Route.ModelPattern)
	in.Route.DisplayName = strings.TrimSpace(in.Route.DisplayName)
	if !validName(in.Name) || !validName(in.Route.ModelPattern) || !routing.IsExactRouteModelPattern(in.Route.ModelPattern) || len(in.Route.DisplayName) > 200 {
		return Group{}, invalid("Group requires a name and an exact public model name")
	}
	if in.Mode == "" {
		in.Mode = "failover"
	}
	if in.Mode != "failover" && in.Mode != "manual" {
		return Group{}, invalid("Unsupported group mode")
	}
	if len(in.Members) > 500 {
		return Group{}, invalid("A group can create at most 500 members at once")
	}
	if in.Route.RoutingStrategy == "" {
		in.Route.RoutingStrategy = "weighted"
	}
	known := false
	for _, s := range routing.KnownRouteRoutingStrategies {
		if string(s) == in.Route.RoutingStrategy {
			known = true
		}
	}
	if !known {
		return Group{}, invalid("Unsupported routing strategy")
	}
	var out Group
	err := Write(ctx, db, func(tx *sqlx.Tx) error {
		// The lifecycle lock protects the exact-name check against graph writers.
		var count int
		query := `SELECT COUNT(*) FROM token_routes WHERE model_pattern=? OR display_name=?`
		args := []any{in.Route.ModelPattern, in.Route.ModelPattern}
		if in.Route.DisplayName != "" {
			query += ` OR model_pattern=? OR display_name=?`
			args = append(args, in.Route.DisplayName, in.Route.DisplayName)
		}
		if err := tx.GetContext(ctx, &count, tx.Rebind(query), args...); err != nil {
			return err
		}
		if count > 0 {
			return conflict("Public model name already belongs to a route")
		}
		id, err := InsertNative(ctx, tx, "groups", in.Name, in.Mode, int64(0), `{}`, in.Enabled)
		if err != nil {
			return err
		}
		out = Group{ID: id, Name: in.Name, Mode: in.Mode, Enabled: in.Enabled, Ownership: "native", ModelPattern: in.Route.ModelPattern, DisplayName: in.Route.DisplayName, Members: []Member{}}
		now := time.Now().UTC().Format(time.RFC3339)
		if err = tx.GetContext(ctx, &out.RouteID, tx.Rebind(`INSERT INTO token_routes (model_pattern,display_name,route_mode,routing_strategy,enabled,created_at,updated_at) VALUES (?,?,'pattern',?,?,?,?) RETURNING id`), in.Route.ModelPattern, in.Route.DisplayName, in.Route.RoutingStrategy, true, now, now); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO upstream_route_groups (route_id,group_id) VALUES (?,?)`), out.RouteID, id); err != nil {
			return err
		}
		for _, input := range in.Members {
			member, err := createMember(ctx, tx, id, input)
			if err != nil {
				return err
			}
			out.Members = append(out.Members, member)
			if input.GrantID == in.ActiveGrantID {
				out.ActiveMemberID = member.ID
			}
		}
		if in.ActiveGrantID != 0 && out.ActiveMemberID == 0 {
			return invalid("Active grant must be included in the group members")
		}
		if out.Mode == "manual" && out.Enabled && out.ActiveMemberID == 0 {
			return invalid("Enabled manual group requires an active member")
		}
		_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE upstream_groups SET active_item_id=? WHERE id=?`), out.ActiveMemberID, id)
		return err
	})
	return out, err
}

func UpdateGroup(ctx context.Context, db *sqlx.DB, id int64, in GroupUpdate) (Group, error) {
	if in.Name == nil && in.Mode == nil && in.Enabled == nil && in.ActiveMemberID == nil {
		return Group{}, invalid("Empty group update")
	}
	if in.Name != nil {
		*in.Name = strings.TrimSpace(*in.Name)
		if !validName(*in.Name) {
			return Group{}, invalid("Invalid group name")
		}
	}
	if in.Mode != nil && *in.Mode != "manual" && *in.Mode != "failover" {
		return Group{}, invalid("Unsupported group mode")
	}
	var out Group
	err := Write(ctx, db, func(tx *sqlx.Tx) error {
		if err := tx.GetContext(ctx, &out, tx.Rebind(lockQuery(tx, `SELECT id,name,mode,enabled,active_item_id,origin_key FROM upstream_groups WHERE id=?`)), id); err != nil {
			return err
		}
		if in.Name != nil {
			out.Name = *in.Name
		}
		if in.Mode != nil {
			out.Mode = *in.Mode
		}
		if in.Enabled != nil {
			out.Enabled = *in.Enabled
		}
		if in.ActiveMemberID != nil {
			out.ActiveMemberID = *in.ActiveMemberID
		}
		if out.ActiveMemberID != 0 {
			var count int
			if err := tx.GetContext(ctx, &count, tx.Rebind(`SELECT COUNT(*) FROM upstream_group_items WHERE id=? AND group_id=?`), out.ActiveMemberID, id); err != nil {
				return err
			}
			if count != 1 {
				return invalid("Active member must belong to this group")
			}
		}
		if out.Mode == "manual" && out.Enabled && out.ActiveMemberID == 0 {
			return invalid("Enabled manual group requires an active member")
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE upstream_groups SET name=?,mode=?,enabled=?,active_item_id=? WHERE id=?`), out.Name, out.Mode, out.Enabled, out.ActiveMemberID, id); err != nil {
			return err
		}
		if err := tx.GetContext(ctx, &out, tx.Rebind(groupSelect+` WHERE g.id=?`), id); err != nil {
			return err
		}
		out.Members = []Member{}
		return tx.SelectContext(ctx, &out.Members, tx.Rebind(memberSelect+` WHERE i.group_id=? ORDER BY i.priority,i.id`), id)
	})
	out.Ownership = Ownership(out.Origin)
	for i := range out.Members {
		out.Members[i].Ownership = Ownership(out.Members[i].Origin)
	}
	return out, err
}

func createMember(ctx context.Context, tx *sqlx.Tx, groupID int64, in MemberCreate) (Member, error) {
	weight := int64(1)
	if in.Weight != nil {
		weight = *in.Weight
	}
	if weight <= 0 || weight > 2147483647 || in.Priority < -2147483648 || in.Priority > 2147483647 {
		return Member{}, invalid("Priority must fit int32; weight must be a positive int32")
	}
	var mask int
	if err := tx.GetContext(ctx, &mask, tx.Rebind(lockQuery(tx, `SELECT protocols FROM upstream_grants WHERE id=?`)), in.GrantID); err != nil {
		return Member{}, err
	}
	if err := validOrder(in.ProtocolOrder, mask); err != nil {
		return Member{}, err
	}
	if in.ProtocolOrder == nil {
		in.ProtocolOrder = store.DirectProtocolOrder{}
	}
	id, err := InsertNative(ctx, tx, "group_items", groupID, in.GrantID, in.Priority, weight, in.ProtocolOrder)
	if err != nil {
		return Member{}, err
	}
	var out Member
	err = tx.GetContext(ctx, &out, tx.Rebind(memberSelect+` WHERE i.id=?`), id)
	out.Ownership = "native"
	return out, err
}

func CreateMember(ctx context.Context, db *sqlx.DB, groupID int64, in MemberCreate) (Member, error) {
	var out Member
	err := Write(ctx, db, func(tx *sqlx.Tx) error {
		var exists int
		if err := tx.GetContext(ctx, &exists, tx.Rebind(lockQuery(tx, `SELECT 1 FROM upstream_groups WHERE id=?`)), groupID); err != nil {
			return err
		}
		var err error
		out, err = createMember(ctx, tx, groupID, in)
		return err
	})
	return out, err
}
