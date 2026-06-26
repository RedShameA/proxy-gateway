package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"net/http"
	"regexp"
	"strings"

	applicationprofiles "proxygateway/internal/application/profiles"
	domainprofile "proxygateway/internal/domain/profile"
)

func (g *Gateway) handleAccessProfiles(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req accessProfilePatchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		id, err := prefixedID("profile")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create profile id")
			return
		}
		cfg := defaultAccessProfileConfig(id)
		applyAccessProfilePatch(&cfg, req)
		if err := g.validateAccessProfileConfig(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := g.insertAccessProfileConfig(cfg); err != nil {
			writeError(w, http.StatusInternalServerError, "create access profile")
			return
		}
		if cfg.AutoEvalEnabled && profileTypeNeedsEvaluation(cfg.Type) {
			_, _ = g.enqueueProfileEvaluationRun(id, cfg.Name, "access_profile_change", 1, true)
		}
		if len(cfg.EgressCountries) > 0 {
			g.enqueueUnknownCountryObservations(cfg.candidateFilter())
		}
		writeJSON(w, http.StatusOK, g.accessProfileSummary(cfg))
	case http.MethodGet:
		page, pageSize := parsePagination(r)
		offset := (page - 1) * pageSize

		var total int
		_ = g.db.QueryRow(`SELECT COUNT(*) FROM access_profiles`).Scan(&total)

		rows, err := g.db.Query(`SELECT id FROM access_profiles ORDER BY created_at, id LIMIT ? OFFSET ?`, pageSize, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list access profiles")
			return
		}
		var profileIDs []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				writeError(w, http.StatusInternalServerError, "scan profile")
				return
			}
			profileIDs = append(profileIDs, id)
		}
		if err := rows.Close(); err != nil {
			writeError(w, http.StatusInternalServerError, "close profile rows")
			return
		}
		profiles := make([]applicationprofiles.Summary, 0, len(profileIDs))
		for _, profileID := range profileIDs {
			cfg, err := g.loadAccessProfileConfig(profileID)
			if err != nil {
				continue
			}
			profiles = append(profiles, g.accessProfileSummary(cfg))
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": profiles, "access_profiles": profiles, "total": total})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (g *Gateway) listAccessProfilesSummary() []applicationprofiles.Summary {
	rows, err := g.db.Query(`SELECT id FROM access_profiles ORDER BY created_at, id LIMIT 20`)
	if err != nil {
		return []applicationprofiles.Summary{}
	}
	var profileIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		profileIDs = append(profileIDs, id)
	}
	if err := rows.Close(); err != nil {
		return []applicationprofiles.Summary{}
	}
	profiles := make([]applicationprofiles.Summary, 0, len(profileIDs))
	for _, profileID := range profileIDs {
		cfg, err := g.loadAccessProfileConfig(profileID)
		if err != nil {
			continue
		}
		profiles = append(profiles, g.accessProfileSummary(cfg))
	}
	if profiles == nil {
		profiles = []applicationprofiles.Summary{}
	}
	return profiles
}

func (g *Gateway) handleAccessProfileSubroutes(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/access-profiles/")
	profileID, rest, ok := strings.Cut(trimmed, "/")
	if !ok {
		g.handleAccessProfileRoot(w, r, profileID)
		return
	}
	if rest == "proxy-credentials" {
		g.handleAccessProfileCredentials(w, r, profileID)
		return
	}
	if strings.HasPrefix(rest, "actions/") {
		action := strings.TrimPrefix(rest, "actions/")
		g.handleAccessProfileAction(w, r, profileID, action)
		return
	}
	prefix, credentialID, ok := strings.Cut(rest, "/")
	if !ok || prefix != "proxy-credentials" || credentialID == "" || strings.Contains(credentialID, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	g.handleAccessProfileCredential(w, r, profileID, credentialID)
}

func (g *Gateway) handleAccessProfileRoot(w http.ResponseWriter, r *http.Request, profileID string) {
	switch r.Method {
	case http.MethodGet:
		g.handleAccessProfileGet(w, r, profileID)
	case http.MethodPatch:
		g.handleAccessProfilePatch(w, r, profileID)
	case http.MethodDelete:
		g.handleAccessProfileDelete(w, r, profileID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (g *Gateway) handleAccessProfileCredentials(w http.ResponseWriter, r *http.Request, profileID string) {
	switch r.Method {
	case http.MethodGet:
		g.handleAccessProfileCredentialList(w, r, profileID)
	case http.MethodPost:
		g.handleAccessProfileCredentialCreate(w, r, profileID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (g *Gateway) handleAccessProfileCredentialCreate(w http.ResponseWriter, r *http.Request, profileID string) {
	var req struct {
		Remark   string `json:"remark"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := domainprofile.ValidateProxyCredential(req.Remark, req.Password); errors.Is(err, domainprofile.ErrCredentialRemarkRequired) {
		writeError(w, http.StatusBadRequest, validationProxyCredentialRemarkRequired)
		return
	} else if errors.Is(err, domainprofile.ErrCredentialPasswordLength) {
		writeError(w, http.StatusBadRequest, validationProxyCredentialPasswordLength)
		return
	} else if errors.Is(err, domainprofile.ErrCredentialPasswordCharset) {
		writeError(w, http.StatusBadRequest, validationProxyCredentialPasswordCharset)
		return
	} else if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var exists int
	if err := g.db.QueryRow(`SELECT 1 FROM access_profiles WHERE id = ?`, profileID).Scan(&exists); err != nil {
		writeError(w, http.StatusBadRequest, "access profile not found")
		return
	}
	var dupExists int
	if err := g.db.QueryRow(`SELECT 1 FROM proxy_credentials WHERE profile_id = ? AND password = ?`, profileID, req.Password).Scan(&dupExists); err == nil {
		writeError(w, http.StatusConflict, validationProxyCredentialPasswordDuplicate)
		return
	}
	id, err := prefixedID("cred")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create credential id")
		return
	}
	_, err = g.db.Exec(
		`INSERT INTO proxy_credentials (id, profile_id, remark, password, password_hash, created_at)
		 VALUES (?, ?, ?, ?, '', ?)`,
		id, profileID, strings.TrimSpace(req.Remark), req.Password, unixMillisNow(),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create proxy credential")
		return
	}
	var profileIdentifier string
	_ = g.db.QueryRow(`SELECT profile_identifier FROM access_profiles WHERE id = ?`, profileID).Scan(&profileIdentifier)
	if profileIdentifier == "" {
		profileIdentifier = profileID
	}
	endpoint := g.proxyEndpoint(r)
	httpURL, httpsURL, socks5URL := proxyURLs(profileIdentifier, req.Password, endpoint)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                id,
		"access_profile_id": profileID,
		"remark":            strings.TrimSpace(req.Remark),
		"password":          req.Password,
		"enabled":           true,
		"created_at":        0,
		"last_used_at":      nil,
		"http_proxy_url":    httpURL,
		"https_proxy_url":   httpsURL,
		"socks5_proxy_url":  socks5URL,
	})
}

func (g *Gateway) handleAccessProfileCredentialList(w http.ResponseWriter, r *http.Request, profileID string) {
	if !g.accessProfileExists(profileID) {
		writeError(w, http.StatusNotFound, "access profile not found")
		return
	}
	var profileIdentifier string
	_ = g.db.QueryRow(`SELECT profile_identifier FROM access_profiles WHERE id = ?`, profileID).Scan(&profileIdentifier)
	if profileIdentifier == "" {
		profileIdentifier = profileID
	}
	endpoint := g.proxyEndpoint(r)
	rows, err := g.db.Query(
		`SELECT id, remark, password, enabled, created_at, last_used_at
		   FROM proxy_credentials
		  WHERE profile_id = ?
		  ORDER BY created_at, id`,
		profileID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list proxy credentials")
		return
	}
	credentials := []map[string]any{}
	for rows.Next() {
		var id, remark, password string
		var enabled int
		var createdAt, lastUsedAt int64
		if err := rows.Scan(&id, &remark, &password, &enabled, &createdAt, &lastUsedAt); err != nil {
			_ = rows.Close()
			writeError(w, http.StatusInternalServerError, "scan proxy credential")
			return
		}
		httpURL, httpsURL, socks5URL := proxyURLs(profileIdentifier, password, endpoint)
		credentials = append(credentials, map[string]any{
			"id":                id,
			"access_profile_id": profileID,
			"remark":            remark,
			"password":          password,
			"enabled":           enabled == 1,
			"created_at":        createdAt,
			"last_used_at":      lastUsedAt,
			"http_proxy_url":    httpURL,
			"https_proxy_url":   httpsURL,
			"socks5_proxy_url":  socks5URL,
		})
	}
	if err := rows.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "close proxy credential rows")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": credentials, "proxy_credentials": credentials, "total": len(credentials)})
}

func (g *Gateway) handleAccessProfileCredential(w http.ResponseWriter, r *http.Request, profileID string, credentialID string) {
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Enabled == nil {
			writeError(w, http.StatusBadRequest, validationEnabledRequired)
			return
		}
		enabled := 0
		if *req.Enabled {
			enabled = 1
		}
		result, err := g.db.Exec(`UPDATE proxy_credentials SET enabled = ? WHERE id = ? AND profile_id = ?`, enabled, credentialID, profileID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update proxy credential")
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			writeError(w, http.StatusNotFound, "proxy credential not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
	case http.MethodDelete:
		if !g.accessProfileExists(profileID) {
			writeError(w, http.StatusNotFound, "access profile not found")
			return
		}
		result, err := g.db.Exec(`DELETE FROM proxy_credentials WHERE id = ? AND profile_id = ?`, credentialID, profileID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "delete proxy credential")
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			writeError(w, http.StatusNotFound, "proxy credential not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (g *Gateway) handleAccessProfileDelete(w http.ResponseWriter, r *http.Request, profileID string) {
	if !g.accessProfileExists(profileID) {
		writeError(w, http.StatusNotFound, "access profile not found")
		return
	}
	tx, err := g.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete access profile")
		return
	}
	if _, err := tx.Exec(`DELETE FROM proxy_credentials WHERE profile_id = ?`, profileID); err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, "delete proxy credentials")
		return
	}
	if _, err := tx.Exec(`DELETE FROM maintenance_runs WHERE target_id = ?`, profileID); err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, "delete maintenance runs")
		return
	}
	retainedNodeIDs, err := retainedNodeIDsForProfileTx(tx, profileID)
	if err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, "load retained nodes")
		return
	}
	if _, err := tx.Exec(`DELETE FROM retained_profile_nodes WHERE profile_id = ?`, profileID); err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, "release retained nodes")
		return
	}
	result, err := tx.Exec(`DELETE FROM access_profiles WHERE id = ?`, profileID)
	if err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, "delete access profile")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		_ = tx.Rollback()
		writeError(w, http.StatusNotFound, "access profile not found")
		return
	}
	deletedFingerprints, err := cleanupNodesWithoutReferencesTx(tx, retainedNodeIDs)
	if err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, "cleanup retained nodes")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "delete access profile")
		return
	}
	g.invalidateRuntimeFingerprints(deletedFingerprints)
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (g *Gateway) handleAccessProfilePatch(w http.ResponseWriter, r *http.Request, profileID string) {
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req accessProfilePatchRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.isEmpty() {
		writeError(w, http.StatusBadRequest, validationAccessProfilePatchRequired)
		return
	}
	cfg, err := g.loadAccessProfileConfig(profileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "access profile not found")
		return
	}
	original := cfg
	applyAccessProfilePatch(&cfg, req)
	if err := g.validateAccessProfileConfig(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	plan := domainprofile.PlanConfigUpdate(original.domainSnapshot(), cfg.domainSnapshot())
	cfg.applyDomainSnapshot(plan.Config)
	var deletedFingerprints []string
	if plan.ReleaseRetainedNodes {
		tx, err := g.db.Begin()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update access profile")
			return
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback()
			}
		}()
		if err := g.updateAccessProfileConfigTx(tx, cfg, plan.EvaluationChanged, plan.ResetCurrentPath); err != nil {
			writeError(w, http.StatusInternalServerError, "update access profile")
			return
		}
		deletedFingerprints, err = releaseRetainedProfileNodesExceptTx(tx, profileID, nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "release retained nodes")
			return
		}
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "update access profile")
			return
		}
		committed = true
	} else if err := g.updateAccessProfileConfig(cfg, plan.EvaluationChanged, plan.ResetCurrentPath); err != nil {
		writeError(w, http.StatusInternalServerError, "update access profile")
		return
	}
	g.invalidateRuntimeFingerprints(deletedFingerprints)
	if plan.EnqueueEvaluation {
		_, _ = g.enqueueProfileEvaluationRun(profileID, cfg.Name, "access_profile_change", cfg.ConfigVersion, true)
	}
	if plan.EnqueueUnknownCountryObservation {
		g.enqueueUnknownCountryObservations(cfg.candidateFilter())
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

type accessProfileConfig struct {
	ID                           string
	Name                         string
	ProfileIdentifier            string
	Type                         string
	FixedNodeID                  string
	ExitNodeIDs                  []string
	ChainEvaluationMode          string
	TestURL                      string
	EgressCountry                string
	EgressCountryMode            string
	EgressCountries              []string
	NodeSourceMode               string
	SourceIDs                    []string
	Protocols                    []string
	NameIncludeRegex             string
	NameExcludeRegex             string
	ManualOnly                   bool
	MinEvalInterval              int
	CandidateLimit               int
	RelativeImprovementThreshold float64
	AbsoluteLatencyImprovementMS int
	CurrentNodeID                string
	CurrentExitNodeID            string
	State                        string
	LastEvaluatedAt              int64
	LastError                    string
	CurrentPathLatencyMS         int64
	SwitchReason                 string
	LastEvaluationDetailsJSON    string
	AutoEvalEnabled              bool
	AutoEvalInterval             int
	NodeStickyEnabled            bool
	ConfigVersion                int64
}

func (cfg accessProfileConfig) domainSnapshot() domainprofile.ConfigSnapshot {
	return domainprofile.ConfigSnapshot{
		Type:                         cfg.Type,
		FixedNodeID:                  cfg.FixedNodeID,
		ExitNodeIDs:                  cfg.ExitNodeIDs,
		ChainEvaluationMode:          cfg.ChainEvaluationMode,
		TestURL:                      cfg.TestURL,
		EgressCountry:                cfg.EgressCountry,
		EgressCountryMode:            cfg.EgressCountryMode,
		EgressCountries:              cfg.EgressCountries,
		NodeSourceMode:               cfg.NodeSourceMode,
		SourceIDs:                    cfg.SourceIDs,
		Protocols:                    cfg.Protocols,
		NameIncludeRegex:             cfg.NameIncludeRegex,
		NameExcludeRegex:             cfg.NameExcludeRegex,
		ManualOnly:                   cfg.ManualOnly,
		MinEvaluationIntervalSeconds: cfg.MinEvalInterval,
		CandidateLimit:               cfg.CandidateLimit,
		RelativeImprovementThreshold: cfg.RelativeImprovementThreshold,
		AbsoluteLatencyImprovementMS: cfg.AbsoluteLatencyImprovementMS,
		CurrentNodeID:                cfg.CurrentNodeID,
		CurrentExitNodeID:            cfg.CurrentExitNodeID,
		State:                        cfg.State,
		CurrentPathLatencyMS:         cfg.CurrentPathLatencyMS,
		SwitchReason:                 cfg.SwitchReason,
		LastEvaluationDetailsJSON:    cfg.LastEvaluationDetailsJSON,
		AutoEvaluationEnabled:        cfg.AutoEvalEnabled,
		AutoEvaluationInterval:       cfg.AutoEvalInterval,
		NodeStickyEnabled:            cfg.NodeStickyEnabled,
		ConfigVersion:                cfg.ConfigVersion,
	}
}

func (cfg *accessProfileConfig) applyDomainSnapshot(snapshot domainprofile.ConfigSnapshot) {
	cfg.CurrentNodeID = snapshot.CurrentNodeID
	cfg.CurrentExitNodeID = snapshot.CurrentExitNodeID
	cfg.CurrentPathLatencyMS = snapshot.CurrentPathLatencyMS
	cfg.SwitchReason = snapshot.SwitchReason
	cfg.LastEvaluationDetailsJSON = snapshot.LastEvaluationDetailsJSON
	cfg.State = snapshot.State
	cfg.ConfigVersion = snapshot.ConfigVersion
}

type accessProfilePatchRequest struct {
	Name                *string                          `json:"name"`
	ProfileIdentifier   *string                          `json:"profile_identifier"`
	Type                *string                          `json:"type"`
	FixedNodeID         *string                          `json:"fixed_node_id"`
	ExitNodeIDs         *[]string                        `json:"exit_node_ids"`
	ChainEvaluationMode *string                          `json:"chain_evaluation_mode"`
	TestURL             *string                          `json:"test_url"`
	CandidateFilter     *accessProfileCandidateFilter    `json:"candidate_filter"`
	SwitchingTolerance  *accessProfileSwitchingTolerance `json:"switching_tolerance"`
	EvaluationSchedule  *accessProfileEvaluationSchedule `json:"evaluation_schedule"`
	EgressCountry       *string                          `json:"egress_country"`
	EgressCountryMode   *string                          `json:"egress_country_mode"`
	EgressCountries     *[]string                        `json:"egress_countries"`
	NodeSourceMode      *string                          `json:"node_source_mode"`
	SourceIDs           *[]string                        `json:"source_ids"`
	Protocols           *[]string                        `json:"protocols"`
	NameIncludeRegex    *string                          `json:"name_include_regex"`
	NameExcludeRegex    *string                          `json:"name_exclude_regex"`
	ManualOnly          *bool                            `json:"manual_only"`
	CandidateLimit      *int                             `json:"candidate_limit"`
	MinEvalInterval     *int                             `json:"min_evaluation_interval_seconds"`
	AutoEvalEnabled     *bool                            `json:"auto_evaluation_enabled"`
	AutoEvalInterval    *int                             `json:"auto_evaluation_interval_seconds"`
	NodeStickyEnabled   *bool                            `json:"node_sticky_enabled"`
}

type accessProfileCandidateFilter struct {
	SourceMode        string   `json:"source_mode"`
	SourceIDs         []string `json:"source_ids"`
	Protocols         []string `json:"protocols"`
	NameInclude       string   `json:"name_include"`
	NameExclude       string   `json:"name_exclude"`
	EgressCountryMode string   `json:"egress_country_mode"`
	EgressCountries   []string `json:"egress_countries"`
}

type accessProfileSwitchingTolerance struct {
	RelativeImprovementThreshold *float64 `json:"relative_improvement_threshold"`
	AbsoluteLatencyImprovementMS *int     `json:"absolute_latency_improvement_ms"`
}

type accessProfileEvaluationSchedule struct {
	Mode            string `json:"mode"`
	IntervalSeconds *int   `json:"interval_seconds"`
}

func (req accessProfilePatchRequest) isEmpty() bool {
	return req.Name == nil &&
		req.ProfileIdentifier == nil &&
		req.Type == nil &&
		req.FixedNodeID == nil &&
		req.ExitNodeIDs == nil &&
		req.ChainEvaluationMode == nil &&
		req.TestURL == nil &&
		req.CandidateFilter == nil &&
		req.SwitchingTolerance == nil &&
		req.EvaluationSchedule == nil &&
		req.EgressCountry == nil &&
		req.EgressCountryMode == nil &&
		req.EgressCountries == nil &&
		req.NodeSourceMode == nil &&
		req.SourceIDs == nil &&
		req.Protocols == nil &&
		req.NameIncludeRegex == nil &&
		req.NameExcludeRegex == nil &&
		req.ManualOnly == nil &&
		req.CandidateLimit == nil &&
		req.MinEvalInterval == nil &&
		req.AutoEvalEnabled == nil &&
		req.AutoEvalInterval == nil &&
		req.NodeStickyEnabled == nil
}

func applyAccessProfilePatch(cfg *accessProfileConfig, req accessProfilePatchRequest) {
	if req.Name != nil {
		cfg.Name = *req.Name
	}
	if req.ProfileIdentifier != nil {
		cfg.ProfileIdentifier = *req.ProfileIdentifier
	}
	if req.Type != nil {
		cfg.Type = *req.Type
	}
	if req.FixedNodeID != nil {
		cfg.FixedNodeID = *req.FixedNodeID
	}
	if req.ExitNodeIDs != nil {
		cfg.ExitNodeIDs = *req.ExitNodeIDs
	}
	if req.ChainEvaluationMode != nil {
		cfg.ChainEvaluationMode = *req.ChainEvaluationMode
	}
	if req.TestURL != nil {
		cfg.TestURL = *req.TestURL
	}
	if req.CandidateFilter != nil {
		filter := req.CandidateFilter
		cfg.NodeSourceMode = internalNodeSourceMode(filter.SourceMode)
		cfg.SourceIDs = filter.SourceIDs
		cfg.Protocols = filter.Protocols
		cfg.NameIncludeRegex = filter.NameInclude
		cfg.NameExcludeRegex = filter.NameExclude
		cfg.EgressCountryMode = filter.EgressCountryMode
		cfg.EgressCountries = filter.EgressCountries
	}
	if req.SwitchingTolerance != nil {
		if req.SwitchingTolerance.RelativeImprovementThreshold != nil {
			cfg.RelativeImprovementThreshold = *req.SwitchingTolerance.RelativeImprovementThreshold
		}
		if req.SwitchingTolerance.AbsoluteLatencyImprovementMS != nil {
			cfg.AbsoluteLatencyImprovementMS = *req.SwitchingTolerance.AbsoluteLatencyImprovementMS
		}
	}
	if req.EvaluationSchedule != nil {
		switch strings.ToLower(strings.TrimSpace(req.EvaluationSchedule.Mode)) {
		case "disabled":
			cfg.AutoEvalEnabled = false
		case "custom":
			cfg.AutoEvalEnabled = true
			if req.EvaluationSchedule.IntervalSeconds != nil {
				cfg.AutoEvalInterval = *req.EvaluationSchedule.IntervalSeconds
			}
		case "inherit", "":
			cfg.AutoEvalEnabled = true
			if req.EvaluationSchedule.Mode == "inherit" {
				cfg.AutoEvalInterval = 0
			} else if req.EvaluationSchedule.IntervalSeconds != nil {
				cfg.AutoEvalInterval = *req.EvaluationSchedule.IntervalSeconds
			}
		}
	}
	if req.EgressCountry != nil {
		cfg.EgressCountry = *req.EgressCountry
	}
	if req.EgressCountryMode != nil {
		cfg.EgressCountryMode = *req.EgressCountryMode
	}
	if req.EgressCountries != nil {
		cfg.EgressCountries = *req.EgressCountries
	}
	if req.NodeSourceMode != nil {
		cfg.NodeSourceMode = *req.NodeSourceMode
	}
	if req.SourceIDs != nil {
		cfg.SourceIDs = *req.SourceIDs
	}
	if req.Protocols != nil {
		cfg.Protocols = *req.Protocols
	}
	if req.NameIncludeRegex != nil {
		cfg.NameIncludeRegex = *req.NameIncludeRegex
	}
	if req.NameExcludeRegex != nil {
		cfg.NameExcludeRegex = *req.NameExcludeRegex
	}
	if req.ManualOnly != nil {
		cfg.ManualOnly = *req.ManualOnly
	}
	if req.CandidateLimit != nil {
		cfg.CandidateLimit = *req.CandidateLimit
	}
	if req.MinEvalInterval != nil {
		cfg.MinEvalInterval = *req.MinEvalInterval
	}
	if req.AutoEvalEnabled != nil {
		cfg.AutoEvalEnabled = *req.AutoEvalEnabled
	}
	if req.AutoEvalInterval != nil {
		cfg.AutoEvalInterval = *req.AutoEvalInterval
	}
	if req.NodeStickyEnabled != nil {
		cfg.NodeStickyEnabled = *req.NodeStickyEnabled
	}
}

func defaultAccessProfileConfig(id string) accessProfileConfig {
	return accessProfileConfig{
		ID:                           id,
		Type:                         "fastest",
		EgressCountryMode:            "include",
		NodeSourceMode:               "all",
		RelativeImprovementThreshold: 0.2,
		AbsoluteLatencyImprovementMS: 100,
		State:                        "pending",
		AutoEvalEnabled:              true,
		ConfigVersion:                1,
	}
}

func (g *Gateway) loadAccessProfileConfig(profileID string) (accessProfileConfig, error) {
	var cfg accessProfileConfig
	var sourceIDsJSON, protocolsJSON, egressCountriesJSON, exitNodeIDsJSON string
	var manualOnly, autoEvalEnabled, nodeStickyEnabled int
	err := g.db.QueryRow(
		`SELECT id, name, profile_identifier, type, fixed_node_id, exit_node_ids_json, chain_evaluation_mode, test_url,
		        egress_country, egress_country_mode, egress_countries_json, node_source_mode, source_ids_json, protocols_json,
		        name_include_regex, name_exclude_regex, manual_only, min_evaluation_interval_seconds, candidate_limit,
		        relative_improvement_threshold, absolute_latency_improvement_ms,
		        current_node_id, current_exit_node_id, state, last_evaluated_at, last_error, current_path_latency_ms,
		        switch_reason, last_evaluation_details_json, auto_evaluation_enabled, auto_evaluation_interval_seconds, node_sticky_enabled, config_version
		   FROM access_profiles
		  WHERE id = ?`,
		profileID,
	).Scan(
		&cfg.ID,
		&cfg.Name,
		&cfg.ProfileIdentifier,
		&cfg.Type,
		&cfg.FixedNodeID,
		&exitNodeIDsJSON,
		&cfg.ChainEvaluationMode,
		&cfg.TestURL,
		&cfg.EgressCountry,
		&cfg.EgressCountryMode,
		&egressCountriesJSON,
		&cfg.NodeSourceMode,
		&sourceIDsJSON,
		&protocolsJSON,
		&cfg.NameIncludeRegex,
		&cfg.NameExcludeRegex,
		&manualOnly,
		&cfg.MinEvalInterval,
		&cfg.CandidateLimit,
		&cfg.RelativeImprovementThreshold,
		&cfg.AbsoluteLatencyImprovementMS,
		&cfg.CurrentNodeID,
		&cfg.CurrentExitNodeID,
		&cfg.State,
		&cfg.LastEvaluatedAt,
		&cfg.LastError,
		&cfg.CurrentPathLatencyMS,
		&cfg.SwitchReason,
		&cfg.LastEvaluationDetailsJSON,
		&autoEvalEnabled,
		&cfg.AutoEvalInterval,
		&nodeStickyEnabled,
		&cfg.ConfigVersion,
	)
	if err != nil {
		return accessProfileConfig{}, err
	}
	cfg.ExitNodeIDs = unmarshalStringSlice(exitNodeIDsJSON)
	cfg.EgressCountries = unmarshalStringSlice(egressCountriesJSON)
	cfg.SourceIDs = unmarshalStringSlice(sourceIDsJSON)
	cfg.Protocols = unmarshalStringSlice(protocolsJSON)
	cfg.ManualOnly = manualOnly == 1
	cfg.AutoEvalEnabled = autoEvalEnabled == 1
	cfg.NodeStickyEnabled = nodeStickyEnabled == 1
	cfg.applyDefaults()
	return cfg, nil
}

func (cfg *accessProfileConfig) applyDefaults() {
	if cfg.EgressCountryMode == "" {
		cfg.EgressCountryMode = "include"
	}
}

func (g *Gateway) validateAccessProfileConfig(cfg *accessProfileConfig) error {
	cfg.applyDefaults()
	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.Name == "" {
		return errors.New(validationAccessProfileNameRequired)
	}
	cfg.ProfileIdentifier = strings.TrimSpace(cfg.ProfileIdentifier)
	if cfg.ProfileIdentifier == "" {
		cfg.ProfileIdentifier = cfg.ID
	}
	if err := validateProfileIdentifier(cfg.ProfileIdentifier); err != nil {
		return err
	}
	var dupID string
	err := g.db.QueryRow(`SELECT id FROM access_profiles WHERE profile_identifier = ? AND id != ?`, cfg.ProfileIdentifier, cfg.ID).Scan(&dupID)
	if err == nil {
		return errors.New(validationProfileIdentifierDuplicate)
	}
	if err := domainprofile.ValidateEvaluationTiming(cfg.CandidateLimit, cfg.MinEvalInterval, cfg.AutoEvalInterval); errors.Is(err, domainprofile.ErrCandidateTimingNonNegative) {
		return errors.New(validationCandidateTimingNonNegative)
	} else if errors.Is(err, domainprofile.ErrEvaluationIntervalNonNegative) {
		return errors.New(validationEvaluationIntervalNonNegative)
	} else if err != nil {
		return err
	}
	if err := domainprofile.ValidateSwitchingTolerance(cfg.RelativeImprovementThreshold, cfg.AbsoluteLatencyImprovementMS); errors.Is(err, domainprofile.ErrSwitchingToleranceNonNegative) {
		return errors.New(validationSwitchingToleranceNonNegative)
	} else if err != nil {
		return err
	}
	cfg.FixedNodeID = strings.TrimSpace(cfg.FixedNodeID)
	cfg.TestURL = strings.TrimSpace(cfg.TestURL)
	cfg.NameIncludeRegex = strings.TrimSpace(cfg.NameIncludeRegex)
	cfg.NameExcludeRegex = strings.TrimSpace(cfg.NameExcludeRegex)
	if err := validateOptionalRegex(cfg.NameIncludeRegex, validationNameIncludeRegexInvalid); err != nil {
		return err
	}
	if err := validateOptionalRegex(cfg.NameExcludeRegex, validationNameExcludeRegexInvalid); err != nil {
		return err
	}
	cfg.ExitNodeIDs = normalizeStringList(cfg.ExitNodeIDs)
	cfg.EgressCountries = normalizeEgressCountryList(cfg.EgressCountries)
	cfg.Protocols = normalizeLowerStringList(cfg.Protocols)
	cfg.EgressCountry = normalizeEgressCountryValue(cfg.EgressCountry)
	if len(cfg.EgressCountries) == 0 && cfg.EgressCountry != "" {
		cfg.EgressCountries = []string{cfg.EgressCountry}
	}
	if len(cfg.EgressCountries) > 0 {
		cfg.EgressCountry = cfg.EgressCountries[0]
	} else {
		cfg.EgressCountry = ""
	}
	cfg.EgressCountryMode = strings.ToLower(strings.TrimSpace(cfg.EgressCountryMode))
	if cfg.EgressCountryMode == "" {
		cfg.EgressCountryMode = "include"
	}
	if cfg.EgressCountryMode != "include" && cfg.EgressCountryMode != "exclude" {
		return errors.New(validationEgressCountryMode)
	}
	cfg.NodeSourceMode = normalizeNodeSourceMode(cfg.NodeSourceMode, cfg.SourceIDs, cfg.ManualOnly)
	if cfg.NodeSourceMode == "specific_subscriptions" && len(cfg.SourceIDs) == 0 {
		return errors.New(validationSelectedSourcesRequired)
	}
	cfg.ManualOnly = cfg.NodeSourceMode == "manual"
	cfg.Type = normalizeAccessProfileType(cfg.Type)
	switch cfg.Type {
	case "fixed_node":
		if cfg.FixedNodeID == "" {
			return errors.New(validationFixedNodeRequired)
		}
		if _, err := g.loadNode(cfg.FixedNodeID); err != nil {
			return errors.New("fixed node not found")
		}
		cfg.CurrentNodeID = cfg.FixedNodeID
		cfg.CurrentExitNodeID = ""
		cfg.ExitNodeIDs = []string{}
		cfg.ChainEvaluationMode = ""
		cfg.NodeStickyEnabled = false
		cfg.State = "ready"
	case "fastest":
		if err := validateProfileTestURL(cfg.TestURL); err != nil {
			return err
		}
		cfg.TestURL = effectiveProfileTestURL(cfg.TestURL)
		cfg.CurrentExitNodeID = ""
		cfg.State = cfg.dynamicStateAfterUpdate()
	case "random":
		cfg.CurrentNodeID = ""
		cfg.CurrentExitNodeID = ""
		cfg.NodeStickyEnabled = false
		cfg.State = "ready"
	case "chain":
		if len(cfg.ExitNodeIDs) == 0 && cfg.FixedNodeID != "" {
			cfg.ExitNodeIDs = []string{cfg.FixedNodeID}
		}
		if err := domainprofile.ValidateChainExitNodes(cfg.ExitNodeIDs, "end_to_end"); errors.Is(err, domainprofile.ErrExitNodesRequired) {
			return errors.New(validationExitNodesRequired)
		}
		for _, exitNodeID := range cfg.ExitNodeIDs {
			if _, err := g.loadNode(exitNodeID); err != nil {
				return errors.New("exit node not found")
			}
		}
		cfg.FixedNodeID = cfg.ExitNodeIDs[0]
		cfg.ChainEvaluationMode = normalizeChainEvaluationMode(cfg.ChainEvaluationMode)
		if err := domainprofile.ValidateChainExitNodes(cfg.ExitNodeIDs, cfg.ChainEvaluationMode); errors.Is(err, domainprofile.ErrChainLinkSingleExitRequired) {
			return errors.New(validationChainLinkSingleExitRequired)
		}
		if cfg.ChainEvaluationMode == "end_to_end" {
			if err := validateProfileTestURL(cfg.TestURL); err != nil {
				return err
			}
			cfg.TestURL = effectiveProfileTestURL(cfg.TestURL)
		}
		if !stringInSlice(cfg.CurrentExitNodeID, cfg.ExitNodeIDs) {
			cfg.CurrentExitNodeID = ""
		}
		if cfg.CurrentNodeID != "" && cfg.CurrentExitNodeID == "" && len(cfg.ExitNodeIDs) == 1 {
			cfg.CurrentExitNodeID = cfg.ExitNodeIDs[0]
		}
		cfg.State = cfg.dynamicStateAfterUpdate()
	default:
		return errors.New("unsupported access profile type")
	}
	return nil
}

func validateOptionalRegex(pattern, message string) error {
	if pattern == "" {
		return nil
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return errors.New(message)
	}
	return nil
}

func normalizeAccessProfileType(profileType string) string {
	return strings.TrimSpace(profileType)
}

func normalizeChainEvaluationMode(mode string) string {
	return domainprofile.NormalizeChainEvaluationMode(mode)
}

func (cfg accessProfileConfig) candidateFilter() candidateFilter {
	return candidateFilter{
		EgressCountry:     cfg.EgressCountry,
		EgressCountries:   cfg.EgressCountries,
		EgressCountryMode: cfg.EgressCountryMode,
		NodeSourceMode:    cfg.NodeSourceMode,
		SourceIDs:         cfg.SourceIDs,
		Protocols:         cfg.Protocols,
		NameIncludeRegex:  cfg.NameIncludeRegex,
		NameExcludeRegex:  cfg.NameExcludeRegex,
		ManualOnly:        cfg.ManualOnly,
	}
}

func (cfg accessProfileConfig) nodeStickyEnabled() bool {
	return cfg.NodeStickyEnabled && (cfg.Type == "fastest" || cfg.Type == "chain")
}

func (g *Gateway) insertAccessProfileConfig(cfg accessProfileConfig) error {
	_, err := g.db.Exec(
		`INSERT INTO access_profiles (
			id, profile_identifier, name, type, fixed_node_id, exit_node_ids_json, chain_evaluation_mode, test_url,
			egress_country, egress_country_mode, egress_countries_json, node_source_mode, source_ids_json, protocols_json,
			name_include_regex, name_exclude_regex, manual_only, min_evaluation_interval_seconds, candidate_limit,
			relative_improvement_threshold, absolute_latency_improvement_ms,
			current_node_id, current_exit_node_id, state, auto_evaluation_enabled, auto_evaluation_interval_seconds, node_sticky_enabled, config_version, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cfg.ID,
		cfg.ProfileIdentifier,
		cfg.Name,
		cfg.Type,
		cfg.FixedNodeID,
		stringSliceJSON(cfg.ExitNodeIDs),
		cfg.ChainEvaluationMode,
		cfg.TestURL,
		cfg.EgressCountry,
		cfg.EgressCountryMode,
		stringSliceJSON(cfg.EgressCountries),
		cfg.NodeSourceMode,
		stringSliceJSON(cfg.SourceIDs),
		stringSliceJSON(cfg.Protocols),
		cfg.NameIncludeRegex,
		cfg.NameExcludeRegex,
		boolInt(cfg.ManualOnly),
		cfg.MinEvalInterval,
		cfg.CandidateLimit,
		cfg.RelativeImprovementThreshold,
		cfg.AbsoluteLatencyImprovementMS,
		cfg.CurrentNodeID,
		cfg.CurrentExitNodeID,
		cfg.State,
		boolInt(cfg.AutoEvalEnabled),
		cfg.AutoEvalInterval,
		boolInt(cfg.NodeStickyEnabled),
		cfg.ConfigVersion,
		unixMillisNow(),
	)
	return err
}

func (g *Gateway) updateAccessProfileConfig(cfg accessProfileConfig, evaluationChanged bool, resetCurrentPath bool) error {
	return g.updateAccessProfileConfigTx(g.db, cfg, evaluationChanged, resetCurrentPath)
}

type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (g *Gateway) updateAccessProfileConfigTx(exec sqlExecer, cfg accessProfileConfig, evaluationChanged bool, resetCurrentPath bool) error {
	_, err := exec.Exec(
		`UPDATE access_profiles
		    SET name = ?,
		        profile_identifier = ?,
		        type = ?,
		        fixed_node_id = ?,
		        exit_node_ids_json = ?,
		        chain_evaluation_mode = ?,
		        test_url = ?,
		        egress_country = ?,
		        egress_country_mode = ?,
		        egress_countries_json = ?,
		        node_source_mode = ?,
		        source_ids_json = ?,
		        protocols_json = ?,
		        name_include_regex = ?,
		        name_exclude_regex = ?,
		        manual_only = ?,
		        min_evaluation_interval_seconds = ?,
		        candidate_limit = ?,
		        relative_improvement_threshold = ?,
		        absolute_latency_improvement_ms = ?,
		        current_node_id = ?,
		        current_exit_node_id = ?,
		        state = ?,
		        auto_evaluation_enabled = ?,
		        auto_evaluation_interval_seconds = ?,
		        node_sticky_enabled = ?,
		        last_error = CASE WHEN ? = 1 THEN '' ELSE last_error END,
		        current_path_failed_evaluations = CASE WHEN ? = 1 THEN 0 ELSE current_path_failed_evaluations END,
		        current_path_missed_success_cycles = CASE WHEN ? = 1 THEN 0 ELSE current_path_missed_success_cycles END,
		        switch_reason = CASE WHEN ? = 1 THEN ? ELSE switch_reason END,
		        last_evaluation_details_json = CASE WHEN ? = 1 THEN ? ELSE last_evaluation_details_json END,
		        last_evaluated_at = CASE WHEN ? = 1 THEN 0 ELSE last_evaluated_at END,
		        last_evaluation_started_at = CASE WHEN ? = 1 THEN 0 ELSE last_evaluation_started_at END,
		        config_version = ?
		  WHERE id = ?`,
		cfg.Name,
		cfg.ProfileIdentifier,
		cfg.Type,
		cfg.FixedNodeID,
		stringSliceJSON(cfg.ExitNodeIDs),
		cfg.ChainEvaluationMode,
		cfg.TestURL,
		cfg.EgressCountry,
		cfg.EgressCountryMode,
		stringSliceJSON(cfg.EgressCountries),
		cfg.NodeSourceMode,
		stringSliceJSON(cfg.SourceIDs),
		stringSliceJSON(cfg.Protocols),
		cfg.NameIncludeRegex,
		cfg.NameExcludeRegex,
		boolInt(cfg.ManualOnly),
		cfg.MinEvalInterval,
		cfg.CandidateLimit,
		cfg.RelativeImprovementThreshold,
		cfg.AbsoluteLatencyImprovementMS,
		cfg.CurrentNodeID,
		cfg.CurrentExitNodeID,
		cfg.State,
		boolInt(cfg.AutoEvalEnabled),
		cfg.AutoEvalInterval,
		boolInt(cfg.NodeStickyEnabled),
		boolInt(evaluationChanged),
		boolInt(resetCurrentPath),
		boolInt(resetCurrentPath),
		boolInt(resetCurrentPath),
		cfg.SwitchReason,
		boolInt(resetCurrentPath),
		cfg.LastEvaluationDetailsJSON,
		boolInt(resetCurrentPath),
		boolInt(resetCurrentPath),
		cfg.ConfigVersion,
		cfg.ID,
	)
	return err
}

func (cfg accessProfileConfig) dynamicStateAfterUpdate() string {
	if cfg.AutoEvalEnabled {
		return "running"
	}
	if cfg.CurrentNodeID != "" {
		return "ready"
	}
	return "pending"
}

func (g *Gateway) accessProfileExists(profileID string) bool {
	var exists int
	err := g.db.QueryRow(`SELECT 1 FROM access_profiles WHERE id = ?`, profileID).Scan(&exists)
	return err == nil
}

func (g *Gateway) proxyPathForCredential(credential proxyCredentialRecord) (selectedProxyPath, error) {
	cfg, err := g.loadAccessProfileConfig(credential.ProfileID)
	if err != nil {
		return selectedProxyPath{}, errors.New("access profile not found")
	}
	switch cfg.Type {
	case "random":
		nodes, err := g.candidateNodes(cfg.candidateFilter())
		if err != nil || len(nodes) == 0 {
			return selectedProxyPath{}, errors.New("access profile has no usable proxy path")
		}
		nodes = g.usableNodes(nodes)
		if len(nodes) == 0 {
			return selectedProxyPath{}, errors.New("access profile has no usable proxy path")
		}
		idx, err := cryptoRandomIndex(len(nodes))
		if err != nil {
			return selectedProxyPath{}, err
		}
		return selectedProxyPath{
			Credential:        credential,
			ProfileID:         credential.ProfileID,
			Profile:           cfg.Name,
			ProfileIdentifier: cfg.effectiveProfileIdentifier(),
			Node:              nodes[idx],
		}, nil
	case "chain":
		exitNodeID := cfg.CurrentExitNodeID
		if exitNodeID == "" && len(cfg.ExitNodeIDs) == 1 {
			exitNodeID = cfg.ExitNodeIDs[0]
		}
		if !profileStateHasReusablePath(cfg.State) || cfg.CurrentNodeID == "" || exitNodeID == "" {
			return selectedProxyPath{}, errors.New("access profile has no usable proxy path")
		}
		if !g.chainPathMatchesProfile(cfg, cfg.CurrentNodeID, exitNodeID) {
			return selectedProxyPath{}, errors.New("access profile has no usable proxy path")
		}
		frontNode, err := g.loadUsableNode(cfg.CurrentNodeID)
		if err != nil {
			return selectedProxyPath{}, err
		}
		exitNode, err := g.loadUsableNode(exitNodeID)
		if err != nil {
			return selectedProxyPath{}, err
		}
		return selectedProxyPath{
			Credential:        credential,
			ProfileID:         credential.ProfileID,
			Profile:           cfg.Name,
			ProfileIdentifier: cfg.effectiveProfileIdentifier(),
			FrontNode:         frontNode,
			ExitNode:          exitNode,
		}, nil
	default:
		if !profileStateHasReusablePath(cfg.State) || cfg.CurrentNodeID == "" {
			return selectedProxyPath{}, errors.New("access profile has no usable proxy path")
		}
		if cfg.Type == "fastest" && !g.profileNodeMatchesCandidateFilter(cfg.ID, cfg.CurrentNodeID, cfg.candidateFilter()) {
			return selectedProxyPath{}, errors.New("access profile has no usable proxy path")
		}
		node, err := g.loadUsableNode(cfg.CurrentNodeID)
		if err != nil {
			return selectedProxyPath{}, err
		}
		return selectedProxyPath{
			Credential:        credential,
			ProfileID:         credential.ProfileID,
			Profile:           cfg.Name,
			ProfileIdentifier: cfg.effectiveProfileIdentifier(),
			Node:              node,
		}, nil
	}
}

func profileStateHasReusablePath(state string) bool {
	return state == "ready" || state == "degraded" || state == "running"
}

func (g *Gateway) profileWaitingForObservation(profileID string) bool {
	var state string
	err := g.db.QueryRow(`SELECT state FROM access_profiles WHERE id = ?`, profileID).Scan(&state)
	return err == nil && state == "waiting_observation"
}

func (g *Gateway) loadUsableNode(nodeID string) (nodeRecord, error) {
	node, err := g.loadNode(nodeID)
	if err != nil {
		return nodeRecord{}, err
	}
	if !node.Enabled {
		return nodeRecord{}, errors.New("node is disabled")
	}
	return node, nil
}

func (g *Gateway) usableNodes(nodes []nodeRecord) []nodeRecord {
	usable := make([]nodeRecord, 0, len(nodes))
	for _, node := range nodes {
		if node.Enabled && g.nodeUsable(node.ID) {
			usable = append(usable, node)
		}
	}
	return usable
}

func (g *Gateway) nodeUsable(nodeID string) bool {
	var usable int
	err := g.db.QueryRow(`SELECT usable FROM node_observations WHERE node_id = ?`, nodeID).Scan(&usable)
	return err == nil && usable == 1
}

func (g *Gateway) nodeIDMatchesCandidateFilter(nodeID string, filter candidateFilter) bool {
	nodes, err := g.candidateNodes(filter)
	if err != nil {
		return false
	}
	return nodeIDInRecords(nodeID, nodes)
}

func (g *Gateway) profileNodeMatchesCandidateFilter(profileID, nodeID string, filter candidateFilter) bool {
	return g.nodeIDMatchesCandidateFilter(nodeID, filter) || g.profileRetainsNode(profileID, nodeID)
}

func (g *Gateway) chainPathMatchesProfile(cfg accessProfileConfig, frontNodeID, exitNodeID string) bool {
	if !stringInSlice(exitNodeID, cfg.ExitNodeIDs) {
		return false
	}
	nodes, err := g.candidateNodes(cfg.candidateFilter())
	if err != nil {
		return g.profileRetainsNode(cfg.ID, frontNodeID)
	}
	nodes = excludeNodes(nodes, cfg.ExitNodeIDs)
	return nodeIDInRecords(frontNodeID, nodes) || g.profileRetainsNode(cfg.ID, frontNodeID)
}

func (g *Gateway) unknownCountryCandidateCount(filter candidateFilter) int {
	filter.EgressCountry = ""
	filter.EgressCountries = nil
	nodes, err := g.candidateNodes(filter)
	if err != nil {
		return 0
	}
	count := 0
	for _, node := range nodes {
		var country string
		err := g.db.QueryRow(`SELECT egress_country FROM node_observations WHERE node_id = ? AND usable = 1`, node.ID).Scan(&country)
		if err != nil || strings.TrimSpace(country) == "" {
			count++
		}
	}
	return count
}

func (g *Gateway) enqueueUnknownCountryObservations(filter candidateFilter) {
	filter.EgressCountry = ""
	filter.EgressCountries = nil
	nodes, err := g.candidateNodes(filter)
	if err != nil {
		return
	}
	settings, _ := g.loadMaintenanceSettings()
	probeURL := settings.EgressIPProbeURL
	if probeURL == "" {
		probeURL = defaultEgressIPProbeURL
	}
	var targets []nodeRecord
	for _, node := range nodes {
		var country string
		err := g.db.QueryRow(`SELECT egress_country FROM node_observations WHERE node_id = ? AND usable = 1`, node.ID).Scan(&country)
		if err == nil && strings.TrimSpace(country) != "" {
			continue
		}
		targets = append(targets, node)
	}
	if len(targets) > 0 {
		_, _ = g.createNodeObservationRun("country_profile_unknown_country", "all_nodes", targets, probeURL)
		g.notifyMaintenanceRunner()
	}
}

func (g *Gateway) accessProfileSummary(cfg accessProfileConfig) applicationprofiles.Summary {
	var totalCreds, enabledCreds int
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM proxy_credentials WHERE profile_id = ?`, cfg.ID).Scan(&totalCreds)
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM proxy_credentials WHERE profile_id = ? AND enabled = 1`, cfg.ID).Scan(&enabledCreds)
	return applicationprofiles.BuildSummary(applicationprofiles.SummaryInput{
		ID:                      cfg.ID,
		Name:                    cfg.Name,
		Type:                    cfg.Type,
		State:                   cfg.State,
		ProfileIdentifier:       cfg.ProfileIdentifier,
		CurrentNodeID:           cfg.CurrentNodeID,
		CurrentExitNodeID:       cfg.CurrentExitNodeID,
		NodeSourceMode:          cfg.NodeSourceMode,
		SourceIDs:               cfg.SourceIDs,
		EgressCountry:           cfg.EgressCountry,
		EgressCountryMode:       cfg.EgressCountryMode,
		EgressCountries:         cfg.EgressCountries,
		NameIncludeRegex:        cfg.NameIncludeRegex,
		NameExcludeRegex:        cfg.NameExcludeRegex,
		CandidateLimit:          cfg.CandidateLimit,
		MinEvaluationInterval:   cfg.MinEvalInterval,
		AutoEvaluationEnabled:   cfg.AutoEvalEnabled,
		AutoEvaluationInterval:  cfg.AutoEvalInterval,
		NodeStickyEnabled:       cfg.NodeStickyEnabled,
		ConfigVersion:           cfg.ConfigVersion,
		CurrentPath:             g.accessProfileCurrentPath(cfg),
		ProxyCredentialsCount:   totalCreds,
		EnabledCredentialsCount: enabledCreds,
		LastEvaluatedAt:         cfg.LastEvaluatedAt,
		LastError:               cfg.LastError,
		SwitchReason:            cfg.SwitchReason,
	})
}

func (cfg accessProfileConfig) effectiveProfileIdentifier() string {
	if cfg.ProfileIdentifier != "" {
		return cfg.ProfileIdentifier
	}
	return cfg.ID
}

func (g *Gateway) accessProfileCurrentPath(cfg accessProfileConfig) any {
	if cfg.Type == "chain" {
		if cfg.CurrentNodeID == "" || cfg.CurrentExitNodeID == "" {
			return nil
		}
		if !g.chainPathMatchesProfile(cfg, cfg.CurrentNodeID, cfg.CurrentExitNodeID) {
			return nil
		}
		frontNode, frontOK := g.nodePathSummary(cfg.CurrentNodeID)
		exitNode, exitOK := g.nodePathSummary(cfg.CurrentExitNodeID)
		if !frontOK || !exitOK {
			return nil
		}
		return applicationprofiles.BuildChainPathSummary(frontNode, exitNode, cfg.ChainEvaluationMode, cfg.CurrentPathLatencyMS, cfg.LastEvaluatedAt)
	}
	if cfg.CurrentNodeID == "" {
		return nil
	}
	if cfg.Type == "fastest" && !g.profileNodeMatchesCandidateFilter(cfg.ID, cfg.CurrentNodeID, cfg.candidateFilter()) {
		return nil
	}
	node, ok := g.nodePathSummary(cfg.CurrentNodeID)
	if !ok {
		return nil
	}
	return applicationprofiles.BuildSinglePathSummary(node, cfg.CurrentPathLatencyMS, cfg.LastEvaluatedAt)
}

func (g *Gateway) nodePathSummary(nodeID string) (applicationprofiles.NodePathSummary, bool) {
	node, err := g.loadNode(nodeID)
	if err != nil {
		return applicationprofiles.NodePathSummary{}, false
	}
	var egressIP any = nil
	country := ""
	var latency any = nil
	var observedAt any = nil
	obs := g.nodeObservation(nodeID)
	if obs != nil {
		if ip, ok := obs["egress_ip"].(string); ok && ip != "" {
			egressIP = ip
		}
		if ec, ok := obs["egress_country"].(string); ok {
			country = ec
		}
		if lat, ok := obs["latency_ms"].(int64); ok && lat > 0 {
			latency = lat
		}
		if ts, ok := obs["last_success_at"].(int64); ok && ts > 0 {
			observedAt = ts
		}
	}
	return applicationprofiles.NodePathSummary{
		ID:                   node.ID,
		Name:                 node.Name,
		Protocol:             node.Type,
		Server:               node.Server,
		ServerPort:           node.ServerPort,
		EgressIP:             egressIP,
		EgressCountry:        egressCountryDisplay(country),
		ObservationLatencyMS: latency,
		LastObservedAt:       observedAt,
	}, true
}

func egressCountryDisplay(country string) map[string]any {
	country = normalizeEgressCountryValue(country)
	if country == "" || country == "__unknown__" {
		return map[string]any{"value": "__unknown__", "iso_code": nil, "name_zh": "未知", "is_unknown": true}
	}
	return map[string]any{"value": country, "iso_code": country, "name_zh": countryNameZH(country), "is_unknown": false}
}

func nullableUnixMillis(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func cryptoRandomIndex(n int) (int, error) {
	if n <= 0 {
		return 0, errors.New("empty random range")
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

func (g *Gateway) handleAccessProfileGet(w http.ResponseWriter, r *http.Request, profileID string) {
	cfg, err := g.loadAccessProfileConfig(profileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "access profile not found")
		return
	}
	sourceIDs := cfg.SourceIDs
	if sourceIDs == nil {
		sourceIDs = []string{}
	}
	profileIdentifier := cfg.effectiveProfileIdentifier()
	endpoint := g.proxyEndpoint(r)
	credRows, cErr := g.db.Query(
		`SELECT id, remark, password, enabled, created_at, last_used_at FROM proxy_credentials WHERE profile_id = ? ORDER BY created_at, id`, profileID)
	credentials := []applicationprofiles.Credential{}
	if cErr == nil {
		for credRows.Next() {
			var cid, remark, password string
			var enabled int
			var createdAt, lastUsedAt int64
			if err := credRows.Scan(&cid, &remark, &password, &enabled, &createdAt, &lastUsedAt); err != nil {
				continue
			}
			httpURL, httpsURL, socks5URL := proxyURLs(profileIdentifier, password, endpoint)
			credentials = append(credentials, applicationprofiles.Credential{
				ID:              cid,
				AccessProfileID: profileID,
				Remark:          remark,
				Password:        password,
				Enabled:         enabled == 1,
				CreatedAt:       createdAt,
				LastUsedAt:      lastUsedAt,
				HTTPProxyURL:    httpURL,
				HTTPSProxyURL:   httpsURL,
				SOCKS5ProxyURL:  socks5URL,
			})
		}
		_ = credRows.Close()
	}
	var totalCreds, enabledCreds int
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM proxy_credentials WHERE profile_id = ?`, profileID).Scan(&totalCreds)
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM proxy_credentials WHERE profile_id = ? AND enabled = 1`, profileID).Scan(&enabledCreds)
	filter := cfg.candidateFilter()
	nodes, _ := g.candidateNodes(filter)
	usableCount := 0
	candidateNodeIDs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		candidateNodeIDs = append(candidateNodeIDs, n.ID)
		if g.nodeUsable(n.ID) {
			usableCount++
		}
	}
	domainStats := domainprofile.BuildCandidateStats(cfg.Type, candidateNodeIDs, usableCount, g.unknownCountryCandidateCount(filter), cfg.ExitNodeIDs)
	candidateStats := applicationprofiles.CandidateStats{
		Total:                domainStats.Total,
		Usable:               domainStats.Usable,
		UnknownEgressCountry: domainStats.UnknownEgressCountry,
		FrontCandidates:      domainStats.FrontCandidates,
		ExitNodes:            domainStats.ExitNodes,
		PathCombinations:     domainStats.PathCombinations,
	}
	eventRows, eErr := g.db.Query(
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE target_id = ? AND run_type IN ('profile_evaluation', 'profile_switch')
		  ORDER BY created_at DESC, rowid DESC LIMIT 10`,
		profileID,
	)
	recentEvents := []map[string]any{}
	if eErr == nil {
		defer eventRows.Close()
		for eventRows.Next() {
			run, err := scanMaintenanceRun(eventRows)
			if err != nil {
				continue
			}
			recentEvents = append(recentEvents, run.toMap())
		}
	}
	egressCountries := cfg.EgressCountries
	if egressCountries == nil {
		egressCountries = []string{}
	}
	protocols := cfg.Protocols
	if protocols == nil {
		protocols = []string{}
	}
	exitNodeIDs := cfg.ExitNodeIDs
	if exitNodeIDs == nil {
		exitNodeIDs = []string{}
	}
	writeJSON(w, http.StatusOK, applicationprofiles.BuildDetail(applicationprofiles.DetailInput{
		Summary: applicationprofiles.SummaryInput{
			ID:                      cfg.ID,
			Name:                    cfg.Name,
			Type:                    cfg.Type,
			State:                   cfg.State,
			ProfileIdentifier:       cfg.ProfileIdentifier,
			CurrentNodeID:           cfg.CurrentNodeID,
			CurrentExitNodeID:       cfg.CurrentExitNodeID,
			NodeSourceMode:          cfg.NodeSourceMode,
			SourceIDs:               sourceIDs,
			EgressCountry:           cfg.EgressCountry,
			EgressCountryMode:       cfg.EgressCountryMode,
			EgressCountries:         egressCountries,
			NameIncludeRegex:        cfg.NameIncludeRegex,
			NameExcludeRegex:        cfg.NameExcludeRegex,
			CandidateLimit:          cfg.CandidateLimit,
			MinEvaluationInterval:   cfg.MinEvalInterval,
			AutoEvaluationEnabled:   cfg.AutoEvalEnabled,
			AutoEvaluationInterval:  cfg.AutoEvalInterval,
			NodeStickyEnabled:       cfg.NodeStickyEnabled,
			ConfigVersion:           cfg.ConfigVersion,
			CurrentPath:             g.accessProfileCurrentPath(cfg),
			ProxyCredentialsCount:   totalCreds,
			EnabledCredentialsCount: enabledCreds,
			LastEvaluatedAt:         cfg.LastEvaluatedAt,
			LastError:               cfg.LastError,
			SwitchReason:            cfg.SwitchReason,
		},
		FixedNodeID:                  cfg.FixedNodeID,
		ExitNodeIDs:                  exitNodeIDs,
		ChainEvaluationMode:          cfg.ChainEvaluationMode,
		TestURL:                      cfg.TestURL,
		CurrentPathLatencyMS:         cfg.CurrentPathLatencyMS,
		LastEvaluationDetails:        parseJSONObject(cfg.LastEvaluationDetailsJSON),
		ProxyCredentials:             credentials,
		CandidateStats:               candidateStats,
		RecentEvents:                 recentEvents,
		CandidateFilterSourceMode:    apiNodeSourceMode(cfg.NodeSourceMode),
		SourceIDs:                    sourceIDs,
		Protocols:                    protocols,
		EgressCountries:              egressCountries,
		RelativeImprovementThreshold: cfg.RelativeImprovementThreshold,
		AbsoluteLatencyImprovementMS: cfg.AbsoluteLatencyImprovementMS,
	}))
}

func parseJSONObject(raw string) map[string]any {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
		return map[string]any{}
	}
	return value
}

func (g *Gateway) proxyEndpoint(r *http.Request) string {
	ep := strings.TrimSpace(g.getKVSetting("public_proxy_endpoint"))
	if ep != "" {
		return ep
	}
	host := r.Host
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		if h, p, err := net.SplitHostPort(host); err == nil {
			if strings.Contains(h, ":") {
				return "[" + h + "]:" + p
			}
		}
	}
	return host
}

func proxyURLs(identifier, password, endpoint string) (httpURL, httpsURL, socks5URL string) {
	return "http://" + identifier + ":" + password + "@" + endpoint,
		"https://" + identifier + ":" + password + "@" + endpoint,
		"socks5://" + identifier + ":" + password + "@" + endpoint
}

func validateProfileIdentifier(s string) error {
	err := domainprofile.ValidateIdentifier(s)
	if errors.Is(err, domainprofile.ErrIdentifierLength) {
		return errors.New(validationProfileIdentifierLength)
	}
	if errors.Is(err, domainprofile.ErrIdentifierCharset) {
		return errors.New(validationProfileIdentifierCharset)
	}
	return err
}

func (g *Gateway) handleAccessProfileAction(w http.ResponseWriter, r *http.Request, profileID string, action string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg, err := g.loadAccessProfileConfig(profileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "access profile not found")
		return
	}
	switch action {
	case "evaluate":
		if !profileTypeNeedsEvaluation(cfg.Type) {
			writeError(w, http.StatusBadRequest, "profile_type_not_evaluable")
			return
		}
		runID, err := g.enqueueProfileEvaluationRun(profileID, cfg.Name, "manual", cfg.ConfigVersion, true)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "enqueue evaluation")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"run_id": runID, "state": "queued"})
	case "switch-to-best-observed":
		if cfg.CurrentNodeID == "" {
			writeError(w, http.StatusConflict, "no current path to switch from")
			return
		}
		run, err := g.createMaintenanceRun(maintenanceRunTypeProfileSwitch, "manual", profileID, cfg.Name, 1, map[string]any{
			"profile_id":     profileID,
			"config_version": cfg.ConfigVersion,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create profile switch run")
			return
		}
		detail := run.detail()
		detail["switch_reason"] = "manual_switch_requested"
		if err := g.finishMaintenanceRun(run.ID, maintenanceRunResultSuccess, "manual_switch_requested", 1, detail, ""); err != nil {
			writeError(w, http.StatusInternalServerError, "finish profile switch run")
			return
		}
		_, _ = g.enqueueProfileEvaluationRun(profileID, cfg.Name, "manual", cfg.ConfigVersion, true)
		writeJSON(w, http.StatusOK, map[string]any{"run_id": run.ID, "state": "finished"})
	default:
		writeError(w, http.StatusNotFound, "unknown action")
	}
}
