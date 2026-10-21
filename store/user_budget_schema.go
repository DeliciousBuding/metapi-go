package store

import "fmt"

func buildGatewayUserBudgetsDDL(dialect string) string {
	boolean := "INTEGER"
	if isPG(dialect) {
		boolean = "BOOLEAN"
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS gateway_user_budgets (
		 user_id BIGINT PRIMARY KEY REFERENCES gateway_users(id) ON DELETE RESTRICT,
		 limit_units BIGINT NOT NULL DEFAULT 0 CHECK(limit_units>=0),
		 used_units BIGINT NOT NULL DEFAULT 0 CHECK(used_units>=0),
		 reserved_units BIGINT NOT NULL DEFAULT 0 CHECK(reserved_units>=0),
		 unlimited %s NOT NULL DEFAULT FALSE)`, boolean)
}
func buildGatewayUserGroupPricingDDL(_ string) string {
	return `CREATE TABLE IF NOT EXISTS gateway_user_group_pricing (
		 group_id BIGINT PRIMARY KEY REFERENCES gateway_user_groups(id) ON DELETE CASCADE,
		 multiplier_units BIGINT NOT NULL CHECK(multiplier_units>0))`
}
func buildGatewayUserBudgetLedgerDDL(_ string) string {
	return `CREATE TABLE IF NOT EXISTS gateway_user_budget_ledger (
		 request_id TEXT PRIMARY KEY,
		 user_id BIGINT NOT NULL REFERENCES gateway_users(id) ON DELETE RESTRICT,
		 group_id BIGINT NOT NULL,
		 multiplier_units BIGINT NOT NULL CHECK(multiplier_units>0),
		 reserved_units BIGINT NOT NULL CHECK(reserved_units>=0),
		 charged_units BIGINT,
		 total_tokens BIGINT,
		 state TEXT NOT NULL CHECK(state IN ('pending','settled','released','unknown')),
		 created_at TEXT NOT NULL,
		 settled_at TEXT)`
}
func migrateGatewayUserBudgets(db *DB) error {
	for _, query := range []string{
		buildGatewayUserBudgetsDDL(db.Dialect),
		buildGatewayUserGroupPricingDDL(db.Dialect),
		buildGatewayUserBudgetLedgerDDL(db.Dialect),
		`CREATE INDEX IF NOT EXISTS gateway_user_budget_ledger_user_idx ON gateway_user_budget_ledger(user_id,state)`,
	} {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}
