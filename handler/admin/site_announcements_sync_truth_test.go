package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestParseOptionalSiteAnnouncementSyncSiteID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantID  int64
		wantNil bool
		wantErr bool
	}{
		{name: "empty body means all sites", body: "", wantNil: true},
		{name: "empty object means all sites", body: `{}`, wantNil: true},
		{name: "null site rejected", body: `{"siteId":null}`, wantErr: true},
		{name: "positive integer", body: `{"siteId":42}`, wantID: 42},
		{name: "string rejected", body: `{"siteId":"42"}`, wantErr: true},
		{name: "fraction rejected", body: `{"siteId":1.5}`, wantErr: true},
		{name: "zero rejected", body: `{"siteId":0}`, wantErr: true},
		{name: "negative rejected", body: `{"siteId":-1}`, wantErr: true},
		{name: "array rejected", body: `[]`, wantErr: true},
		{name: "top-level null rejected", body: `null`, wantErr: true},
		{name: "unknown field rejected", body: `{"siteID":1}`, wantErr: true},
		{name: "trailing JSON rejected", body: `{"siteId":1} {}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/site-announcements/sync", strings.NewReader(tt.body))
			got, err := parseOptionalSiteAnnouncementSyncSiteID(req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got siteID=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("siteID=%d, want nil", *got)
				}
				return
			}
			if got == nil || *got != tt.wantID {
				t.Fatalf("siteID=%v, want %d", got, tt.wantID)
			}
		})
	}
}

func TestSiteAnnouncementSyncHandlerRejectsInvalidOrMissingSite(t *testing.T) {
	db, r := setupEventsAnnouncementsTest(t)

	invalidBodies := []string{
		`{`,
		`{"siteId":"1"}`,
		`{"siteId":1.5}`,
		`{"siteId":0}`,
		`{"siteId":-1}`,
		`{"siteId":null}`,
		`{"siteID":1}`,
		`[]`,
		`null`,
		`{"siteId":1} {}`,
	}
	for _, body := range invalidBodies {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/site-announcements/sync", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s, want 400", rec.Code, rec.Body.String())
			}
		})
	}

	missingID := int64(9_999_999)
	rec := doPostJSON(t, r, "/api/site-announcements/sync", map[string]any{"siteId": missingID})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing site status=%d body=%s, want 404", rec.Code, rec.Body.String())
	}

	var taskCount int
	if err := db.Get(&taskCount, `SELECT COUNT(*) FROM admin_background_tasks`); err != nil {
		t.Fatalf("count background tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("invalid requests queued %d background tasks, want 0", taskCount)
	}
}

func TestSyncSiteAnnouncements_SQLiteTruth(t *testing.T) {
	db, _ := setupEventsAnnouncementsTest(t)
	runSiteAnnouncementSyncTruth(t, db, "sqlite-truth")
}

func TestSyncSiteAnnouncements_PostgresTruth(t *testing.T) {
	db, _ := setupEventsAnnouncementsPostgresTest(t)
	runSiteAnnouncementSyncTruth(t, db, "pg-truth-"+strconv.FormatInt(time.Now().UnixNano(), 36))
}

func runSiteAnnouncementSyncTruth(t *testing.T, db *store.DB, suffix string) {
	t.Helper()
	server := newSiteAnnouncementNoticeServer(t, http.StatusOK, `{"success":true,"data":"Planned maintenance"}`)
	siteID := seedSiteAnnouncementSyncSite(t, db, suffix, server.URL)
	cleanupSiteAnnouncementSyncRows(t, db, siteID)

	result := SyncSiteAnnouncements(db.DB, &siteID)
	if result.ScannedSites != 1 || result.Inserted != 1 || result.Events != 1 || result.Failed != 0 {
		t.Fatalf("unexpected sync result: %+v", result)
	}

	var announcementID int64
	if err := db.Get(&announcementID, `SELECT id FROM site_announcements WHERE site_id = ?`, siteID); err != nil {
		t.Fatalf("load announcement id: %v", err)
	}
	var relatedID int64
	if err := db.Get(&relatedID, `SELECT related_id FROM events WHERE related_type = 'site_announcement' AND related_id = ?`, announcementID); err != nil {
		t.Fatalf("load related event: %v", err)
	}
	if relatedID != announcementID {
		t.Fatalf("event related_id=%d, want announcement id %d", relatedID, announcementID)
	}
}

func TestSyncSiteAnnouncements_RollsBackAnnouncementWhenEventInsertFails(t *testing.T) {
	db, _ := setupEventsAnnouncementsTest(t)
	server := newSiteAnnouncementNoticeServer(t, http.StatusOK, `{"success":true,"data":"Atomic notice"}`)
	siteID := seedSiteAnnouncementSyncSite(t, db, "event-failure", server.URL)
	cleanupSiteAnnouncementSyncRows(t, db, siteID)
	if _, err := db.Exec(`DROP TABLE events`); err != nil {
		t.Fatalf("drop events: %v", err)
	}

	result := SyncSiteAnnouncements(db.DB, &siteID)
	if result.Inserted != 0 || result.Events != 0 || result.Failed != 1 {
		t.Fatalf("unexpected failed sync result: %+v", result)
	}
	if len(result.FailedSites) != 1 || !strings.Contains(result.FailedSites[0].Message, "event:") {
		t.Fatalf("missing event-stage failure: %+v", result.FailedSites)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcements WHERE site_id = ?`, siteID); err != nil {
		t.Fatalf("count announcements: %v", err)
	}
	if count != 0 {
		t.Fatalf("announcement count=%d, want rollback to 0", count)
	}
}

func TestSyncSiteAnnouncements_UpdatesExistingWithoutDuplicateEvent(t *testing.T) {
	db, _ := setupEventsAnnouncementsTest(t)
	server := newSiteAnnouncementNoticeServer(t, http.StatusOK, `{"success":true,"data":"Stable notice"}`)
	siteID := seedSiteAnnouncementSyncSite(t, db, "update-existing", server.URL)
	cleanupSiteAnnouncementSyncRows(t, db, siteID)

	first := SyncSiteAnnouncements(db.DB, &siteID)
	second := SyncSiteAnnouncements(db.DB, &siteID)
	if first.Inserted != 1 || first.Events != 1 || first.Failed != 0 {
		t.Fatalf("initial sync failed: %+v", first)
	}
	if second.Updated != 1 || second.Inserted != 0 || second.Events != 0 || second.Failed != 0 {
		t.Fatalf("conflict update was not truthful: %+v", second)
	}
	var announcements int
	if err := db.Get(&announcements, `SELECT COUNT(*) FROM site_announcements WHERE site_id = ?`, siteID); err != nil {
		t.Fatalf("count announcements: %v", err)
	}
	var events int
	if err := db.Get(&events, `SELECT COUNT(*) FROM events WHERE related_type = 'site_announcement'`); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if announcements != 1 || events != 1 {
		t.Fatalf("announcements=%d events=%d, want 1/1", announcements, events)
	}
}

func TestSyncSiteAnnouncements_DoesNotCountFailedUpdate(t *testing.T) {
	db, _ := setupEventsAnnouncementsTest(t)
	server := newSiteAnnouncementNoticeServer(t, http.StatusOK, `{"success":true,"data":"Stable notice"}`)
	siteID := seedSiteAnnouncementSyncSite(t, db, "update-failure", server.URL)
	cleanupSiteAnnouncementSyncRows(t, db, siteID)

	first := SyncSiteAnnouncements(db.DB, &siteID)
	if first.Inserted != 1 || first.Failed != 0 {
		t.Fatalf("initial sync failed: %+v", first)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER fail_site_announcement_update
		BEFORE UPDATE ON site_announcements
		BEGIN
			SELECT RAISE(FAIL, 'forced update failure');
		END
	`); err != nil {
		t.Fatalf("create update failure trigger: %v", err)
	}

	second := SyncSiteAnnouncements(db.DB, &siteID)
	if second.Updated != 0 || second.Failed != 1 {
		t.Fatalf("failed update was counted as success: %+v", second)
	}
	if len(second.FailedSites) != 1 || !strings.Contains(second.FailedSites[0].Message, "update:") {
		t.Fatalf("missing update-stage failure: %+v", second.FailedSites)
	}
}

func TestSiteAnnouncementSyncTaskFailsWhenAllTargetedSitesFail(t *testing.T) {
	resetBackgroundTasksForTests()
	t.Cleanup(resetBackgroundTasksForTests)
	db, r := setupEventsAnnouncementsTest(t)
	SetBackgroundTaskDB(db.DB)
	t.Cleanup(func() { SetBackgroundTaskDB(nil) })
	server := newSiteAnnouncementNoticeServer(t, http.StatusOK, `{"success":false,"message":"upstream denied"}`)
	siteID := seedSiteAnnouncementSyncSite(t, db, "task-failure", server.URL)
	cleanupSiteAnnouncementSyncRows(t, db, siteID)

	rec := doPostJSON(t, r, "/api/site-announcements/sync", map[string]any{"siteId": siteID})
	if rec.Code != http.StatusOK {
		t.Fatalf("queue status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode queue response: %v", err)
	}
	waitAllBackgroundTasksForTests(5 * time.Second)
	task := getBackgroundTask(db.DB, payload.TaskID)
	if task == nil {
		t.Fatal("background task not found")
	}
	if task.Status != BackgroundTaskFailed || task.Error == nil {
		t.Fatalf("task status=%s error=%v, want failed", task.Status, task.Error)
	}
	result, ok := task.Result.(SiteAnnouncementSyncResult)
	if !ok {
		t.Fatalf("failed task result type=%T, want SiteAnnouncementSyncResult", task.Result)
	}
	if result.ScannedSites != 1 || result.Failed != 1 || len(result.FailedSites) != 1 {
		t.Fatalf("failed task lost structured result: %+v", result)
	}
}

func newSiteAnnouncementNoticeServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/notice" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func seedSiteAnnouncementSyncSite(t *testing.T, db *store.DB, suffix, baseURL string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	name := "Sync Site " + suffix
	url := strings.TrimRight(baseURL, "/")
	if db.Dialect == store.DialectPostgres {
		var id int64
		if err := db.QueryRowx(
			`INSERT INTO sites (name, url, platform, status, api_key, created_at, updated_at)
			 VALUES (?, ?, 'newapi', 'active', 'test-token', ?, ?) RETURNING id`,
			name, url, now, now,
		).Scan(&id); err != nil {
			t.Fatalf("insert postgres sync site: %v", err)
		}
		return id
	}
	res, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, api_key, created_at, updated_at)
		 VALUES (?, ?, 'newapi', 'active', 'test-token', ?, ?)`,
		name, url, now, now,
	)
	if err != nil {
		t.Fatalf("insert sqlite sync site: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("sync site id: %v", err)
	}
	return id
}

func cleanupSiteAnnouncementSyncRows(t *testing.T, db *store.DB, siteID int64) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM events WHERE related_type = 'site_announcement'`)
		_, _ = db.Exec(`DELETE FROM sites WHERE id = ?`, siteID)
	})
}

func TestSiteAnnouncementSyncTerminalError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		result SiteAnnouncementSyncResult
		want   bool
	}{
		{name: "success", result: SiteAnnouncementSyncResult{ScannedSites: 1, Inserted: 1}, want: false},
		{name: "fatal query", result: SiteAnnouncementSyncResult{Failed: 1}, want: true},
		{name: "all targets failed", result: SiteAnnouncementSyncResult{ScannedSites: 2, Failed: 2}, want: true},
		{name: "partial write survives", result: SiteAnnouncementSyncResult{ScannedSites: 1, Inserted: 1, Failed: 1}, want: false},
		{name: "unsupported is not failure", result: SiteAnnouncementSyncResult{ScannedSites: 1, Unsupported: 1}, want: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := siteAnnouncementSyncTerminalError(tt.result)
			if (got != nil) != tt.want {
				t.Fatalf("error=%v, wantError=%v for %+v", got, tt.want, tt.result)
			}
		})
	}
}

func TestSiteAnnouncementFailureDeduplicatesPerSite(t *testing.T) {
	t.Parallel()
	var result SiteAnnouncementSyncResult
	recordSiteAnnouncementFailure(&result, 7, "site", "insert", fmt.Errorf("first"))
	recordSiteAnnouncementFailure(&result, 7, "site", "event", fmt.Errorf("second"))
	if result.Failed != 1 || len(result.FailedSites) != 1 {
		t.Fatalf("failure was not deduplicated: %+v", result)
	}
	if !strings.Contains(result.FailedSites[0].Message, "insert: first") || !strings.Contains(result.FailedSites[0].Message, "event: second") {
		t.Fatalf("failure stages missing: %+v", result.FailedSites[0])
	}
}
