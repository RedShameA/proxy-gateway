package app

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	applicationnodes "proxygateway/internal/application/nodes"
)

var (
	errNodeNotFound         = errors.New("node not found")
	errManualNodeSourceMiss = errors.New("manual node source not found")
	errDuplicateNode        = errors.New("duplicate node")
)

type nodePatchRequest struct {
	Enabled    *bool   `json:"enabled"`
	Name       *string `json:"name"`
	Type       *string `json:"type"`
	Server     *string `json:"server"`
	ServerPort *int    `json:"server_port"`
	Username   *string `json:"username"`
	Password   *string `json:"password"`
	ImportText *string `json:"import_text"`
}

func (g *Gateway) handleNodes(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name       string `json:"name"`
			Type       string `json:"type"`
			Server     string `json:"server"`
			ServerPort int    `json:"server_port"`
			Username   string `json:"username"`
			Password   string `json:"password"`
			RawJSON    string `json:"raw_json"`
			ImportText string `json:"import_text"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if strings.TrimSpace(req.ImportText) != "" {
			result, err := g.importManualNodes(req.ImportText)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
		if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Type) == "" {
			writeError(w, http.StatusBadRequest, validationNodeNameTypeRequired)
			return
		}
		if req.Type != "direct" && (strings.TrimSpace(req.Server) == "" || req.ServerPort <= 0 || req.ServerPort > 65535) {
			writeError(w, http.StatusBadRequest, validationNodeEndpointRequired)
			return
		}
		id, err := g.upsertNode(parsedSubscriptionNode{
			Name:       strings.TrimSpace(req.Name),
			Type:       normalizeNodeType(req.Type),
			Server:     req.Server,
			ServerPort: req.ServerPort,
			Username:   req.Username,
			Password:   req.Password,
			RawJSON:    req.RawJSON,
		}, "manual", "Manual", "manual")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create node")
			return
		}
		g.enqueueNodeObservationForManualImport(id, strings.TrimSpace(req.Name))
		writeJSON(w, http.StatusOK, map[string]string{"id": id})
	case http.MethodGet:
		page, pageSize := parsePagination(r)
		offset := (page - 1) * pageSize

		nodeIDs, total, err := g.listNodeIDs(r, pageSize, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list nodes")
			return
		}
		nodes := make([]map[string]any, 0, len(nodeIDs))
		for _, nodeID := range nodeIDs {
			item, err := g.nodeListItem(nodeID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "load node")
				return
			}
			nodes = append(nodes, item)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": nodes, "nodes": nodes, "total": total})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (g *Gateway) listNodeIDs(r *http.Request, pageSize, offset int) ([]string, int, error) {
	where, args := nodeListWhereClause(r)
	from := ` FROM nodes n LEFT JOIN node_observations o ON o.node_id = n.id`
	var total int
	if err := g.db.QueryRow(`SELECT COUNT(*)`+from+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := g.db.Query(
		`SELECT n.id`+from+where+` ORDER BY n.created_at, n.id LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, total, nil
}

func nodeListWhereClause(r *http.Request) (string, []any) {
	query := r.URL.Query()
	clauses := []string{`NOT (
		NOT EXISTS (SELECT 1 FROM node_sources s WHERE s.node_id = n.id)
		AND EXISTS (SELECT 1 FROM retained_profile_nodes rp WHERE rp.node_id = n.id)
	)`}
	var args []any
	if value := strings.ToLower(strings.TrimSpace(query.Get("name"))); value != "" {
		pattern := likeContainsPattern(value)
		clauses = append(clauses, `(LOWER(n.name) LIKE ? ESCAPE '\' OR EXISTS (
			SELECT 1 FROM node_sources s
			 WHERE s.node_id = n.id
			   AND (LOWER(s.source_name) LIKE ? ESCAPE '\' OR LOWER(s.display_name) LIKE ? ESCAPE '\')
		))`)
		args = append(args, pattern, pattern, pattern)
	}
	countryFilter := normalizeEgressCountryValue(query.Get("egress_country"))
	if countryFilter == "" {
		countryFilter = normalizeEgressCountryValue(query.Get("country"))
	}
	if countryFilter != "" {
		clauses = append(clauses, `CASE WHEN TRIM(COALESCE(o.egress_country, '')) = '' THEN '__unknown__' ELSE UPPER(o.egress_country) END = ?`)
		args = append(args, countryFilter)
	}
	protocolFilter := strings.ToLower(strings.TrimSpace(query.Get("protocol")))
	if protocolFilter != "" {
		clauses = append(clauses, `LOWER(n.type) = ?`)
		args = append(args, protocolFilter)
	}
	sourceIDFilter := strings.TrimSpace(query.Get("source_id"))
	if sourceIDFilter != "" {
		clauses = append(clauses, `EXISTS (SELECT 1 FROM node_sources s WHERE s.node_id = n.id AND s.source_id = ?)`)
		args = append(args, sourceIDFilter)
	}
	sourceTypeFilter := strings.TrimSpace(query.Get("source_type"))
	if sourceTypeFilter != "" {
		clauses = append(clauses, `EXISTS (SELECT 1 FROM node_sources s WHERE s.node_id = n.id AND s.source_type = ?)`)
		args = append(args, sourceTypeFilter)
	}
	usableFilter := strings.ToLower(strings.TrimSpace(query.Get("usable")))
	stateFilter := strings.ToLower(strings.TrimSpace(query.Get("state")))
	switch stateFilter {
	case "disabled":
		clauses = append(clauses, `n.enabled != 1`)
	case "pending_observation":
		clauses = append(clauses, `n.enabled = 1 AND o.node_id IS NULL`)
	case "usable":
		clauses = append(clauses, `n.enabled = 1 AND COALESCE(o.usable, 0) = 1`)
	case "unusable":
		clauses = append(clauses, `n.enabled = 1 AND o.node_id IS NOT NULL AND COALESCE(o.usable, 0) != 1`)
	}
	switch usableFilter {
	case "true", "1":
		clauses = append(clauses, `COALESCE(o.usable, 0) = 1`)
	case "false", "0":
		clauses = append(clauses, `COALESCE(o.usable, 0) != 1`)
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func likeContainsPattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(value) + "%"
}

func (g *Gateway) nodeListItem(nodeID string) (map[string]any, error) {
	node, err := g.loadNode(nodeID)
	if err != nil {
		return nil, err
	}
	observation := g.nodeObservation(node.ID)
	sources := g.nodeSources(node.ID)
	state := "pending_observation"
	if !node.Enabled {
		state = "disabled"
	} else if observation != nil {
		if usable, ok := observation["usable"].(bool); ok && usable {
			state = "usable"
		} else if ok && !usable {
			state = "unusable"
		}
	}
	var egressIP any = nil
	var egressCountry string
	var latencyMS any = nil
	var lastObservedAt any = nil
	var lastErr string
	if observation != nil {
		if ip, ok := observation["egress_ip"].(string); ok && ip != "" {
			egressIP = ip
		}
		if ec, ok := observation["egress_country"].(string); ok {
			egressCountry = ec
		}
		if lat, ok := observation["latency_ms"].(int64); ok && lat > 0 {
			latencyMS = lat
		}
		var observedAt int64
		if ls, ok := observation["last_success_at"].(int64); ok && ls > 0 {
			observedAt = ls
		}
		if lf, ok := observation["last_failure_at"].(int64); ok && lf > observedAt {
			observedAt = lf
		}
		if observedAt > 0 {
			lastObservedAt = observedAt
		}
		if le, ok := observation["last_error"].(string); ok {
			lastErr = le
		}
	}
	return map[string]any{
		"id":                     node.ID,
		"name":                   node.Name,
		"type":                   node.Type,
		"protocol":               node.Type,
		"server":                 node.Server,
		"server_port":            node.ServerPort,
		"username":               node.Username,
		"password":               node.Password,
		"enabled":                node.Enabled,
		"state":                  state,
		"sources":                sources,
		"observation":            observation,
		"egress_ip":              egressIP,
		"egress_country":         egressCountry,
		"observation_latency_ms": latencyMS,
		"last_observed_at":       lastObservedAt,
		"last_error":             lastErr,
	}, nil
}

func (g *Gateway) importManualNodes(importText string) (map[string]any, error) {
	nodes, skippedSummary, err := parseManualNodeImport(importText)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, errors.New("no importable node found")
	}
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		id, err := g.upsertNode(node, "manual", "Manual", "manual")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
		g.enqueueNodeObservationForManualImport(id, node.Name)
	}
	result := map[string]any{
		"id":                    ids[0],
		"ids":                   ids,
		"imported_nodes":        len(ids),
		"skipped_entries":       skippedSummary.count(),
		"skipped_entry_summary": skippedSummary.rows(),
		"parse_error":           "",
	}
	return result, nil
}

func parseManualNodeImport(importText string) ([]parsedSubscriptionNode, skippedEntrySummarySet, error) {
	text := strings.TrimSpace(importText)
	if text == "" {
		return nil, skippedEntrySummarySet{}, errors.New(validationNodeImportRequired)
	}
	if strings.HasPrefix(text, "{") {
		raw := json.RawMessage(text)
		node, reason := parseSingBoxOutboundNode(raw)
		if reason == "" {
			return []parsedSubscriptionNode{node}, skippedEntrySummarySet{}, nil
		}
	}
	nodes, skippedSummary, err := parseSubscriptionNodes([]byte(text))
	if err != nil {
		return nil, skippedEntrySummarySet{}, err
	}
	return nodes, skippedSummary, nil
}

func (g *Gateway) enqueueNodeObservationForManualImport(nodeID, nodeName string) {
	settings, _ := g.loadMaintenanceSettings()
	probeURL := settings.EgressIPProbeURL
	if probeURL == "" {
		probeURL = defaultEgressIPProbeURL
	}
	_, _ = g.createNodeObservationRun("manual_node_import", "single_node", []nodeRecord{{ID: nodeID, Name: nodeName, Enabled: true}}, probeURL)
	g.notifyMaintenanceRunner()
}

func (g *Gateway) handleNodeSubroutes(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	nodeID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	if nodeID == "" || strings.Contains(nodeID, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		node, err := g.loadNode(nodeID)
		if err != nil {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		sources := g.nodeSources(nodeID)
		observation := g.nodeObservation(nodeID)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          node.ID,
			"name":        node.Name,
			"type":        node.Type,
			"protocol":    node.Type,
			"server":      node.Server,
			"server_port": node.ServerPort,
			"username":    node.Username,
			"password":    node.Password,
			"enabled":     node.Enabled,
			"sources":     sources,
			"observation": observation,
			"raw_json":    node.RawJSON,
		})
	case http.MethodPatch:
		var req nodePatchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Enabled == nil && !req.hasManualNodeFields() {
			writeError(w, http.StatusBadRequest, "enabled or manual node fields are required")
			return
		}
		if req.hasManualNodeFields() {
			updatedNodeID, split, err := g.updateManualNode(nodeID, req)
			if err != nil {
				switch {
				case errors.Is(err, errNodeNotFound):
					writeError(w, http.StatusNotFound, "node not found")
				case errors.Is(err, errManualNodeSourceMiss):
					writeError(w, http.StatusBadRequest, "manual node source not found")
				case errors.Is(err, errDuplicateNode):
					writeError(w, http.StatusConflict, "duplicate node")
				default:
					writeError(w, http.StatusBadRequest, err.Error())
				}
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"updated": true, "id": updatedNodeID, "split": split})
			return
		}
		enabled := 0
		if *req.Enabled {
			enabled = 1
		}
		var oldFingerprint string
		if enabled == 0 {
			node, err := g.loadNode(nodeID)
			if err != nil {
				writeError(w, http.StatusNotFound, "node not found")
				return
			}
			oldFingerprint = runtimeFingerprintForNode(node)
		}
		res, err := g.db.Exec(`UPDATE nodes SET enabled = ? WHERE id = ?`, enabled, nodeID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update node")
			return
		}
		affected, err := res.RowsAffected()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update node")
			return
		}
		if affected == 0 {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		if oldFingerprint != "" {
			g.invalidateRuntimeFingerprint(oldFingerprint)
		}
		writeJSON(w, http.StatusOK, map[string]any{"updated": true, "id": nodeID, "split": false})
	case http.MethodDelete:
		if err := g.deleteManualNodeSource(nodeID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (req nodePatchRequest) hasManualNodeFields() bool {
	return req.ImportText != nil || req.hasStructuredManualNodeFields()
}

func (req nodePatchRequest) hasStructuredManualNodeFields() bool {
	return req.Name != nil || req.Type != nil || req.Server != nil || req.ServerPort != nil || req.Username != nil || req.Password != nil
}

func (req nodePatchRequest) manualNode() (parsedSubscriptionNode, error) {
	if req.ImportText != nil {
		if req.hasStructuredManualNodeFields() {
			return parsedSubscriptionNode{}, errors.New(validationNodeImportExclusive)
		}
		nodes, _, err := parseManualNodeImport(*req.ImportText)
		if err != nil {
			return parsedSubscriptionNode{}, err
		}
		if len(nodes) != 1 {
			return parsedSubscriptionNode{}, errors.New(validationNodeImportSingleRequired)
		}
		return nodes[0], nil
	}
	if req.Name == nil || req.Type == nil || req.Server == nil || req.ServerPort == nil {
		return parsedSubscriptionNode{}, errors.New(validationNodeManualFieldsRequired)
	}
	name := strings.TrimSpace(*req.Name)
	nodeType := normalizeNodeType(*req.Type)
	server := strings.TrimSpace(*req.Server)
	port := *req.ServerPort
	if name == "" {
		return parsedSubscriptionNode{}, errors.New(validationNodeNameRequired)
	}
	if nodeType != "http" && nodeType != "socks5" {
		return parsedSubscriptionNode{}, errors.New(validationNodeTypeSupported)
	}
	if server == "" || port <= 0 || port > 65535 {
		return parsedSubscriptionNode{}, errors.New(validationNodeEndpointRequired)
	}
	username := ""
	if req.Username != nil {
		username = *req.Username
	}
	password := ""
	if req.Password != nil {
		password = *req.Password
	}
	return parsedSubscriptionNode{
		Name:       name,
		Type:       nodeType,
		Server:     server,
		ServerPort: port,
		Username:   username,
		Password:   password,
	}, nil
}

func (g *Gateway) updateManualNode(nodeID string, req nodePatchRequest) (string, bool, error) {
	node, err := req.manualNode()
	if err != nil {
		return "", false, err
	}
	outboundJSON, err := normalizedNodeOutboundJSON(node)
	if err != nil {
		return "", false, err
	}
	fingerprint := outboundFingerprint(outboundJSON)
	updateService := applicationnodes.Service{
		NewNodeID: func() (string, error) {
			return prefixedID("node")
		},
	}
	tx, err := g.db.Begin()
	if err != nil {
		return "", false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	oldFingerprint, err := nodeRuntimeFingerprintTx(tx, nodeID)
	if err != nil {
		return "", false, err
	}
	result, err := updateService.UpdateManual(nodeManualUpdateRepositoryTx{nodeUpsertRepositoryTx: nodeUpsertRepositoryTx{tx: tx}}, applicationnodes.ManualUpdateInput{
		NodeID:       nodeID,
		Fingerprint:  fingerprint,
		Name:         node.Name,
		Type:         node.Type,
		Server:       node.Server,
		ServerPort:   node.ServerPort,
		Username:     node.Username,
		Password:     node.Password,
		RawJSON:      node.RawJSON,
		OutboundJSON: outboundJSON,
		Enabled:      req.Enabled,
		NowMillis:    unixMillisNow(),
	})
	if errors.Is(err, applicationnodes.ErrNodeNotFound) {
		return "", false, errNodeNotFound
	}
	if errors.Is(err, applicationnodes.ErrManualNodeSourceMissing) {
		return "", false, errManualNodeSourceMiss
	}
	if errors.Is(err, applicationnodes.ErrDuplicateNode) {
		return "", false, errDuplicateNode
	}
	if err != nil {
		return "", false, err
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	committed = true
	if !result.Split && oldFingerprint != fingerprint {
		g.invalidateRuntimeFingerprint(oldFingerprint)
	}
	g.enqueueNodeObservationForManualImport(result.NodeID, node.Name)
	return result.NodeID, result.Split, nil
}

func nodeEnabledTx(tx *sql.Tx, nodeID string) (int, error) {
	var enabled int
	err := tx.QueryRow(`SELECT enabled FROM nodes WHERE id = ?`, nodeID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errNodeNotFound
	}
	return enabled, err
}

func nodeFingerprintTx(tx *sql.Tx, nodeID string) (string, error) {
	var fingerprint string
	err := tx.QueryRow(`SELECT fingerprint FROM nodes WHERE id = ?`, nodeID).Scan(&fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNodeNotFound
	}
	return fingerprint, err
}

func nodeRuntimeFingerprintTx(tx *sql.Tx, nodeID string) (string, error) {
	var node nodeRecord
	var enabled int
	var storedFingerprint string
	err := tx.QueryRow(
		`SELECT id, name, type, server, server_port, username, password, outbound_json, enabled, fingerprint FROM nodes WHERE id = ?`,
		nodeID,
	).Scan(&node.ID, &node.Name, &node.Type, &node.Server, &node.ServerPort, &node.Username, &node.Password, &node.OutboundJSON, &enabled, &storedFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNodeNotFound
	}
	if err != nil {
		return "", err
	}
	node.Enabled = enabled != 0
	if fingerprint := runtimeFingerprintForNode(node); fingerprint != "" {
		return fingerprint, nil
	}
	return storedFingerprint, nil
}

func nodeSourceCountsTx(tx *sql.Tx, nodeID string) (int, int, error) {
	var manualSources, totalSources int
	err := tx.QueryRow(
		`SELECT COALESCE(SUM(CASE WHEN source_type = 'manual' THEN 1 ELSE 0 END), 0), COUNT(*)
		   FROM node_sources
		  WHERE node_id = ?`,
		nodeID,
	).Scan(&manualSources, &totalSources)
	return manualSources, totalSources, err
}

func (g *Gateway) deleteManualNodeSource(nodeID string) error {
	tx, err := g.db.Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	result, err := applicationnodes.DeleteManualSource(nodeDeleteRepositoryTx{tx: tx}, nodeID)
	if errors.Is(err, applicationnodes.ErrManualNodeSourceMissing) {
		return errors.New("manual node source not found")
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	g.invalidateRuntimeFingerprints(result.DeletedFingerprints)
	g.notifyMaintenanceRunner()
	return nil
}

func (g *Gateway) loadNode(id string) (nodeRecord, error) {
	var n nodeRecord
	var enabled int
	err := g.db.QueryRow(
		`SELECT id, name, type, server, server_port, username, password, raw_json, outbound_json, enabled FROM nodes WHERE id = ?`,
		id,
	).Scan(&n.ID, &n.Name, &n.Type, &n.Server, &n.ServerPort, &n.Username, &n.Password, &n.RawJSON, &n.OutboundJSON, &enabled)
	n.Enabled = enabled == 1
	if err != nil {
		return nodeRecord{}, err
	}
	return n, nil
}

func (g *Gateway) invalidateRuntimeFingerprint(fingerprint string) {
	if strings.TrimSpace(fingerprint) == "" {
		return
	}
	if engine, ok := g.protocolEngine.(closeableNodeProtocolEngine); ok {
		engine.InvalidateFingerprint(fingerprint)
	}
}

func (g *Gateway) invalidateRuntimeFingerprints(fingerprints []string) {
	for _, fingerprint := range fingerprints {
		g.invalidateRuntimeFingerprint(fingerprint)
	}
}

func runtimeFingerprintForNode(node nodeRecord) string {
	_, fingerprint, err := runtimeOutboundJSONForNode(node)
	if err == nil {
		return fingerprint
	}
	return ""
}

func (g *Gateway) upsertNode(n parsedSubscriptionNode, sourceID, sourceName, sourceType string) (string, error) {
	tx, err := g.db.Begin()
	if err != nil {
		return "", err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	id, err := g.upsertNodeTx(tx, n, sourceID, sourceName, sourceType)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	committed = true
	return id, nil
}

func (g *Gateway) upsertNodeTx(tx *sql.Tx, n parsedSubscriptionNode, sourceID, sourceName, sourceType string) (string, error) {
	n.Type = normalizeNodeType(n.Type)
	outboundJSON, err := normalizedNodeOutboundJSON(n)
	if err != nil {
		return "", err
	}
	fingerprint := outboundFingerprint(outboundJSON)
	service := applicationnodes.Service{
		NewNodeID: func() (string, error) {
			return prefixedID("node")
		},
	}
	return service.Upsert(nodeUpsertRepositoryTx{tx: tx}, applicationnodes.UpsertInput{
		Fingerprint:  fingerprint,
		Name:         n.Name,
		Type:         n.Type,
		Server:       n.Server,
		ServerPort:   n.ServerPort,
		Username:     n.Username,
		Password:     n.Password,
		RawJSON:      n.RawJSON,
		OutboundJSON: outboundJSON,
		SourceID:     sourceID,
		SourceName:   sourceName,
		SourceType:   sourceType,
		NowMillis:    unixMillisNow(),
	})
}

type nodeUpsertRepositoryTx struct {
	tx *sql.Tx
}

func (r nodeUpsertRepositoryTx) FindNodeIDByFingerprint(fingerprint string) (string, error) {
	var id string
	err := r.tx.QueryRow(`SELECT id FROM nodes WHERE fingerprint = ?`, fingerprint).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (r nodeUpsertRepositoryTx) CreateNode(record applicationnodes.CreateNodeRecord) error {
	_, err := r.tx.Exec(
		`INSERT INTO nodes (id, fingerprint, name, type, server, server_port, username, password, raw_json, outbound_json, source_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.Fingerprint,
		record.Name,
		record.Type,
		record.Server,
		record.ServerPort,
		record.Username,
		record.Password,
		record.RawJSON,
		record.OutboundJSON,
		record.SourceID,
		record.CreatedAt,
	)
	return err
}

func (r nodeUpsertRepositoryTx) BindNodeSource(record applicationnodes.BindSourceRecord) error {
	_, err := r.tx.Exec(
		`INSERT OR IGNORE INTO node_sources (node_id, source_id, source_name, source_type, display_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		record.NodeID,
		record.SourceID,
		record.SourceName,
		record.SourceType,
		record.DisplayName,
		record.CreatedAt,
	)
	return err
}

type nodeManualUpdateRepositoryTx struct {
	nodeUpsertRepositoryTx
}

func (r nodeManualUpdateRepositoryTx) CurrentNodeEnabled(nodeID string) (int, error) {
	return nodeEnabledTx(r.tx, nodeID)
}

func (r nodeManualUpdateRepositoryTx) NodeSourceCounts(nodeID string) (int, int, error) {
	return nodeSourceCountsTx(r.tx, nodeID)
}

func (r nodeManualUpdateRepositoryTx) FindOtherNodeIDByFingerprint(fingerprint, excludeNodeID string) (string, error) {
	var id string
	err := r.tx.QueryRow(`SELECT id FROM nodes WHERE fingerprint = ? AND id != ? LIMIT 1`, fingerprint, excludeNodeID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (r nodeManualUpdateRepositoryTx) UpdateNode(record applicationnodes.UpdateNodeRecord) error {
	_, err := r.tx.Exec(
		`UPDATE nodes
		    SET fingerprint = ?, name = ?, type = ?, server = ?, server_port = ?, username = ?, password = ?, raw_json = ?, outbound_json = ?, enabled = ?
		  WHERE id = ?`,
		record.Fingerprint,
		record.Name,
		record.Type,
		record.Server,
		record.ServerPort,
		record.Username,
		record.Password,
		record.RawJSON,
		record.OutboundJSON,
		record.Enabled,
		record.NodeID,
	)
	return err
}

func (r nodeManualUpdateRepositoryTx) UpdateManualSourceDisplayName(nodeID, name string) error {
	_, err := r.tx.Exec(`UPDATE node_sources SET display_name = ? WHERE node_id = ? AND source_type = 'manual'`, name, nodeID)
	return err
}

func (r nodeManualUpdateRepositoryTx) DeleteManualSource(nodeID string) error {
	_, err := r.tx.Exec(`DELETE FROM node_sources WHERE node_id = ? AND source_type = 'manual'`, nodeID)
	return err
}

func (r nodeManualUpdateRepositoryTx) SetNodeEnabled(nodeID string, enabled int) error {
	_, err := r.tx.Exec(`UPDATE nodes SET enabled = ? WHERE id = ?`, enabled, nodeID)
	return err
}

type nodeDeleteRepositoryTx struct {
	tx *sql.Tx
}

func (r nodeDeleteRepositoryTx) DeleteManualSource(nodeID string) (int64, error) {
	res, err := r.tx.Exec(`DELETE FROM node_sources WHERE node_id = ? AND source_type = 'manual'`, nodeID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r nodeDeleteRepositoryTx) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	return cleanupNodesWithoutReferencesTx(r.tx, nodeIDs)
}

func outboundFingerprint(outboundJSON string) string {
	sum := sha256.Sum256([]byte(outboundJSON))
	return hex.EncodeToString(sum[:])
}

func normalizedNodeOutboundJSON(n parsedSubscriptionNode) (string, error) {
	if strings.TrimSpace(n.OutboundJSON) != "" {
		return canonicalOutboundJSON(n.OutboundJSON)
	}
	nodeType := normalizeNodeType(n.Type)
	outboundType, err := singBoxOutboundType(nodeType)
	if err != nil {
		return "", err
	}
	outbound := struct {
		Type       string          `json:"type"`
		Server     string          `json:"server,omitempty"`
		ServerPort int             `json:"server_port,omitempty"`
		Method     string          `json:"method,omitempty"`
		UUID       string          `json:"uuid,omitempty"`
		Flow       string          `json:"flow,omitempty"`
		Security   string          `json:"security,omitempty"`
		AlterID    int             `json:"alter_id,omitempty"`
		Username   string          `json:"username,omitempty"`
		Password   string          `json:"password,omitempty"`
		TLS        json.RawMessage `json:"tls,omitempty"`
		Transport  json.RawMessage `json:"transport,omitempty"`
	}{
		Type: outboundType,
	}
	if nodeType != "direct" {
		outbound.Server = strings.TrimSpace(n.Server)
		outbound.ServerPort = n.ServerPort
		outbound.Method = strings.TrimSpace(n.Method)
		outbound.UUID = strings.TrimSpace(n.UUID)
		outbound.Flow = normalizeVLESSFlow(n.Flow)
		outbound.Security = strings.TrimSpace(n.Security)
		outbound.AlterID = n.AlterID
		outbound.Username = n.Username
		outbound.Password = n.Password
		outbound.TLS, err = canonicalRawMessage(n.TLSJSON)
		if err != nil {
			return "", err
		}
		outbound.Transport, err = canonicalRawMessage(n.TransportJSON)
		if err != nil {
			return "", err
		}
		if nodeType == "vmess" && outbound.Security == "" {
			outbound.Security = "auto"
		}
		if outbound.Server == "" || outbound.ServerPort <= 0 {
			return "", errors.New(validationNodeEndpointRequired)
		}
		if missingProtocolRequiredField(nodeType, outbound.Method, outbound.Password, outbound.UUID) {
			return "", errors.New(validationNodeProtocolFieldsRequired)
		}
	}
	raw, err := json.Marshal(outbound)
	if err != nil {
		return "", err
	}
	return canonicalOutboundJSON(string(raw))
}

func normalizedNodeUsesStructuredCanonical(nodeType string) bool {
	switch normalizeNodeType(nodeType) {
	case "direct", "http", "socks5", "shadowsocks", "vmess":
		return true
	default:
		return false
	}
}

func parsedNodeFromOutboundJSON(text string) (parsedSubscriptionNode, bool) {
	var outbound struct {
		Type       string          `json:"type"`
		Server     string          `json:"server"`
		ServerPort int             `json:"server_port"`
		Port       int             `json:"port"`
		Method     string          `json:"method"`
		UUID       string          `json:"uuid"`
		Flow       string          `json:"flow"`
		Security   string          `json:"security"`
		AlterID    int             `json:"alter_id"`
		AlterID2   int             `json:"alterId"`
		Username   string          `json:"username"`
		Password   string          `json:"password"`
		TLS        json.RawMessage `json:"tls"`
		Transport  json.RawMessage `json:"transport"`
	}
	if err := json.Unmarshal([]byte(text), &outbound); err != nil || strings.TrimSpace(outbound.Type) == "" {
		return parsedSubscriptionNode{}, false
	}
	port := outbound.ServerPort
	if port == 0 {
		port = outbound.Port
	}
	return parsedSubscriptionNode{
		Type:          normalizeNodeType(outbound.Type),
		Server:        outbound.Server,
		ServerPort:    port,
		Method:        outbound.Method,
		UUID:          outbound.UUID,
		Flow:          outbound.Flow,
		Security:      outbound.Security,
		AlterID:       firstPositive(outbound.AlterID, outbound.AlterID2),
		TLSJSON:       outbound.TLS,
		TransportJSON: outbound.Transport,
		Username:      outbound.Username,
		Password:      outbound.Password,
	}, true
}

func canonicalRawMessage(raw json.RawMessage) (json.RawMessage, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	canonical, err := canonicalJSON(string(raw))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}

func canonicalJSON(text string) (string, error) {
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return "", err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func canonicalOutboundJSON(text string) (string, error) {
	var value map[string]any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return "", err
	}
	if nodeType := normalizeNodeType(anyString(value["type"])); nodeType != "" {
		outboundType, err := singBoxOutboundType(nodeType)
		if err != nil {
			return "", err
		}
		value["type"] = outboundType
	}
	delete(value, "tag")
	if port := anyInt(value["server_port"]); port > 0 {
		value["server_port"] = port
	} else if port := anyInt(value["port"]); port > 0 {
		value["server_port"] = port
	}
	delete(value, "port")
	if normalizeNodeType(anyString(value["type"])) == "vmess" && anyInt(value["alter_id"]) == 0 {
		delete(value, "alter_id")
		delete(value, "alterId")
	}
	pruneEmptyOutboundValues(value)
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func pruneEmptyOutboundValues(value map[string]any) {
	for key, item := range value {
		switch v := item.(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				delete(value, key)
			}
		case map[string]any:
			pruneEmptyOutboundValues(v)
			if len(v) == 0 {
				delete(value, key)
			}
		case []any:
			if len(v) == 0 {
				delete(value, key)
			}
		}
	}
}

func singBoxOutboundType(nodeType string) (string, error) {
	switch normalizeNodeType(nodeType) {
	case "direct":
		return "direct", nil
	case "http":
		return "http", nil
	case "socks5":
		return "socks", nil
	case "shadowsocks":
		return "shadowsocks", nil
	case "vmess":
		return "vmess", nil
	case "trojan":
		return "trojan", nil
	case "naive":
		return "naive", nil
	case "wireguard":
		return "wireguard", nil
	case "hysteria":
		return "hysteria", nil
	case "shadowtls":
		return "shadowtls", nil
	case "vless":
		return "vless", nil
	case "tuic":
		return "tuic", nil
	case "hysteria2":
		return "hysteria2", nil
	case "anytls":
		return "anytls", nil
	case "tor":
		return "tor", nil
	case "ssh":
		return "ssh", nil
	default:
		return "", fmt.Errorf("unsupported node type %q", nodeType)
	}
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (g *Gateway) nodeSources(nodeID string) []map[string]any {
	rows, err := g.db.Query(
		`SELECT source_id, source_name, source_type, display_name FROM node_sources WHERE node_id = ? ORDER BY created_at, source_id`,
		nodeID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var sources []map[string]any
	for rows.Next() {
		var sourceID, sourceName, sourceType, displayName string
		if err := rows.Scan(&sourceID, &sourceName, &sourceType, &displayName); err != nil {
			return sources
		}
		sources = append(sources, map[string]any{
			"source_id":    sourceID,
			"source_name":  sourceName,
			"source_type":  sourceType,
			"display_name": displayName,
		})
	}
	return sources
}
