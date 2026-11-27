package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

type oauthRecordDB struct {
	*isolationDB
	lookup, write error
}

func (db *oauthRecordDB) LoadRouteUnitMemberWithAccount(ctx context.Context, unit, account int64) (*struct {
	Member  store.OAuthRouteUnitMember
	Account store.Account
	Unit    store.OAuthRouteUnit
}, error) {
	if db.lookup != nil {
		return nil, db.lookup
	}
	return db.isolationDB.LoadRouteUnitMemberWithAccount(ctx, unit, account)
}
func (db *oauthRecordDB) UpdateRouteUnitMemberSuccessFields(ctx context.Context, id int64, fields map[string]any) error {
	if db.write != nil {
		return db.write
	}
	return db.isolationDB.UpdateRouteUnitMemberSuccessFields(ctx, id, fields)
}
func (db *oauthRecordDB) UpdateRouteUnitMemberCooldownFields(ctx context.Context, id int64, fields map[string]any) error {
	if db.write != nil {
		return db.write
	}
	return db.isolationDB.UpdateRouteUnitMemberCooldownFields(ctx, id, fields)
}
func TestOAuthMemberRecordErrorsAndIsolation(t *testing.T) {
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)
	for _, op := range []string{"success", "failure", "probe-success", "probe-failure"} {
		for _, mode := range []string{"ok", "missing", "read-error", "write-error"} {
			t.Run(op+"/"+mode, func(t *testing.T) {
				db := &oauthRecordDB{isolationDB: newIsolationDB()}
				tr := newIsolationRouter(db.isolationDB)
				tr.db = db
				unit := int64(10)
				ch := isolationChannel(21, 1, 1)
				ch.OAuthRouteUnitID = &unit
				db.seedChannel(ch, isolationAccount(1, 4), isolationRoute(1, "weighted"))
				for _, id := range []int64{1, 2} {
					if mode == "missing" && id == 2 {
						continue
					}
					db.seedMember(store.OAuthRouteUnitMember{ID: 100 + id, UnitID: unit, AccountID: id}, isolationAccount(id, 4), store.OAuthRouteUnit{ID: unit})
				}
				fault := errors.New("fixture DB error")
				switch mode {
				case "read-error":
					db.lookup = fault
				case "write-error":
					db.write = fault
				}
				b := int64(2)
				ctx := context.Background()
				var err error
				switch op {
				case "success":
					err = tr.RecordSuccess(ctx, ch.ID, 1, 0, nil, &b)
				case "failure":
					err = tr.RecordFailure(ctx, ch.ID, SiteRuntimeFailureContext{}, &b)
				case "probe-success":
					err = tr.RecordProbeSuccess(ctx, ch.ID, 1, nil, &b)
				case "probe-failure":
					err = tr.RecordProbeFailure(ctx, ch.ID, SiteRuntimeFailureContext{}, &b)
				}
				if mode == "ok" {
					if err != nil || db.lastMemberID != 102 || db.memberCalls != 1 {
						t.Fatalf("member=%d calls=%d err=%v", db.lastMemberID, db.memberCalls, err)
					}
					return
				}
				if err == nil || mode != "missing" && !errors.Is(err, fault) || db.successCalls != 0 || db.cooldownCalls != 0 {
					t.Fatalf("mode=%s err=%v channel success/cooldown writes=%d/%d", mode, err, db.successCalls, db.cooldownCalls)
				}
			})
		}
	}
}
