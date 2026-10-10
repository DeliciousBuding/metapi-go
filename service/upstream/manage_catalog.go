package upstream

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

type ChannelCreate struct {
	Name           string                `json:"name"`
	Provider       string                `json:"provider"`
	Dialect        string                `json:"dialect"`
	Enabled        bool                  `json:"enabled"`
	BaseURL        string                `json:"baseUrl"`
	Endpoints      store.DirectEndpoints `json:"endpointConfig"`
	ChatPath       string                `json:"openaiChatCompletionPath"`
	ResponsesPath  string                `json:"openaiResponsePath"`
	MessagesPath   string                `json:"anthropicMessagePath"`
	UseSystemProxy bool                  `json:"useSystemProxy"`
	ChannelProxy   string                `json:"channelProxy"`
	CustomHeaders  string                `json:"customHeaders"`
	ParamOverride  string                `json:"paramOverride"`
}

func CreateChannel(ctx context.Context, db *sqlx.DB, in ChannelCreate) (int64, error) {
	var id int64
	err := Write(ctx, db, func(tx *sqlx.Tx) error {
		var err error
		id, err = InsertNative(ctx, tx, "channels", strings.TrimSpace(in.Name), in.Dialect, in.Provider, in.Enabled, in.BaseURL, in.Endpoints, in.ChatPath, in.ResponsesPath, in.MessagesPath, in.UseSystemProxy, in.ChannelProxy, in.CustomHeaders, in.ParamOverride, "")
		return err
	})
	return id, err
}

type Grant struct {
	ID             int64  `db:"id" json:"id"`
	ModelID        int64  `db:"model_id" json:"modelId"`
	CredentialID   int64  `db:"credential_id" json:"credentialId"`
	CredentialName string `db:"credential_name" json:"credentialName"`
	Enabled        bool   `db:"enabled" json:"enabled"`
	Protocols      int    `db:"protocols" json:"protocols"`
	MemberCount    int    `db:"member_count" json:"memberCount"`
	Origin         string `db:"origin_key" json:"-"`
	Ownership      string `json:"ownership"`
}
type Model struct {
	ID        int64   `db:"id" json:"id"`
	Name      string  `db:"name" json:"name"`
	Enabled   bool    `db:"enabled" json:"enabled"`
	Origin    string  `db:"origin_key" json:"-"`
	Ownership string  `json:"ownership"`
	Grants    []Grant `json:"grants"`
}

const grantSelect = `SELECT g.id,g.model_id,g.credential_id,k.name AS credential_name,g.enabled,g.protocols,g.origin_key,(SELECT COUNT(*) FROM upstream_group_items i WHERE i.grant_id=g.id) AS member_count FROM upstream_grants g JOIN upstream_credentials k ON k.id=g.credential_id`

func ListModels(ctx context.Context, db *sqlx.DB, channelID int64) ([]Model, error) {
	var exists int
	if err := db.GetContext(ctx, &exists, db.Rebind(`SELECT 1 FROM upstream_channels WHERE id=?`), channelID); err != nil {
		return nil, normalizeError(err)
	}
	models := []Model{}
	if err := db.SelectContext(ctx, &models, db.Rebind(`SELECT id,name,enabled,origin_key FROM upstream_models WHERE channel_id=? ORDER BY name,id`), channelID); err != nil {
		return nil, err
	}
	grants := []Grant{}
	if err := db.SelectContext(ctx, &grants, db.Rebind(grantSelect+` WHERE g.model_id IN (SELECT id FROM upstream_models WHERE channel_id=?) ORDER BY g.id`), channelID); err != nil {
		return nil, err
	}
	byModel := map[int64][]Grant{}
	for _, g := range grants {
		g.Ownership = Ownership(g.Origin)
		byModel[g.ModelID] = append(byModel[g.ModelID], g)
	}
	for i := range models {
		models[i].Ownership = Ownership(models[i].Origin)
		models[i].Grants = byModel[models[i].ID]
		if models[i].Grants == nil {
			models[i].Grants = []Grant{}
		}
	}
	return models, nil
}

type ModelsCreate struct {
	Name    *string  `json:"name"`
	Names   []string `json:"names"`
	Enabled *bool    `json:"enabled"`
}

func CreateModels(ctx context.Context, db *sqlx.DB, channelID int64, in ModelsCreate) ([]Model, error) {
	if in.Name != nil && in.Names != nil {
		return nil, invalid("Use name or names, not both")
	}
	names := in.Names
	if in.Name != nil {
		names = []string{*in.Name}
	}
	if len(names) == 0 || len(names) > 500 {
		return nil, invalid("Select between 1 and 500 model names")
	}
	seen := map[string]bool{}
	for i, n := range names {
		n = strings.TrimSpace(n)
		if !validName(n) || seen[n] {
			return nil, invalid("Model names must be non-empty and unique")
		}
		seen[n] = true
		names[i] = n
	}
	out := []Model{}
	err := Write(ctx, db, func(tx *sqlx.Tx) error {
		var exists int
		if err := tx.GetContext(ctx, &exists, tx.Rebind(lockQuery(tx, `SELECT 1 FROM upstream_channels WHERE id=?`)), channelID); err != nil {
			return err
		}
		for _, n := range names {
			id, err := InsertNative(ctx, tx, "models", channelID, n, boolDefault(in.Enabled, true))
			if err != nil {
				return err
			}
			out = append(out, Model{ID: id, Name: n, Enabled: boolDefault(in.Enabled, true), Ownership: "native", Grants: []Grant{}})
		}
		return nil
	})
	return out, err
}

type ModelUpdate struct {
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
}

func UpdateModel(ctx context.Context, db *sqlx.DB, id int64, in ModelUpdate) ([]int64, error) {
	if in.Name == nil && in.Enabled == nil {
		return nil, invalid("Empty model update")
	}
	if in.Name != nil {
		*in.Name = strings.TrimSpace(*in.Name)
		if !validName(*in.Name) {
			return nil, invalid("Invalid model name")
		}
	}
	affected := []int64{}
	err := Write(ctx, db, func(tx *sqlx.Tx) error {
		var row Model
		if err := tx.GetContext(ctx, &row, tx.Rebind(lockQuery(tx, `SELECT id,name,enabled,origin_key FROM upstream_models WHERE id=?`)), id); err != nil {
			return err
		}
		if in.Name != nil {
			row.Name = *in.Name
		}
		if in.Enabled != nil {
			row.Enabled = *in.Enabled
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE upstream_models SET name=?,enabled=? WHERE id=?`), row.Name, row.Enabled, id); err != nil {
			return err
		}
		return tx.SelectContext(ctx, &affected, tx.Rebind(`SELECT DISTINCT rg.route_id FROM upstream_route_groups rg JOIN upstream_group_items i ON i.group_id=rg.group_id JOIN upstream_grants g ON g.id=i.grant_id WHERE g.model_id=? ORDER BY rg.route_id`), id)
	})
	return affected, err
}

type GrantCreate struct {
	ModelID      int64 `json:"modelId"`
	CredentialID int64 `json:"credentialId"`
	Protocols    []int `json:"protocols"`
	Enabled      *bool `json:"enabled"`
}
type GrantUpdate struct {
	Protocols *[]int `json:"protocols"`
	Enabled   *bool  `json:"enabled"`
}

func protocolMask(protocols []int) (int, error) {
	if len(protocols) == 0 {
		return 0, invalid("Grant protocols must not be empty")
	}
	mask := 0
	for _, p := range protocols {
		if !store.ValidDirectProtocol(p) || mask&p != 0 {
			return 0, invalid("Grant protocols must be unique supported protocol bits")
		}
		mask |= p
	}
	return mask, nil
}

func validateGrantChannel(ctx context.Context, tx *sqlx.Tx, modelID, credentialID int64, mask int) error {
	var channelID, credentialChannel int64
	if err := tx.GetContext(ctx, &channelID, tx.Rebind(`SELECT channel_id FROM upstream_models WHERE id=?`), modelID); err != nil {
		return err
	}
	if err := tx.GetContext(ctx, &credentialChannel, tx.Rebind(`SELECT channel_id FROM upstream_credentials WHERE id=?`), credentialID); err != nil {
		return err
	}
	if channelID != credentialChannel {
		return invalid("Model and credential must belong to the same channel")
	}
	var endpoints store.DirectEndpoints
	var chat, responses, messages string
	query := lockQuery(tx, `SELECT endpoint_config,openai_chat_completion_path,openai_response_path,anthropic_message_path FROM upstream_channels WHERE id=?`)
	if err := tx.QueryRowxContext(ctx, tx.Rebind(query), channelID).Scan(&endpoints, &chat, &responses, &messages); err != nil {
		return err
	}
	available := 0
	if endpoints.IsConfigured() {
		available = endpoints.ProtocolMask()
	} else {
		if chat != "" {
			available |= 2
		}
		if responses != "" {
			available |= 4
		}
		if messages != "" {
			available |= 8
		}
	}
	if mask&available != mask {
		return invalid("Grant protocols must be supported by the channel endpoints")
	}
	return nil
}

func CreateGrant(ctx context.Context, db *sqlx.DB, in GrantCreate) (Grant, error) {
	mask, err := protocolMask(in.Protocols)
	if err != nil {
		return Grant{}, err
	}
	var out Grant
	err = Write(ctx, db, func(tx *sqlx.Tx) error {
		if err := validateGrantChannel(ctx, tx, in.ModelID, in.CredentialID, mask); err != nil {
			return err
		}
		id, err := InsertNative(ctx, tx, "grants", in.ModelID, in.CredentialID, mask, boolDefault(in.Enabled, true))
		if err != nil {
			return err
		}
		return tx.GetContext(ctx, &out, tx.Rebind(grantSelect+` WHERE g.id=?`), id)
	})
	out.Ownership = Ownership(out.Origin)
	return out, err
}

func UpdateGrant(ctx context.Context, db *sqlx.DB, id int64, in GrantUpdate) (Grant, error) {
	if in.Protocols == nil && in.Enabled == nil {
		return Grant{}, invalid("Empty grant update")
	}
	var mask int
	var err error
	if in.Protocols != nil {
		mask, err = protocolMask(*in.Protocols)
		if err != nil {
			return Grant{}, err
		}
	}
	var out Grant
	err = Write(ctx, db, func(tx *sqlx.Tx) error {
		// Match channel-edit lock order before locking the shared grant.
		var modelID, credentialID int64
		if err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT model_id,credential_id FROM upstream_grants WHERE id=?`), id).Scan(&modelID, &credentialID); err != nil {
			return err
		}
		if in.Protocols != nil {
			if err := validateGrantChannel(ctx, tx, modelID, credentialID, mask); err != nil {
				return err
			}
		}
		var currentMask int
		if err := tx.GetContext(ctx, &currentMask, tx.Rebind(lockQuery(tx, `SELECT protocols FROM upstream_grants WHERE id=?`)), id); err != nil {
			return err
		}
		if err := tx.GetContext(ctx, &out, tx.Rebind(grantSelect+` WHERE g.id=?`), id); err != nil {
			return err
		}
		if in.Protocols != nil {
			rows := []struct {
				ID    int64                     `db:"id"`
				Order store.DirectProtocolOrder `db:"protocol_order"`
			}{}
			if err := tx.SelectContext(ctx, &rows, tx.Rebind(`SELECT id,protocol_order FROM upstream_group_items WHERE grant_id=? ORDER BY id`), id); err != nil {
				return err
			}
			bad := []int64{}
			for _, row := range rows {
				for _, p := range row.Order {
					if p&mask == 0 {
						bad = append(bad, row.ID)
						break
					}
				}
			}
			if len(bad) > 0 {
				return &Error{Status: 409, Message: "Member protocol orders still reference removed protocols", MemberIDs: bad}
			}
			out.Protocols = mask
		} else {
			out.Protocols = currentMask
		}
		if in.Enabled != nil {
			out.Enabled = *in.Enabled
		}
		_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE upstream_grants SET protocols=?,enabled=? WHERE id=?`), out.Protocols, out.Enabled, id)
		return err
	})
	out.Ownership = Ownership(out.Origin)
	return out, err
}

func validOrder(order store.DirectProtocolOrder, mask int) error {
	raw, _ := json.Marshal(order)
	if order == nil {
		raw = []byte(`[]`)
	}
	var checked store.DirectProtocolOrder
	if checked.Scan(string(raw)) != nil {
		return invalid("Invalid member protocol order")
	}
	for _, p := range order {
		if p&mask == 0 {
			return invalid("Member protocol order must be a subset of its grant")
		}
	}
	return nil
}
