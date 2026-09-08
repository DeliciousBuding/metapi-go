package service

import (
	"context"
	"net/http"
	"testing"
)

// An authoritative empty list is not a network failure. It must retire stale
// discovered channels without deleting manual model declarations or widening a
// downstream route grant. A failed request must leave the last snapshot intact.
func TestModelRefreshEmptySnapshotRetiresAutomaticChannelsButFailureDoesNot(t *testing.T) {
	for _, platformName := range []string{"openai", "new-api"} {
		for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
			label := "empty"
			if status != http.StatusOK {
				label = "unauthorized"
			}
			t.Run(platformName+"/"+label, func(t *testing.T) {
				db := openModelRefreshTestDB(t)
				upstream := startModelsUpstream(t, status)
				siteID := seedSiteOnPlatform(t, db, "lifecycle", upstream.URL, platformName)
				accountID, tokenID := seedAccountOnSite(t, db, siteID, "lifecycle-account", "sk-lifecycle-test", "active")
				if _, err := db.Exec("INSERT INTO model_availability (account_id, model_name, available, is_manual) VALUES (?, 'retired-model', ?, ?), (?, 'manual-model', ?, ?)", accountID, true, false, accountID, true, true); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec("INSERT INTO token_model_availability (token_id, model_name, available) VALUES (?, 'retired-model', ?)", tokenID, true); err != nil {
					t.Fatal(err)
				}
				first, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db, RebuildOptions{CreateModelRoutes: true})
				if err != nil || first.RoutesCreated != 1 {
					t.Fatalf("seed routing: %+v %v", first, err)
				}
				var beforeChannels int
				if err := db.Get(&beforeChannels, "SELECT COUNT(*) FROM route_channels"); err != nil {
					t.Fatal(err)
				}
				result := RefreshAccountModels(context.Background(), db, accountID, false, false)
				var automaticAvailable, tokenAvailable, manualAvailable, channels, routes int
				for _, query := range []struct {
					sql  string
					dest *int
				}{
					{"SELECT COUNT(*) FROM model_availability WHERE available AND NOT is_manual", &automaticAvailable},
					{"SELECT COUNT(*) FROM model_availability WHERE available AND is_manual", &manualAvailable},
					{"SELECT COUNT(*) FROM token_model_availability WHERE available", &tokenAvailable},
					{"SELECT COUNT(*) FROM route_channels", &channels},
					{"SELECT COUNT(*) FROM token_routes", &routes},
				} {
					if err := db.Get(query.dest, query.sql); err != nil {
						t.Fatal(err)
					}
				}
				if manualAvailable != 1 || routes != 1 {
					t.Fatalf("manual model or route identity lost: manual=%d routes=%d", manualAvailable, routes)
				}
				if status == http.StatusOK {
					if result.Success || result.ErrorCode != "empty_models" || !result.RebuildRan || automaticAvailable != 0 || tokenAvailable != 0 || channels != 0 {
						t.Fatalf("empty upstream left stale routing: result=%+v account=%d token=%d channels=%d", result, automaticAvailable, tokenAvailable, channels)
					}
				} else if result.Success || result.ErrorCode == "empty_models" || result.RebuildRan || automaticAvailable != 1 || tokenAvailable != 1 || channels != beforeChannels {
					t.Fatalf("failure erased last observed availability: result=%+v account=%d token=%d channels=%d", result, automaticAvailable, tokenAvailable, channels)
				}
			})
		}
	}
}
