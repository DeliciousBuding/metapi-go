package admin

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/store"
)

// wireSiteAnnouncementSchedulerSync injects admin SyncSiteAnnouncements into the
// scheduler package without creating an app→admin import cycle (admin already
// imports scheduler; app cannot import admin).
func wireSiteAnnouncementSchedulerSync() {
	scheduler.SetDefaultSiteAnnouncementSyncFunc(func(db *store.DB) scheduler.SiteAnnouncementSyncResult {
		result := SyncSiteAnnouncements(db, nil)
		return scheduler.SiteAnnouncementSyncResult{
			ScannedSites:  result.ScannedSites,
			Inserted:      result.Inserted,
			Updated:       result.Updated,
			Unsupported:   result.Unsupported,
			Notifications: result.Notifications,
			Events:        result.Events,
			Failed:        result.Failed,
		}
	})
}

// SiteAnnouncementSyncResult mirrors the TS syncSiteAnnouncements result shape.
// Used by the admin HTTP task path and by SiteAnnouncementScheduler via SyncSiteAnnouncements.
type SiteAnnouncementSyncResult struct {
	ScannedSites  int                          `json:"scannedSites"`
	Inserted      int                          `json:"inserted"`
	Updated       int                          `json:"updated"`
	Unsupported   int                          `json:"unsupported"`
	Notifications int                          `json:"notifications"`
	Events        int                          `json:"events"`
	Failed        int                          `json:"failed"`
	FailedSites   []SiteAnnouncementFailedSite `json:"failedSites"`
}

// SiteAnnouncementFailedSite is one site that failed during announcement sync.
type SiteAnnouncementFailedSite struct {
	SiteID   int64  `json:"siteId"`
	SiteName string `json:"siteName"`
	Message  string `json:"message"`
}

type announcementSyncSite struct {
	ID       int64   `db:"id"`
	Name     string  `db:"name"`
	URL      string  `db:"url"`
	Platform string  `db:"platform"`
	APIKey   *string `db:"api_key"`
}

// SyncSiteAnnouncements fetches announcements for active sites (or one site)
// via platform adapters and upserts site_announcements. Exported so the
// background SiteAnnouncementScheduler can reuse this path without HTTP.
func SyncSiteAnnouncements(db *store.DB, siteID *int64) SiteAnnouncementSyncResult {
	result := SiteAnnouncementSyncResult{
		FailedSites: []SiteAnnouncementFailedSite{},
	}

	var sites []announcementSyncSite
	var err error
	if siteID != nil && *siteID > 0 {
		err = db.Select(&sites, `SELECT id, name, url, platform, api_key FROM sites WHERE id = ?`, *siteID)
	} else {
		err = db.Select(&sites, `SELECT id, name, url, platform, api_key FROM sites WHERE status = 'active'`)
	}
	if err != nil {
		slog.Error("site-announcement sync: failed to query sites", "error", err)
		result.Failed++
		result.FailedSites = append(result.FailedSites, SiteAnnouncementFailedSite{
			SiteID:   0,
			SiteName: "",
			Message:  err.Error(),
		})
		return result
	}

	ctx := context.Background()
	for _, site := range sites {
		result.ScannedSites++
		adapter := platform.GetAdapter(site.Platform)
		if adapter == nil {
			result.Unsupported++
			continue
		}

		accessToken := strings.TrimSpace(stringPtrValue(site.APIKey))
		if accessToken == "" {
			accessToken = resolveSiteAccessToken(db, site.ID)
		}

		anns, annErr := adapter.GetSiteAnnouncements(ctx, site.URL, accessToken, nil, nil)
		if annErr != nil {
			result.Failed++
			result.FailedSites = append(result.FailedSites, SiteAnnouncementFailedSite{
				SiteID:   site.ID,
				SiteName: site.Name,
				Message:  annErr.Error(),
			})
			continue
		}

		seenAt := time.Now().UTC().Format(time.RFC3339)
		for _, announcement := range anns {
			persistSiteAnnouncement(db, site, announcement, seenAt, &result)
		}
	}

	return result
}

func recordSiteAnnouncementFailure(result *SiteAnnouncementSyncResult, siteID int64, siteName, stage string, err error) {
	message := stage + ": " + err.Error()
	slog.Error("site-announcement sync: write failed", "siteId", siteID, "siteName", siteName, "stage", stage, "error", err)
	for i := range result.FailedSites {
		if result.FailedSites[i].SiteID == siteID {
			result.FailedSites[i].Message += "; " + message
			return
		}
	}
	result.Failed++
	result.FailedSites = append(result.FailedSites, SiteAnnouncementFailedSite{
		SiteID:   siteID,
		SiteName: siteName,
		Message:  message,
	})
}

func resolveSiteAccessToken(db *store.DB, siteID int64) string {
	var token *string
	err := db.Get(&token, `
		SELECT access_token FROM accounts
		WHERE site_id = ? AND status = 'active'
		ORDER BY id ASC
		LIMIT 1
	`, siteID)
	if err != nil || token == nil {
		return ""
	}
	return strings.TrimSpace(*token)
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
