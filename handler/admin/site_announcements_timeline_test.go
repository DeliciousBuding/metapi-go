package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestSiteAnnouncementTimelineSQLite(t *testing.T) {
	db, router := setupEventsAnnouncementsTest(t)
	runSiteAnnouncementTimeline(t, db, router)
	runStableSiteAnnouncementTimeline(t, db)
}

func TestSiteAnnouncementTimelinePostgres(t *testing.T) {
	db, router := setupEventsAnnouncementsPostgresTest(t)
	runSiteAnnouncementTimeline(t, db, router)
	runStableSiteAnnouncementTimeline(t, db)
}

func runStableSiteAnnouncementTimeline(t *testing.T, db *store.DB) {
	t.Helper()
	var upstream atomic.Value
	upstream.Store(map[string]any{"code": 0, "data": []map[string]any{
		{"id": 1, "title": "Site notice", "content": "First announcement", "updated_at": "2026-10-01"},
		{"id": 2, "title": "Site notice", "content": "Separate announcement"},
	}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(upstream.Load()) }))
	t.Cleanup(server.Close)
	siteID := seedSiteAnnouncementSyncSite(t, db, "stable-timeline", server.URL)
	cleanupSiteAnnouncementSyncRows(t, db, siteID)
	if _, err := db.Exec(`UPDATE sites SET platform = 'sub2api' WHERE id = ?`, siteID); err != nil {
		t.Fatal(err)
	}
	if result := SyncSiteAnnouncements(db.DB, &siteID); result.Failed != 0 || result.Inserted != 2 {
		t.Fatalf("initial stable announcements: %+v", result)
	}
	if _, err := db.Exec(`UPDATE site_announcements SET read_at = '2026-10-01' WHERE site_id = ?`, siteID); err != nil {
		t.Fatal(err)
	}
	upstream.Store(map[string]any{"code": 0, "data": []map[string]any{
		{"id": 1, "title": "Site notice", "content": "First announcement", "updated_at": "2026-10-02"},
		{"id": 2, "title": "Site notice", "content": "Separate announcement"},
	}})
	if result := SyncSiteAnnouncements(db.DB, &siteID); result.Failed != 0 || result.Events != 0 {
		t.Fatalf("metadata-only update: %+v", result)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcement_history h JOIN site_announcements a ON a.id = h.announcement_id WHERE a.site_id = ?`, siteID); err != nil || count != 0 {
		t.Fatalf("metadata created versions: %d %v", count, err)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcements WHERE site_id = ? AND read_at IS NOT NULL`, siteID); err != nil || count != 2 {
		t.Fatalf("metadata reset read state: %d %v", count, err)
	}
	upstream.Store(map[string]any{"code": 0, "data": []map[string]any{
		{"id": 1, "title": "Site notice", "content": "Updated first announcement"},
		{"id": 2, "title": "Site notice", "content": "Separate announcement"},
	}})
	if result := SyncSiteAnnouncements(db.DB, &siteID); result.Failed != 0 || result.Events != 1 {
		t.Fatalf("stable content update: %+v", result)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcements WHERE site_id = ?`, siteID); err != nil || count != 2 {
		t.Fatalf("independent identities merged: %d %v", count, err)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcement_history h JOIN site_announcements a ON a.id = h.announcement_id WHERE a.site_id = ?`, siteID); err != nil || count != 1 {
		t.Fatalf("missing stable history: %d %v", count, err)
	}
}

func runSiteAnnouncementTimeline(t *testing.T, db *store.DB, router http.Handler) {
	t.Helper()
	var content atomic.Value
	content.Store("Maintenance at midnight")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": content.Load()})
	}))
	t.Cleanup(server.Close)
	siteID := seedSiteAnnouncementSyncSite(t, db, "timeline", server.URL)
	cleanupSiteAnnouncementSyncRows(t, db, siteID)
	var id int64
	for index, text := range []string{"Maintenance at midnight", "Maintenance at midnight", "Maintenance postponed", "Maintenance at midnight"} {
		content.Store(text)
		result := SyncSiteAnnouncements(db.DB, &siteID)
		if result.Failed != 0 {
			t.Fatalf("sync %d failed: %+v", index, result)
		}
		if index == 0 {
			if err := db.Get(&id, `SELECT id FROM site_announcements WHERE site_id = ?`, siteID); err != nil {
				t.Fatal(err)
			}
		}
		if index <= 1 {
			if _, err := db.Exec(`UPDATE site_announcements SET read_at = '2026-10-01' WHERE id = ?`, id); err != nil {
				t.Fatal(err)
			}
		} else {
			var readAt *string
			if err := db.Get(&readAt, `SELECT read_at FROM site_announcements WHERE id = ?`, id); err != nil || readAt != nil {
				t.Fatalf("changed content remains read: %v %v", readAt, err)
			}
		}
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcements WHERE site_id = ?`, siteID); err != nil || count != 1 {
		t.Fatalf("canonical count=%d: %v", count, err)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/site-announcements/%d/history", id), nil))
	var history []store.SiteAnnouncement
	if err := json.Unmarshal(recorder.Body.Bytes(), &history); err != nil || recorder.Code != 200 {
		t.Fatalf("history: %d %s %v", recorder.Code, recorder.Body, err)
	}
	if len(history) != 3 || history[0].Content != "Maintenance at midnight" || history[1].Content != "Maintenance postponed" || history[2].Content != "Maintenance at midnight" {
		t.Fatalf("history=%+v", history)
	}
	// The scheduler and manual sync may observe the same new content together.
	content.Store("Final schedule")
	var wait sync.WaitGroup
	results := make(chan SiteAnnouncementSyncResult, 2)
	for range 2 {
		wait.Add(1)
		go func() { defer wait.Done(); results <- SyncSiteAnnouncements(db.DB, &siteID) }()
	}
	wait.Wait()
	close(results)
	for result := range results {
		if result.Failed != 0 {
			t.Fatalf("concurrent sync failed: %+v", result)
		}
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcement_history WHERE announcement_id = ?`, id); err != nil || count != 3 {
		t.Fatalf("concurrent revisions=%d err=%v", count, err)
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/site-announcements?siteId=%d&read=false&limit=1", siteID), nil))
	var list []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0]["revisionCount"] != float64(3) {
		t.Fatalf("list=%s err=%v", recorder.Body, err)
	}
	if _, err := db.Exec(`DELETE FROM site_announcements WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcement_history WHERE announcement_id = ?`, id); err != nil || count != 0 {
		t.Fatalf("orphan history=%d err=%v", count, err)
	}
}

func TestSiteAnnouncementLegacyMergeSQLite(t *testing.T) {
	db, _ := setupEventsAnnouncementsTest(t)
	runSiteAnnouncementLegacyMerge(t, db)
}

func TestSiteAnnouncementLegacyMergePostgres(t *testing.T) {
	db, _ := setupEventsAnnouncementsPostgresTest(t)
	runSiteAnnouncementLegacyMerge(t, db)
}

func runSiteAnnouncementLegacyMerge(t *testing.T, db *store.DB) {
	t.Helper()
	siteID := seedSiteForAnnouncement(t, db, "legacy-timeline")
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM sites WHERE id = ?`, siteID) })
	var oldID, currentID int64
	for i, text := range []string{"Previous notice", "Latest notice"} {
		var id int64
		err := db.QueryRowx(`INSERT INTO site_announcements (site_id, platform, source_key, title, content, first_seen_at, last_seen_at, read_at) VALUES (?, 'newapi', ?, 'Site notice', ?, ?, ?, ?) RETURNING id`, siteID, "notice:"+strings.Repeat(fmt.Sprint(i+1), 40), text, fmt.Sprintf("2026-10-%02d", i+1), fmt.Sprintf("2026-10-%02d", i+1), nil).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			oldID = id
		} else {
			currentID = id
		}
	}
	// Same title with a stable upstream identity is a separate announcement.
	if _, err := db.Exec(`INSERT INTO site_announcements (site_id, platform, source_key, title, content, first_seen_at, last_seen_at) VALUES (?, 'sub2api', 'announcement:1', 'Site notice', 'Independent notice', '2026-10-03', '2026-10-03')`, siteID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO events (type, title, related_id, related_type) VALUES ('site_notice', 'Legacy event', ?, 'site_announcement')`, oldID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 'site_announcement_timeline_v1'`); err != nil {
		t.Fatal(err)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcements WHERE site_id = ?`, siteID); err != nil || count != 2 {
		t.Fatalf("merged count=%d err=%v", count, err)
	}
	var current store.SiteAnnouncement
	if err := db.Get(&current, `SELECT * FROM site_announcements WHERE id = ?`, currentID); err != nil || current.SourceKey != "notice" || current.Content != "Latest notice" {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcement_history WHERE announcement_id = ?`, currentID); err != nil || count != 1 {
		t.Fatalf("history count=%d err=%v", count, err)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM events WHERE related_type = 'site_announcement' AND related_id = ?`, oldID); err != nil || count != 0 {
		t.Fatalf("old event relation=%d err=%v", count, err)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM site_announcement_history WHERE announcement_id = ?`, currentID); err != nil || count != 1 {
		t.Fatalf("repeated upgrade duplicated history: %d %v", count, err)
	}
}
