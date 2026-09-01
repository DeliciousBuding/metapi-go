package events

// Checkin event definitions — batch 1 of the structured-event migration
// (F5). The TitleEn / MessageEn / Type values replicate the historical
// producer output byte-for-byte so migrated rows are identical for legacy
// consumers (notifications, CSV export, history fallback).
func init() {
	register(Definition{
		Key:     "checkinSuccess",
		Type:    "checkin",
		TitleEn: "checkin success",
		Params: []ParamSpec{
			{Name: "account", Kind: ParamString, Required: true},
			{Name: "site", Kind: ParamString, Required: true},
			{Name: "reward", Kind: ParamString},
		},
		MessageEn: "{{account}} @ {{site}}: {{reward}}",
	})
	register(Definition{
		Key:     "checkinFailed",
		Type:    "checkin",
		TitleEn: "checkin failed",
		Params: []ParamSpec{
			{Name: "account", Kind: ParamString, Required: true},
			{Name: "site", Kind: ParamString, Required: true},
			{Name: "reason", Kind: ParamString, Required: true},
		},
		MessageEn: "{{account}} @ {{site}}: {{reason}}",
	})
	register(Definition{
		Key:     "checkinSkipped",
		Type:    "checkin",
		TitleEn: "checkin skipped",
		Params: []ParamSpec{
			{Name: "account", Kind: ParamString, Required: true},
			{Name: "site", Kind: ParamString, Required: true},
			{Name: "reason", Kind: ParamString, Required: true},
		},
		MessageEn: "{{account}} @ {{site}}: {{reason}}",
	})
	register(Definition{
		Key:     "checkinFailedCloudflare",
		Type:    "checkin",
		TitleEn: "checkin failed (cloudflare challenge)",
		Params: []ParamSpec{
			{Name: "account", Kind: ParamString, Required: true},
			{Name: "site", Kind: ParamString, Required: true},
			{Name: "reason", Kind: ParamString, Required: true},
		},
		MessageEn: "{{account}} @ {{site}}: {{reason}}",
	})
}
