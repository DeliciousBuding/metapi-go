package admin

import (
	"encoding/json"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- H1: product risk banners ----

func setupAnnouncementsTest(t *testing.T) (*store.DB, chi.Router) {
	t.Helper()
	db, _ := setupStatsSQLiteTest(t)
	r := chi.NewRouter()
	RegisterAnnouncementsRoutes(r, db.DB)
	return db, r
}

func TestAnnouncements_CRUDAndActiveView(t *testing.T) {
	_, r := setupAnnouncementsTest(t)

	// Create two announcements (one critical, one info).
	var body map[string]any
	resp := doPostJSON(t, r, "/api/announcements", map[string]any{
		"title":    "上游故障",
		"message":  "Claude 上游 API 故障中，部分模型可能超时",
		"severity": "critical",
		"link":     "https://status.example.test",
	})
	if resp.Code != 200 {
		t.Fatalf("create returned %d: %s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	items := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %#v, want 1", items)
	}
	first := items[0].(map[string]any)
	if first["severity"] != "critical" || first["dismissed"] != false {
		t.Fatalf("first = %#v, want critical + not dismissed", first)
	}

	resp = doPostJSON(t, r, "/api/announcements", map[string]any{
		"title":    "新功能",
		"message":  "已支持批量模型验证",
		"severity": "info",
	})
	if resp.Code != 200 {
		t.Fatalf("create info returned %d", resp.Code)
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal info: %v", err)
	}
	items = body["items"].([]any)
	announcementID := items[0].(map[string]any)["id"].(float64)
	infoID := int64(announcementID)
	for _, it := range items {
		if it.(map[string]any)["severity"] == "info" {
			infoID = int64(it.(map[string]any)["id"].(float64))
		}
	}

	// Active view shows both (nothing dismissed), critical first.
	resp = doGet(t, r, "/api/announcements/active")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal active: %v", err)
	}
	active := body["items"].([]any)
	if len(active) != 2 {
		t.Fatalf("active = %#v, want 2", active)
	}
	if active[0].(map[string]any)["severity"] != "critical" {
		t.Fatalf("active[0] severity = %v, want critical first", active[0].(map[string]any)["severity"])
	}

	// Dismiss the info one; active view now shows only critical.
	resp = doPostJSON(t, r, "/api/announcements/"+itoa(infoID)+"/dismiss", map[string]any{})
	if resp.Code != 200 {
		t.Fatalf("dismiss returned %d", resp.Code)
	}
	resp = doGet(t, r, "/api/announcements/active")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal active after dismiss: %v", err)
	}
	active = body["items"].([]any)
	if len(active) != 1 || active[0].(map[string]any)["severity"] != "critical" {
		t.Fatalf("active after dismiss = %#v, want 1 critical", active)
	}

	// Content edit resets the dismissal (new revision surfaces again).
	resp = doPutJSON(t, r, "/api/announcements/"+itoa(infoID), map[string]any{
		"title":    "新功能 v2",
		"message":  "已支持批量模型验证和标签系统",
		"severity": "info",
	})
	if resp.Code != 200 {
		t.Fatalf("update returned %d: %s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}
	if body["revision"] != true {
		t.Fatalf("revision = %v, want true (content changed)", body["revision"])
	}
	resp = doGet(t, r, "/api/announcements/active")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal active after edit: %v", err)
	}
	if len(body["items"].([]any)) != 2 {
		t.Fatalf("active after edit = %#v, want 2 (dismissal reset)", body["items"])
	}

	// Delete the critical one; only info remains in admin view.
	var critID int64
	resp = doGet(t, r, "/api/announcements")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	for _, it := range body["items"].([]any) {
		m := it.(map[string]any)
		if m["severity"] == "critical" {
			critID = int64(m["id"].(float64))
		}
	}
	resp = doDelete(t, r, "/api/announcements/"+itoa(critID))
	if resp.Code != 200 {
		t.Fatalf("delete returned %d", resp.Code)
	}
	resp = doGet(t, r, "/api/announcements")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal after delete: %v", err)
	}
	if len(body["items"].([]any)) != 1 {
		t.Fatalf("after delete = %#v, want 1", body["items"])
	}
}

func TestAnnouncements_Validation(t *testing.T) {
	_, r := setupAnnouncementsTest(t)

	// Missing title/message.
	resp := doPostJSON(t, r, "/api/announcements", map[string]any{"title": "", "message": "x", "severity": "info"})
	if resp.Code != 400 {
		t.Fatalf("empty title returned %d, want 400", resp.Code)
	}

	// Bad severity.
	resp = doPostJSON(t, r, "/api/announcements", map[string]any{"title": "t", "message": "m", "severity": "extreme"})
	if resp.Code != 400 {
		t.Fatalf("bad severity returned %d, want 400", resp.Code)
	}

	// Unknown id.
	resp = doPutJSON(t, r, "/api/announcements/999999", map[string]any{"title": "t", "message": "m"})
	if resp.Code != 404 {
		t.Fatalf("unknown update returned %d, want 404", resp.Code)
	}
	resp = doPostJSON(t, r, "/api/announcements/999999/dismiss", map[string]any{})
	if resp.Code != 404 {
		t.Fatalf("unknown dismiss returned %d, want 404", resp.Code)
	}
	resp = doDelete(t, r, "/api/announcements/999999")
	if resp.Code != 404 {
		t.Fatalf("unknown delete returned %d, want 404", resp.Code)
	}
}
