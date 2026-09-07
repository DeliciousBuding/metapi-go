package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

func readRebuildEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func awaitRebuildTask(t *testing.T, r chi.Router, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tasks/"+id, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("task read status=%d: %s", rec.Code, rec.Body.String())
		}
		body := readRebuildEnvelope(t, rec)
		task, ok := body["task"].(map[string]any)
		if !ok {
			t.Fatalf("missing task: %v", body)
		}
		if task["status"] == "succeeded" || task["status"] == "failed" {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("rebuild task did not finish")
	return nil
}

func TestTokenRoutes_RebuildAsyncAcknowledgesAndDeduplicatesBeforeWorkCompletes(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	resetBackgroundTasksForTests()
	RegisterTasksRoutes(r, db.DB)
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	var calls atomic.Int32
	restore := service.SetModelRefreshSideEffectsForTest(func(ctx context.Context, db *sqlx.DB) (service.RouteRebuildStats, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		if err := ctx.Err(); err != nil {
			return service.RouteRebuildStats{}, err
		}
		return service.RebuildTokenRoutesFromAvailability(ctx, db)
	}, func() {})
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		resetBackgroundTasksForTests()
		restore()
		SetBackgroundTaskDB(nil)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/routes/rebuild", strings.NewReader(`{"wait":false,"refreshModels":true}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	returned := make(chan struct{})
	go func() { r.ServeHTTP(rec, req); close(returned) }()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		releaseOnce.Do(func() { close(release) })
		<-returned
		t.Fatal("wait:false waited for the upstream/rebuild work instead of acknowledging a task")
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d: %s", rec.Code, rec.Body.String())
	}
	first := readRebuildEnvelope(t, rec)
	id, ok := first["taskId"].(string)
	if !ok || id == "" || first["jobId"] != id || first["queued"] != true || first["reused"] != false {
		t.Fatalf("not a new observable task: %v", first)
	}
	cancel() // Closing the original page must not cancel the queued work.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}
	secondRec := httptest.NewRecorder()
	r.ServeHTTP(secondRec, httptest.NewRequest(http.MethodPost, "/api/routes/rebuild", strings.NewReader(`{"wait":false,"refreshModels":true}`)))
	second := readRebuildEnvelope(t, secondRec)
	if secondRec.Code != http.StatusAccepted || second["taskId"] != id || second["reused"] != true {
		t.Fatalf("identical in-flight request not reused: %v", second)
	}
	releaseOnce.Do(func() { close(release) })
	task := awaitRebuildTask(t, r, id)
	if task["type"] != routesRebuildTaskType || task["status"] != "succeeded" || calls.Load() != 1 {
		t.Fatalf("wrong final lifecycle/calls=%d: %v", calls.Load(), task)
	}
	result, ok := task["result"].(map[string]any)
	if !ok || result["status"] != "completed" || result["modelRefresh"] == nil {
		t.Fatalf("completed task omitted real rebuild result: %v", task)
	}
}

func TestTokenRoutes_RebuildAsyncActuallyWritesChannels(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	resetBackgroundTasksForTests()
	RegisterTasksRoutes(r, db.DB)
	t.Cleanup(func() { resetBackgroundTasksForTests(); SetBackgroundTaskDB(nil) })
	routeID, _, tokenID := seedRouteChannelRefs(t, db)
	if _, err := db.Exec(`INSERT INTO token_model_availability (token_id, model_name, available) VALUES (?, 'gpt-4o', TRUE)`, tokenID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM route_channels WHERE route_id = ?`, routeID); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/routes/rebuild", strings.NewReader(`{"wait":false,"refreshModels":false}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d: %s", rec.Code, rec.Body.String())
	}
	body := readRebuildEnvelope(t, rec)
	task := awaitRebuildTask(t, r, body["taskId"].(string))
	if task["status"] != "succeeded" {
		t.Fatalf("rebuild failed: %v", task)
	}
	var channels int
	if err := db.Get(&channels, `SELECT COUNT(*) FROM route_channels WHERE route_id = ?`, routeID); err != nil {
		t.Fatal(err)
	}
	result := task["result"].(map[string]any)
	if channels == 0 || result["channelsInserted"].(float64) == 0 {
		t.Fatalf("task succeeded without rebuilding channels: %v", task)
	}
}

func TestTokenRoutes_RebuildAsyncFailureRemainsObservable(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	resetBackgroundTasksForTests()
	RegisterTasksRoutes(r, db.DB)
	t.Cleanup(func() { resetBackgroundTasksForTests(); SetBackgroundTaskDB(nil) })
	db.Close()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/routes/rebuild", strings.NewReader(`{"wait":false,"refreshModels":false}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d: %s", rec.Code, rec.Body.String())
	}
	body := readRebuildEnvelope(t, rec)
	task := awaitRebuildTask(t, r, body["taskId"].(string))
	if task["status"] != "failed" || task["error"] != "route rebuild failed" {
		t.Fatalf("failure hidden or raw internal error exposed: %v", task)
	}
}
