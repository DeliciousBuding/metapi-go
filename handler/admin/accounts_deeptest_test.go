package admin

// Deep-testing probe: concurrent read-modify-write on accounts.extra_config.
//
// updateAccount (accounts.go) computes the merged extraConfig from a row it
// read BEFORE the write (service.MergeExtraConfig over the prior snapshot)
// and then persists it via service.UpdateAccountFields. There is no
// transaction spanning read+merge+write and no optimistic-locking column, so
// two concurrent PUT /api/accounts/{id} requests that each patch a distinct
// extraConfig key can interleave as:
//
//	req A reads extraConfig={}      req B reads extraConfig={}
//	req A writes {keyA}             req B writes {keyB}   <- keyA is lost
//
// This probe fires N concurrent PUTs, each adding one unique key, and asserts
// that the persisted extraConfig still contains every key. A failure here is
// a real lost-update defect (admin UI edits silently disappearing when two
// operators / automations touch the same account).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
)

func TestProbe_UpdateAccount_ConcurrentExtraConfigPatchesDoNotLoseKeys(t *testing.T) {
	const rounds = 8
	const concurrentPatches = 16

	for round := 0; round < rounds; round++ {
		db, router, _ := setupAccountsTest(t)
		_, accountID := setupAccountFixture(t, router)

		var startBarrier sync.WaitGroup
		startBarrier.Add(1)
		var finished sync.WaitGroup
		statuses := make([]int, concurrentPatches)

		for worker := 0; worker < concurrentPatches; worker++ {
			finished.Add(1)
			go func(workerIndex int) {
				defer finished.Done()
				startBarrier.Wait()
				key := fmt.Sprintf("probeKey_%d_%d", round, workerIndex)
				body := map[string]any{
					"extraConfig": map[string]any{key: "v" + strconv.Itoa(workerIndex)},
				}
				resp := doPutJSON(t, router, "/api/accounts/"+strconv.FormatInt(accountID, 10), body)
				statuses[workerIndex] = resp.Code
			}(worker)
		}
		startBarrier.Done()
		finished.Wait()

		for worker, code := range statuses {
			if code != http.StatusOK {
				t.Fatalf("round %d: worker %d PUT failed with status %d", round, worker, code)
			}
		}

		row, err := service.GetAccountWithSiteByID(db.DB, accountID)
		if err != nil {
			t.Fatalf("round %d: reload account: %v", round, err)
		}
		merged := map[string]any{}
		if row.Account.ExtraConfig != nil && *row.Account.ExtraConfig != "" {
			if err := json.Unmarshal([]byte(*row.Account.ExtraConfig), &merged); err != nil {
				t.Fatalf("round %d: extraConfig is not valid JSON: %v (%q)", round, err, *row.Account.ExtraConfig)
			}
		}

		var missingKeys []string
		for worker := 0; worker < concurrentPatches; worker++ {
			key := fmt.Sprintf("probeKey_%d_%d", round, worker)
			if _, ok := merged[key]; !ok {
				missingKeys = append(missingKeys, key)
			}
		}
		if len(missingKeys) > 0 {
			t.Fatalf("round %d: %d/%d concurrently patched extraConfig keys were lost (last write wins over the merged snapshot): %v",
				round, len(missingKeys), concurrentPatches, missingKeys)
		}
	}
}
