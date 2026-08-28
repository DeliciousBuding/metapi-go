package admin

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

// Stats marketplace / model-coverage builders extracted from stats.go.
// These aggregate model_availability + token_model_availability +
// token_routes + recent proxy_logs into the marketplace, token-candidate,
// without-token, missing-token-group, and endpoint-type model surfaces.
// Behavior-neutral extraction (same package, same exported surface).
//

// marketplaceAccount aggregates per-account marketplace detail for one model.
type marketplaceAccountAgg struct {
	ID       int64
	Site     string
	Username *string
	Latency  *float64
	Balance  float64
	Tokens   []map[string]any
	tokenIDs map[int64]struct{}
}

func (h *statsHandler) buildMarketplaceModels() ([]map[string]any, error) {
	// Collect model names from availability tables + exact token_routes patterns.
	type modelKey = string
	type modelAgg struct {
		accounts map[int64]*marketplaceAccountAgg
		latSum   float64
		latCount int
	}
	byModel := map[modelKey]*modelAgg{}

	ensure := func(model string) *modelAgg {
		if agg, ok := byModel[model]; ok {
			return agg
		}
		agg := &modelAgg{accounts: map[int64]*marketplaceAccountAgg{}}
		byModel[model] = agg
		return agg
	}
	ensureAccount := func(agg *modelAgg, accountID int64, site, username string, balance float64, latency *float64) *marketplaceAccountAgg {
		acc, ok := agg.accounts[accountID]
		if !ok {
			var userPtr *string
			if strings.TrimSpace(username) != "" {
				u := username
				userPtr = &u
			}
			acc = &marketplaceAccountAgg{
				ID:       accountID,
				Site:     site,
				Username: userPtr,
				Balance:  balance,
				Tokens:   []map[string]any{},
				tokenIDs: map[int64]struct{}{},
			}
			agg.accounts[accountID] = acc
		}
		if latency != nil {
			if acc.Latency == nil || *latency < *acc.Latency {
				v := *latency
				acc.Latency = &v
			}
			agg.latSum += *latency
			agg.latCount++
		}
		return acc
	}

	// Enabled, non-expired tokens for every account in ONE query (Wave 18
	// N+1 fix): the previous shape re-ran the per-account token query inside
	// the accountRows loop below — once per model×account availability row,
	// i.e. hundreds of identical-round-trip queries per marketplace render.
	// account_tokens is fleet-bounded (a handful of tokens per account), so a
	// single scan grouped in Go is safe on both dialects. Rows arrive ordered
	// (account_id, is_default DESC, id ASC), so each account's slice keeps
	// exactly the ordering the legacy per-account query produced.
	enabledTokensByAccount := make(map[int64][]map[string]any)
	enabledTokenRows, err := queryRowsErr(h.db, `
		SELECT id, account_id, name, is_default
		FROM account_tokens
		WHERE enabled = true
			AND (value_status IS NULL OR value_status <> 'expired')
		ORDER BY account_id ASC, is_default DESC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	for _, tr := range enabledTokenRows {
		tid := coerceInt64(tr["id"])
		if tid <= 0 {
			continue
		}
		accountID := coerceInt64(tr["accountId"])
		enabledTokensByAccount[accountID] = append(enabledTokensByAccount[accountID], map[string]any{
			"id":        tid,
			"name":      coerceString(tr["name"]),
			"isDefault": coerceInt(tr["isDefault"]) == 1 || coerceString(tr["isDefault"]) == "true" || coerceString(tr["isDefault"]) == "1",
		})
	}

	// Account-level availability.
	accountRows, err := queryRowsErr(h.db, `
		SELECT
			ma.model_name AS model_name,
			ma.latency_ms AS latency_ms,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			COALESCE(a.balance, 0) AS balance,
			COALESCE(s.name, '') AS site_name,
			COALESCE(s.platform, '') AS platform
		FROM model_availability ma
		INNER JOIN accounts a ON a.id = ma.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(ma.available, false) = true
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
		ORDER BY ma.model_name ASC, a.id ASC
	`)
	if err != nil {
		return nil, err
	}
	for _, row := range accountRows {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" {
			continue
		}
		agg := ensure(model)
		var latency *float64
		if row["latencyMs"] != nil && coerceString(row["latencyMs"]) != "" {
			v := coerceFloat(row["latencyMs"])
			latency = &v
		}
		acc := ensureAccount(agg,
			coerceInt64(row["accountId"]),
			coerceString(row["siteName"]),
			coerceString(row["username"]),
			coerceFloat(row["balance"]),
			latency,
		)
		// Attach enabled tokens for this account (may also appear via token
		// availability) from the single pre-loaded batch above.
		for _, token := range enabledTokensByAccount[acc.ID] {
			tid := coerceInt64(token["id"])
			if _, exists := acc.tokenIDs[tid]; exists {
				continue
			}
			acc.tokenIDs[tid] = struct{}{}
			acc.Tokens = append(acc.Tokens, token)
		}
	}

	// Token-level availability.
	tokenAvailRows, err := queryRowsErr(h.db, `
		SELECT
			tma.model_name AS model_name,
			tma.latency_ms AS latency_ms,
			at.id AS token_id,
			COALESCE(at.name, '') AS token_name,
			at.is_default AS is_default,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			COALESCE(a.balance, 0) AS balance,
			COALESCE(s.name, '') AS site_name
		FROM token_model_availability tma
		INNER JOIN account_tokens at ON at.id = tma.token_id
		INNER JOIN accounts a ON a.id = at.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(tma.available, false) = true
			AND at.enabled = true
			AND (at.value_status IS NULL OR at.value_status <> 'expired')
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
		ORDER BY tma.model_name ASC, a.id ASC, at.id ASC
	`)
	if err != nil {
		return nil, err
	}
	for _, row := range tokenAvailRows {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" {
			continue
		}
		agg := ensure(model)
		var latency *float64
		if row["latencyMs"] != nil && coerceString(row["latencyMs"]) != "" {
			v := coerceFloat(row["latencyMs"])
			latency = &v
		}
		acc := ensureAccount(agg,
			coerceInt64(row["accountId"]),
			coerceString(row["siteName"]),
			coerceString(row["username"]),
			coerceFloat(row["balance"]),
			latency,
		)
		tid := coerceInt64(row["tokenId"])
		if tid <= 0 {
			continue
		}
		if _, exists := acc.tokenIDs[tid]; exists {
			continue
		}
		acc.tokenIDs[tid] = struct{}{}
		acc.Tokens = append(acc.Tokens, map[string]any{
			"id":        tid,
			"name":      coerceString(row["tokenName"]),
			"isDefault": coerceInt(row["isDefault"]) == 1 || coerceString(row["isDefault"]) == "true" || coerceString(row["isDefault"]) == "1",
		})
	}

	// Exact-model token_routes contribute model names even when availability is empty
	// so operators still see configured routes in the marketplace list.
	routeRows, err := queryRowsErr(h.db, `
		SELECT model_pattern
		FROM token_routes
		WHERE enabled = true
			AND route_mode <> 'explicit_group'
	`)
	if err != nil {
		return nil, err
	}
	for _, row := range routeRows {
		pattern := strings.TrimSpace(coerceString(row["modelPattern"]))
		if pattern == "" {
			continue
		}
		// Only exact patterns (no wildcards / regex markers) become marketplace models.
		if strings.ContainsAny(pattern, "*?^$[]()|\\+") {
			continue
		}
		if _, ok := byModel[pattern]; !ok {
			byModel[pattern] = &modelAgg{accounts: map[int64]*marketplaceAccountAgg{}}
		}
	}

	// Optional success-rate from recent proxy_logs (bounded 7d window).
	since := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	successByModel := map[string]struct {
		total   int
		success int
	}{}
	logRows, err := queryRowsErr(h.db, `
		SELECT
			COALESCE(NULLIF(TRIM(model_actual), ''), NULLIF(TRIM(model_requested), ''), '') AS model,
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) AS success
		FROM proxy_logs
		WHERE created_at >= ?
			AND COALESCE(NULLIF(TRIM(model_actual), ''), NULLIF(TRIM(model_requested), ''), '') <> ''
		GROUP BY COALESCE(NULLIF(TRIM(model_actual), ''), NULLIF(TRIM(model_requested), ''), '')
	`, since)
	if err != nil {
		return nil, err
	}
	for _, row := range logRows {
		model := strings.TrimSpace(coerceString(row["model"]))
		if model == "" {
			continue
		}
		successByModel[model] = struct {
			total   int
			success int
		}{total: coerceInt(row["total"]), success: coerceInt(row["success"])}
	}

	names := make([]string, 0, len(byModel))
	for name := range byModel {
		names = append(names, name)
	}
	sort.Strings(names)

	// Hydrate the merged model-catalog snapshot (nil when the catalog is
	// disabled) so description/tags/pricing stop being hardcoded empties.
	catalogSnapshot := appCatalogSnapshot()

	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		agg := byModel[name]
		accounts := make([]map[string]any, 0, len(agg.accounts))
		accountIDs := make([]int64, 0, len(agg.accounts))
		for id := range agg.accounts {
			accountIDs = append(accountIDs, id)
		}
		sort.Slice(accountIDs, func(i, j int) bool { return accountIDs[i] < accountIDs[j] })
		tokenCount := 0
		for _, id := range accountIDs {
			acc := agg.accounts[id]
			tokenCount += len(acc.Tokens)
			var latency any
			if acc.Latency != nil {
				latency = int64(math.Round(*acc.Latency))
			} else {
				latency = nil
			}
			accounts = append(accounts, map[string]any{
				"id":       acc.ID,
				"site":     acc.Site,
				"username": acc.Username,
				"latency":  latency,
				"balance":  roundMicro(acc.Balance),
				"tokens":   acc.Tokens,
			})
		}
		var avgLatency any
		if agg.latCount > 0 {
			avgLatency = int64(math.Round(agg.latSum / float64(agg.latCount)))
		} else {
			avgLatency = nil
		}
		var successRate any
		if stats, ok := successByModel[name]; ok && stats.total > 0 {
			successRate = math.Round(1000.0*float64(stats.success)/float64(stats.total)) / 10.0
		} else {
			successRate = nil
		}
		entry, catalogHit := catalogLookup(catalogSnapshot, name)
		out = append(out, map[string]any{
			"name":          name,
			"accountCount":  len(accounts),
			"tokenCount":    tokenCount,
			"avgLatency":    avgLatency,
			"successRate":   successRate,
			"description":   catalogDescription(entry, catalogHit),
			"tags":          catalogTags(entry, catalogHit),
			"deprecated":    catalogHit && entry.Status == "deprecated",
			"releaseDate":   catalogReleaseDate(entry, catalogHit),
			"contextWindow": entry.ContextLimit,
			"catalogStatus": catalogStatusOf(catalogHit),
			// supportedEndpointTypes: catalog-driven (provider dialect + modality
			// surfaces) when the model is cataloged; name-prefix heuristic only as a
			// catalog-miss fallback.
			"supportedEndpointTypes": inferEndpointTypesForModel(catalogSnapshot, name, accounts),
			// Official catalog list price as a synthetic pricing source; the
			// frontend labels it "catalog estimate" because marketplace sites
			// are third-party relays (siteName "catalog" is the sentinel).
			"pricingSources": catalogPricingSources(entry, catalogHit),
			"accounts":       accounts,
		})
	}
	return out, nil
}

// appCatalogSnapshot returns the merged model-catalog snapshot, or nil when
// the catalog manager is unavailable (PRICING_CATALOG_ENABLED=false).
func appCatalogSnapshot() *pricingcatalog.CatalogSnapshot {
	manager := app.CatalogManager()
	if manager == nil {
		return nil
	}
	return manager.Snapshot()
}

func catalogLookup(snapshot *pricingcatalog.CatalogSnapshot, name string) (pricingcatalog.CatalogEntry, bool) {
	if snapshot == nil {
		return pricingcatalog.CatalogEntry{}, false
	}
	return snapshot.Lookup(name)
}

func catalogDescription(entry pricingcatalog.CatalogEntry, hit bool) any {
	if !hit || strings.TrimSpace(entry.Description) == "" {
		return nil
	}
	return entry.Description
}

func catalogTags(entry pricingcatalog.CatalogEntry, hit bool) []string {
	if !hit || len(entry.Tags) == 0 {
		return []string{}
	}
	return entry.Tags
}

func catalogReleaseDate(entry pricingcatalog.CatalogEntry, hit bool) any {
	if !hit || strings.TrimSpace(entry.ReleaseDate) == "" {
		return nil
	}
	return entry.ReleaseDate
}

func catalogStatusOf(hit bool) string {
	if hit {
		return "hydrated"
	}
	return "unknown"
}

// catalogPricingSources builds the synthetic catalog list-price source:
// {siteId: 0, siteName: "catalog", groupPricing: {"catalog": {input/output}}}.
// Uses the explicit USD list price when present; otherwise derives the
// per-million rates from the NewAPI ratio set (ratio × $2 base). Only emitted
// when a usable input rate exists so a partial price never misleads.
func catalogPricingSources(entry pricingcatalog.CatalogEntry, hit bool) []any {
	if !hit {
		return []any{}
	}
	input, output, ok := entry.ReferencePerMillionRates()
	if !ok {
		return []any{}
	}
	return []any{map[string]any{
		"siteId":       0,
		"siteName":     "catalog",
		"accountId":    0,
		"username":     nil,
		"ownerBy":      nil,
		"enableGroups": []string{},
		"groupPricing": map[string]any{
			"catalog": map[string]any{
				"quotaType":        0,
				"inputPerMillion":  input,
				"outputPerMillion": output,
			},
		},
	}}
}

// inferEndpointTypesForModel derives the model's routeable endpoint types:
// the API dialect (anthropic / gemini / openai) and the non-chat endpoint
// families (images, audio, videos, embeddings, rerank). When the model is
// cataloged, recognizable big-three model families keep their native dialect;
// other models use the catalog provider/default dialect. Non-chat families
// come from the catalog modalities
// (vision/audio/video output). The name-suffix heuristic is retained for
// embeddings/rerank (no modality signal exists for them) and as the
// catalog-miss fallback.
func inferEndpointTypesForModel(snapshot *pricingcatalog.CatalogSnapshot, modelName string, accounts []map[string]any) []string {
	if entry, ok := catalogLookup(snapshot, modelName); ok {
		return catalogEndpointTypes(entry, modelName)
	}
	return endpointTypesByName(modelName)
}

// catalogEndpointTypes builds the endpoint type set from catalog metadata.
// The dialect first recognizes native big-three model families because catalog
// provider slugs can name a reseller rather than the model protocol. Otherwise
// it maps anthropic/google providers and defaults to OpenAI-compatible. Non-chat
// families come from
// the directed modality lists: an image output means image generation (with
// the edit surface when the model also takes image input), an audio output
// means speech, an audio input means transcription, a video output means
// video creation. A vision chat model (image in, text out) stays on the
// chat surface. Embeddings and rerank have no modality signal, so their
// families still come from the model name.
func catalogEndpointTypes(entry pricingcatalog.CatalogEntry, modelName string) []string {
	var types []string
	if dialect := endpointDialectByName(modelName); dialect != "" {
		// models.dev provider slugs identify the listing/vendor, not always
		// the model protocol (for example, a Claude row can be listed under
		// a relay provider). Keep recognizable big-three model families on
		// their native API surface.
		types = append(types, dialect)
	} else {
		switch strings.ToLower(strings.TrimSpace(entry.Provider)) {
		case "anthropic":
			types = append(types, "anthropic")
		case "google":
			types = append(types, "gemini")
		default:
			// Catalog-only providers (deepseek, meta, mistral, x-ai,
			// relay vendors, and ratio-only rows) use the
			// OpenAI-compatible surface.
			types = append(types, "openai")
		}
	}

	input := modalitySet(entry.ModalitiesInput)
	output := modalitySet(entry.ModalitiesOutput)
	if output["image"] {
		types = append(types, "images.generations")
		if input["image"] {
			// Generation models that accept an input image also serve the
			// edit surface; a vision-only chat model does not.
			types = append(types, "images.edits")
		}
	}
	if output["audio"] {
		types = append(types, "audio.speech")
	}
	if input["audio"] {
		types = append(types, "audio.transcriptions")
	}
	if output["video"] {
		types = append(types, "videos.create")
	}
	return appendEndpointFamilies(types, modelName)
}

// modalitySet is the ordered membership check helper for modality lists.
func modalitySet(modalities []string) map[string]bool {
	set := make(map[string]bool, len(modalities))
	for _, modality := range modalities {
		set[strings.ToLower(strings.TrimSpace(modality))] = true
	}
	return set
}

// endpointTypesByName is the catalog-miss fallback: name-prefix heuristics
// for the big-three dialects plus name-suffix families that no field can
// express (embedding/rerank models are text-in/text-out like chat models).
func endpointTypesByName(modelName string) []string {
	return appendEndpointFamilies(nil, modelName)
}

// appendEndpointFamilies adds the name-derived endpoint families
// (embeddings / rerank) and the big-three dialect when no dialect value was
// supplied yet. Values are deduped and order-stable.
func appendEndpointFamilies(types []string, modelName string) []string {
	lower := strings.ToLower(modelName)
	if len(types) == 0 {
		if dialect := endpointDialectByName(modelName); dialect != "" {
			types = append(types, dialect)
		}
	}
	if containsAnyFold(lower, "embedding", "-embed", "bge-", "e5-") {
		types = append(types, "embeddings")
	}
	if containsAnyFold(lower, "rerank") {
		types = append(types, "rerank")
	}
	return dedupeStrings(types)
}

func endpointDialectByName(modelName string) string {
	lower := strings.ToLower(modelName)
	switch {
	case strings.Contains(lower, "claude"), strings.HasPrefix(lower, "anthropic"):
		return "anthropic"
	case strings.Contains(lower, "gemini"):
		return "gemini"
	case strings.Contains(lower, "gpt"), strings.Contains(lower, "o1"), strings.Contains(lower, "o3"), strings.Contains(lower, "o4"):
		return "openai"
	default:
		return ""
	}
}

func containsAnyFold(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, dup := seen[value]; dup {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (h *statsHandler) buildTokenCandidateModels(allowed map[string]struct{}) (map[string][]map[string]any, error) {
	rows, err := queryRowsErr(h.db, `
		SELECT
			tma.model_name AS model_name,
			a.id AS account_id,
			at.id AS token_id,
			COALESCE(at.name, '') AS token_name,
			at.is_default AS is_default,
			COALESCE(a.username, '') AS username,
			s.id AS site_id,
			COALESCE(s.name, '') AS site_name
		FROM token_model_availability tma
		INNER JOIN account_tokens at ON at.id = tma.token_id
		INNER JOIN accounts a ON a.id = at.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(tma.available, false) = true
			AND at.enabled = true
			AND (at.value_status IS NULL OR at.value_status <> 'expired')
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
		ORDER BY tma.model_name ASC, a.id ASC, at.id ASC
	`)
	if err != nil {
		return nil, err
	}

	out := map[string][]map[string]any{}
	seen := map[string]map[int64]struct{}{} // model -> tokenIDs
	for _, row := range rows {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" || !modelAllowed(model, allowed) {
			continue
		}
		tokenID := coerceInt64(row["tokenId"])
		if tokenID <= 0 {
			continue
		}
		if _, ok := seen[model]; !ok {
			seen[model] = map[int64]struct{}{}
		}
		if _, dup := seen[model][tokenID]; dup {
			continue
		}
		seen[model][tokenID] = struct{}{}
		var username any
		if u := strings.TrimSpace(coerceString(row["username"])); u != "" {
			username = u
		} else {
			username = nil
		}
		out[model] = append(out[model], map[string]any{
			"accountId": coerceInt64(row["accountId"]),
			"tokenId":   tokenID,
			"tokenName": coerceString(row["tokenName"]),
			"isDefault": coerceInt(row["isDefault"]) == 1 || coerceString(row["isDefault"]) == "true" || coerceString(row["isDefault"]) == "1",
			"username":  username,
			"siteId":    coerceInt64(row["siteId"]),
			"siteName":  coerceString(row["siteName"]),
		})
	}
	return out, nil
}

func (h *statsHandler) buildModelsWithoutToken(allowed map[string]struct{}) (map[string][]map[string]any, error) {
	// Accounts with available model_availability but no token_model_availability
	// coverage for that model AND no enabled account_tokens at all (or none that
	// list the model). Operators use this for zero-channel route hints.
	rows, err := queryRowsErr(h.db, `
		SELECT
			ma.model_name AS model_name,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			s.id AS site_id,
			COALESCE(s.name, '') AS site_name
		FROM model_availability ma
		INNER JOIN accounts a ON a.id = ma.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE COALESCE(ma.available, false) = true
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
			AND NOT EXISTS (
				SELECT 1
				FROM account_tokens at
				INNER JOIN token_model_availability tma ON tma.token_id = at.id
				WHERE at.account_id = a.id
					AND at.enabled = true
					AND (at.value_status IS NULL OR at.value_status <> 'expired')
					AND COALESCE(tma.available, false) = true
					AND tma.model_name = ma.model_name
			)
			AND NOT EXISTS (
				-- API-key style accounts that store a single key without managed tokens
				-- are still "without token" for route channel binding when no account_tokens rows exist.
				SELECT 1 FROM account_tokens at
				WHERE at.account_id = a.id
					AND at.enabled = true
					AND (at.value_status IS NULL OR at.value_status <> 'expired')
			)
		ORDER BY ma.model_name ASC, a.id ASC
	`)
	if err != nil {
		return nil, err
	}

	out := map[string][]map[string]any{}
	seen := map[string]map[int64]struct{}{}
	for _, row := range rows {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" || !modelAllowed(model, allowed) {
			continue
		}
		accountID := coerceInt64(row["accountId"])
		if accountID <= 0 {
			continue
		}
		if _, ok := seen[model]; !ok {
			seen[model] = map[int64]struct{}{}
		}
		if _, dup := seen[model][accountID]; dup {
			continue
		}
		seen[model][accountID] = struct{}{}
		var username any
		if u := strings.TrimSpace(coerceString(row["username"])); u != "" {
			username = u
		} else {
			username = nil
		}
		out[model] = append(out[model], map[string]any{
			"accountId": accountID,
			"username":  username,
			"siteId":    coerceInt64(row["siteId"]),
			"siteName":  coerceString(row["siteName"]),
		})
	}
	return out, nil
}

func (h *statsHandler) buildModelsMissingTokenGroups(allowed map[string]struct{}) (map[string][]map[string]any, error) {
	// When an account has model availability and managed tokens, but none of the
	// enabled tokens have a resolvable token_group label, group coverage is uncertain
	// / missing. We do not invent required groups from a remote pricing catalog.
	rows, err := queryRowsErr(h.db, `
		SELECT
			ma.model_name AS model_name,
			a.id AS account_id,
			COALESCE(a.username, '') AS username,
			s.id AS site_id,
			COALESCE(s.name, '') AS site_name,
			COALESCE(at.token_group, '') AS token_group,
			COALESCE(at.name, '') AS token_name
		FROM model_availability ma
		INNER JOIN accounts a ON a.id = ma.account_id
		INNER JOIN sites s ON s.id = a.site_id
		INNER JOIN account_tokens at ON at.account_id = a.id
		WHERE COALESCE(ma.available, false) = true
			AND COALESCE(a.status, '') <> 'disabled'
			AND COALESCE(s.status, '') <> 'disabled'
			AND at.enabled = true
			AND (at.value_status IS NULL OR at.value_status <> 'expired')
		ORDER BY ma.model_name ASC, a.id ASC, at.id ASC
	`)
	if err != nil {
		return nil, err
	}

	type accGroups struct {
		accountID int64
		username  string
		siteID    int64
		siteName  string
		available []string
		uncertain bool
	}
	// model -> accountID -> groups
	byModel := map[string]map[int64]*accGroups{}
	for _, row := range rows {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" || !modelAllowed(model, allowed) {
			continue
		}
		accountID := coerceInt64(row["accountId"])
		if accountID <= 0 {
			continue
		}
		if _, ok := byModel[model]; !ok {
			byModel[model] = map[int64]*accGroups{}
		}
		ag, ok := byModel[model][accountID]
		if !ok {
			ag = &accGroups{
				accountID: accountID,
				username:  coerceString(row["username"]),
				siteID:    coerceInt64(row["siteId"]),
				siteName:  coerceString(row["siteName"]),
			}
			byModel[model][accountID] = ag
		}
		group := resolveTokenGroupLabel(coerceString(row["tokenGroup"]), coerceString(row["tokenName"]))
		if group == "" {
			ag.uncertain = true
			continue
		}
		// de-dupe
		found := false
		for _, g := range ag.available {
			if strings.EqualFold(g, group) {
				found = true
				break
			}
		}
		if !found {
			ag.available = append(ag.available, group)
		}
	}

	out := map[string][]map[string]any{}
	for model, accounts := range byModel {
		for _, ag := range accounts {
			// Only surface accounts where no token has a resolvable group label.
			if len(ag.available) > 0 {
				continue
			}
			var username any
			if strings.TrimSpace(ag.username) != "" {
				username = ag.username
			} else {
				username = nil
			}
			item := map[string]any{
				"accountId":              ag.accountID,
				"username":               username,
				"siteId":                 ag.siteID,
				"siteName":               ag.siteName,
				"missingGroups":          []string{},
				"requiredGroups":         []string{},
				"availableGroups":        []string{},
				"groupCoverageUncertain": true,
			}
			out[model] = append(out[model], item)
		}
	}
	return out, nil
}

func resolveTokenGroupLabel(tokenGroup, tokenName string) string {
	group := strings.TrimSpace(tokenGroup)
	if group != "" {
		return group
	}
	name := strings.TrimSpace(tokenName)
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	if lower == "default" || name == "默认" {
		return ""
	}
	// Names like token-1 / token-N are not group labels.
	if strings.HasPrefix(lower, "token-") {
		return ""
	}
	return name
}

func (h *statsHandler) buildEndpointTypesByModel(allowed map[string]struct{}) (map[string][]string, error) {
	// Union of endpoint types inferred from model names present in availability.
	models := map[string]struct{}{}
	availModels, err := queryRowsErr(h.db, `
		SELECT DISTINCT model_name AS model_name FROM model_availability WHERE COALESCE(available, false) = true
		UNION
		SELECT DISTINCT model_name AS model_name FROM token_model_availability WHERE COALESCE(available, false) = true
	`)
	if err != nil {
		return nil, err
	}
	for _, row := range availModels {
		model := strings.TrimSpace(coerceString(row["modelName"]))
		if model == "" || !modelAllowed(model, allowed) {
			continue
		}
		models[model] = struct{}{}
	}
	out := map[string][]string{}
	catalogSnapshot := appCatalogSnapshot()
	for model := range models {
		types := inferEndpointTypesForModel(catalogSnapshot, model, nil)
		if len(types) == 0 {
			// default OpenAI-compatible when unknown
			types = []string{"openai"}
		}
		out[model] = types
	}
	return out, nil
}

func (h *statsHandler) loadGlobalAllowedModels() map[string]struct{} {
	// Optional whitelist from settings table. Empty / missing → allow all.
	var raw string
	err := h.db.Get(&raw, rebindAdminQuery(h.db, `SELECT value FROM settings WHERE key = ?`), "global_allowed_models")
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		// also accept CSV
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				list = append(list, part)
			}
		}
	}
	if len(list) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(list))
	for _, m := range list {
		m = strings.TrimSpace(m)
		if m != "" {
			out[strings.ToLower(m)] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
