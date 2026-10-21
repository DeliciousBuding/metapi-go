package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"math/big"
	"strings"
	"sync"
)

var activeUserBudgetRequests sync.Map

// TrackUserBudgetRequest is owned by the ingress lifecycle. It blocks local
// reconciliation while a request is actually running. It is not a distributed
// lease: operators must still verify termination on every serving instance.
func TrackUserBudgetRequest(requestID string) func() {
	activeUserBudgetRequests.Store(requestID, struct{}{})
	return func() { activeUserBudgetRequests.Delete(requestID) }
}

// User budgets are token credit, independent of upstream cost: one credit is
// one million reported total tokens at multiplier 1. All arithmetic is integer
// microcredits; charges round up, never down. Pricing changes affect new holds.
const userBudgetScale int64 = 1_000_000
const maxUserBudgetUnits int64 = 1_000_000_000_000_000

var (
	ErrUserBudgetExceeded = errors.New("user budget exhausted or reserved")
	ErrUserBudgetUnknown  = errors.New("user budget accounting unresolved")
	ErrUserBudgetConflict = errors.New("user budget request identity conflict")
)

type UserBudget struct {
	Limit           float64 `json:"limit"`
	Used            float64 `json:"used"`
	Reserved        float64 `json:"reserved"`
	Unlimited       bool    `json:"unlimited"`
	Frozen          bool    `json:"frozen"`
	UnknownRequests int64   `json:"unknownRequests"`
}
type UserGroupPricing struct {
	Multiplier float64 `json:"multiplier"`
}

type UserBudgetHold struct {
	RequestID   string  `db:"request_id" json:"requestId"`
	State       string  `db:"state" json:"state"`
	Reserved    float64 `db:"reserved" json:"reserved"`
	TotalTokens *int64  `db:"total_tokens" json:"totalTokens"`
	CreatedAt   string  `db:"created_at" json:"createdAt"`
	Multiplier  float64 `db:"multiplier" json:"multiplier"`
}

// Administrator-only caller: ordinary users receive aggregate budget DTOs,
// never a cross-request operational reconciliation surface.
func (s *GatewayUserStore) ListUserBudgetHolds(ctx context.Context, userID int64, offset int) ([]UserBudgetHold, error) {
	if _, err := s.GetUser(ctx, userID); err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, ErrGatewayUserInvalid
	}
	rows := []UserBudgetHold{}
	err := s.db.SelectContext(ctx, &rows, s.db.Rebind(`SELECT request_id,state,reserved_units*1.0/1000000 AS reserved,total_tokens,created_at,multiplier_units*1.0/1000000 AS multiplier FROM gateway_user_budget_ledger WHERE user_id=? AND state IN ('pending','unknown') ORDER BY created_at DESC,request_id DESC LIMIT 100 OFFSET ?`), userID, offset)
	return rows, err
}

type UserBudgetReservation struct {
	RequestID       string `db:"request_id"`
	UserID          int64  `db:"user_id"`
	GroupID         int64  `db:"group_id"`
	MultiplierUnits int64  `db:"multiplier_units"`
	ReservedUnits   int64  `db:"reserved_units"`
	ChargedUnits    *int64 `db:"charged_units"`
	TotalTokens     *int64 `db:"total_tokens"`
	State           string `db:"state"`
}

type userBudgetRow struct {
	Limit     int64 `db:"limit_units"`
	Used      int64 `db:"used_units"`
	Reserved  int64 `db:"reserved_units"`
	Unlimited bool  `db:"unlimited"`
}

func (r userBudgetRow) dto() UserBudget {
	return UserBudget{Limit: float64(r.Limit) / 1e6, Used: float64(r.Used) / 1e6, Reserved: float64(r.Reserved) / 1e6, Unlimited: r.Unlimited}
}

func budgetUnits(value float64, positive bool) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(maxUserBudgetUnits)/1e6 || positive && value == 0 {
		return 0, ErrGatewayUserInvalid
	}
	scaled := value * 1e6
	units := int64(math.Round(scaled))
	if math.Abs(scaled-float64(units)) > 0.0001 || positive && units == 0 {
		return 0, ErrGatewayUserInvalid
	}
	return units, nil
}

func (s *GatewayUserStore) GetUserBudget(ctx context.Context, userID int64) (UserBudget, error) {
	if err := s.available(); err != nil {
		return UserBudget{}, err
	}
	var row userBudgetRow
	err := s.db.GetContext(ctx, &row, s.db.Rebind(`SELECT COALESCE(b.limit_units,0) AS limit_units,COALESCE(b.used_units,0) AS used_units,COALESCE(b.reserved_units,0) AS reserved_units,COALESCE(b.unlimited,FALSE) AS unlimited FROM gateway_users u LEFT JOIN gateway_user_budgets b ON b.user_id=u.id WHERE u.id=?`), userID)
	if err != nil {
		return UserBudget{}, err
	}
	out := row.dto()
	err = s.db.GetContext(ctx, &out.UnknownRequests, s.db.Rebind(`SELECT COUNT(*) FROM gateway_user_budget_ledger WHERE user_id=? AND state='unknown'`), userID)
	out.Frozen = out.UnknownRequests > 0
	return out, err
}

func (s *GatewayUserStore) SetUserBudget(ctx context.Context, userID int64, limit float64, unlimited bool) (UserBudget, error) {
	units, err := budgetUnits(limit, false)
	if err != nil {
		return UserBudget{}, err
	}
	if err = s.available(); err != nil {
		return UserBudget{}, err
	}
	// Updating the limit never resets used or in-flight holds, including debt.
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO gateway_user_budgets(user_id,limit_units,unlimited) SELECT id,?,? FROM gateway_users WHERE id=? ON CONFLICT(user_id) DO UPDATE SET limit_units=excluded.limit_units,unlimited=excluded.unlimited`), units, unlimited, userID)
	if err = gatewayUserChanged(result, err); err != nil {
		return UserBudget{}, err
	}
	return s.GetUserBudget(ctx, userID)
}
func (s *GatewayUserStore) GetUserGroupPricing(ctx context.Context, groupID int64) (UserGroupPricing, error) {
	if err := s.available(); err != nil {
		return UserGroupPricing{}, err
	}
	var units int64
	err := s.db.GetContext(ctx, &units, s.db.Rebind(`SELECT COALESCE(p.multiplier_units,1000000) FROM gateway_user_groups g LEFT JOIN gateway_user_group_pricing p ON p.group_id=g.id WHERE g.id=?`), groupID)
	return UserGroupPricing{float64(units) / 1e6}, err
}
func (s *GatewayUserStore) SetUserGroupPricing(ctx context.Context, groupID int64, multiplier float64) (UserGroupPricing, error) {
	units, err := budgetUnits(multiplier, true)
	if err != nil {
		return UserGroupPricing{}, err
	}
	if err = s.available(); err != nil {
		return UserGroupPricing{}, err
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO gateway_user_group_pricing(group_id,multiplier_units) SELECT id,? FROM gateway_user_groups WHERE id=? ON CONFLICT(group_id) DO UPDATE SET multiplier_units=excluded.multiplier_units`), units, groupID)
	if err = gatewayUserChanged(result, err); err != nil {
		return UserGroupPricing{}, err
	}
	return s.GetUserGroupPricing(ctx, groupID)
}

const userBudgetLedgerSelect = `SELECT request_id,user_id,group_id,multiplier_units,reserved_units,charged_units,total_tokens,state FROM gateway_user_budget_ledger WHERE request_id=?`

// ReserveUserBudget reserves a bounded token estimate across every Key. An
// estimate is not a strict promise about third-party reported usage.
// A crashed process leaves its durable hold;
// elapsed wall time never turns an unobserved charge into free credit.
func (s *GatewayUserStore) ReserveUserBudget(ctx context.Context, requestID string, userID, expectedGroupID, estimatedTokens int64) (UserBudgetReservation, error) {
	if err := s.available(); err != nil {
		return UserBudgetReservation{}, err
	}
	if !strings.HasPrefix(requestID, "req_") || len(requestID) > 128 || userID <= 0 || expectedGroupID <= 0 || estimatedTokens <= 0 {
		return UserBudgetReservation{}, ErrGatewayUserInvalid
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return UserBudgetReservation{}, err
	}
	defer tx.Rollback()
	// First statement is a write: serialize both SQLite writers and PG row
	// lockers before reading balance, preventing stale check-then-increment.
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gateway_user_budgets(user_id) VALUES(?) ON CONFLICT(user_id) DO UPDATE SET user_id=excluded.user_id`), userID)
	if err != nil {
		return UserBudgetReservation{}, err
	}
	var existing UserBudgetReservation
	err = tx.GetContext(ctx, &existing, tx.Rebind(userBudgetLedgerSelect), requestID)
	if err == nil {
		if existing.UserID != userID || existing.GroupID != expectedGroupID || existing.State != "pending" {
			return UserBudgetReservation{}, ErrUserBudgetConflict
		}
		return existing, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return UserBudgetReservation{}, err
	}
	var groupID int64
	err = tx.GetContext(ctx, &groupID, tx.Rebind(`SELECT u.group_id FROM gateway_users u JOIN gateway_user_groups g ON g.id=u.group_id WHERE u.id=? AND u.enabled=TRUE AND g.enabled=TRUE`), userID)
	if err != nil {
		return UserBudgetReservation{}, err
	}
	if groupID != expectedGroupID {
		return UserBudgetReservation{}, ErrUserBudgetConflict
	}
	var unresolved int
	if err = tx.GetContext(ctx, &unresolved, tx.Rebind(`SELECT COUNT(*) FROM gateway_user_budget_ledger WHERE user_id=? AND state='unknown'`), userID); err != nil {
		return UserBudgetReservation{}, err
	}
	if unresolved > 0 {
		return UserBudgetReservation{}, ErrUserBudgetUnknown
	}
	var row userBudgetRow
	if err = tx.GetContext(ctx, &row, tx.Rebind(`SELECT limit_units,used_units,reserved_units,unlimited FROM gateway_user_budgets WHERE user_id=?`), userID); err != nil {
		return UserBudgetReservation{}, err
	}
	multiplier := userBudgetScale
	var stored int64
	err = tx.GetContext(ctx, &stored, tx.Rebind(`SELECT multiplier_units FROM gateway_user_group_pricing WHERE group_id=?`), groupID)
	if err == nil {
		multiplier = stored
	} else if !errors.Is(err, sql.ErrNoRows) {
		return UserBudgetReservation{}, err
	}
	hold, valid := userTokenCharge(estimatedTokens, multiplier)
	if !valid {
		return UserBudgetReservation{}, ErrGatewayUserInvalid
	}
	if !row.Unlimited && (row.Used >= row.Limit || row.Reserved > row.Limit-row.Used || hold > row.Limit-row.Used-row.Reserved) {
		return UserBudgetReservation{}, ErrUserBudgetExceeded
	}
	if row.Reserved > maxUserBudgetUnits-hold {
		return UserBudgetReservation{}, ErrUserBudgetExceeded
	}
	if _, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE gateway_user_budgets SET reserved_units=reserved_units+? WHERE user_id=?`), hold, userID); err != nil {
		return UserBudgetReservation{}, err
	}
	if _, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gateway_user_budget_ledger(request_id,user_id,group_id,multiplier_units,reserved_units,state,created_at) VALUES(?,?,?,?,?,'pending',?)`), requestID, userID, groupID, multiplier, hold, gatewayUserNow()); err != nil {
		return UserBudgetReservation{}, err
	}
	return UserBudgetReservation{RequestID: requestID, UserID: userID, GroupID: groupID, MultiplierUnits: multiplier, ReservedUnits: hold, State: "pending"}, tx.Commit()
}

// SettleUserBudget is a request/WS-turn operation, NEVER an attempt operation.
// nil or malformed usage retains the hold and blocks subsequent admissions.
// Actual charges can exceed a hold: this is truthful debt accounting, not a
// claim that a third-party model has a strictly bounded cost.
func (s *GatewayUserStore) SettleUserBudget(ctx context.Context, requestID string, tokens *int64, dispatched bool) error {
	return s.settleUserBudget(ctx, requestID, tokens, dispatched, 0)
}

// ReconcileUserBudget is an explicit administrator correction for unresolved
// holds, including crash-surviving pending entries. It uses the original group
// multiplier. A caller must supply verified TOTAL tokens, not an increment.
func (s *GatewayUserStore) ReconcileUserBudget(ctx context.Context, userID int64, requestID string, totalTokens int64) error {
	if userID <= 0 || totalTokens < 0 {
		return ErrGatewayUserInvalid
	}
	if _, active := activeUserBudgetRequests.Load(requestID); active {
		return ErrUserBudgetConflict
	}
	return s.settleUserBudget(ctx, requestID, &totalTokens, true, userID)
}

func (s *GatewayUserStore) settleUserBudget(ctx context.Context, requestID string, tokens *int64, dispatched bool, reconcileUserID int64) error {
	if err := s.available(); err != nil {
		return err
	}
	var userID int64
	if err := s.db.GetContext(ctx, &userID, s.db.Rebind(`SELECT user_id FROM gateway_user_budget_ledger WHERE request_id=?`), requestID); err != nil {
		return err
	}
	if reconcileUserID != 0 && reconcileUserID != userID {
		return sql.ErrNoRows
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE gateway_user_budgets SET user_id=user_id WHERE user_id=?`), userID); err != nil {
		return err
	}
	var hold UserBudgetReservation
	if err = tx.GetContext(ctx, &hold, tx.Rebind(userBudgetLedgerSelect), requestID); err != nil {
		return err
	}
	if hold.State != "pending" && !(reconcileUserID != 0 && hold.State == "unknown") {
		if reconcileUserID != 0 && (hold.TotalTokens == nil || tokens == nil || *hold.TotalTokens != *tokens) {
			return ErrUserBudgetConflict
		}
		return tx.Commit()
	}
	state, charge := "settled", int64(0)
	if !dispatched {
		state = "released"
		tokens = nil
	} else if tokens == nil || *tokens < 0 {
		state = "unknown"
	} else {
		var valid bool
		charge, valid = userTokenCharge(*tokens, hold.MultiplierUnits)
		if !valid {
			if reconcileUserID != 0 {
				return ErrGatewayUserInvalid
			}
			state = "unknown"
		}
	}
	if state == "unknown" {
		_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE gateway_user_budget_ledger SET state='unknown',settled_at=? WHERE request_id=?`), gatewayUserNow(), requestID)
	} else {
		var used int64
		if err = tx.GetContext(ctx, &used, tx.Rebind(`SELECT used_units FROM gateway_user_budgets WHERE user_id=?`), userID); err != nil {
			return err
		}
		if used > maxUserBudgetUnits-charge {
			return ErrUserBudgetUnknown
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE gateway_user_budgets SET used_units=used_units+?,reserved_units=reserved_units-? WHERE user_id=?`), charge, hold.ReservedUnits, userID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE gateway_user_budget_ledger SET state=?,charged_units=?,total_tokens=?,settled_at=? WHERE request_id=?`), state, charge, tokens, gatewayUserNow(), requestID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func userTokenCharge(tokens, multiplier int64) (int64, bool) {
	if tokens < 0 || multiplier <= 0 {
		return 0, false
	}
	cost := new(big.Int).Mul(big.NewInt(tokens), big.NewInt(multiplier))
	cost.Add(cost, big.NewInt(userBudgetScale-1))
	cost.Div(cost, big.NewInt(userBudgetScale))
	if !cost.IsInt64() || cost.Int64() > maxUserBudgetUnits {
		return 0, false
	}
	return cost.Int64(), true
}
