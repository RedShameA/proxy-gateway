package app

import (
	"context"
	"errors"
	"strings"

	apperrors "proxygateway/internal/application/apperrors"
	maintenanceapp "proxygateway/internal/application/maintenance"
	applicationnodes "proxygateway/internal/application/nodes"
	appsubscriptions "proxygateway/internal/application/subscriptions"
)

var (
	errNodeNotFound         = errors.New("node not found")
	errManualNodeSourceMiss = errors.New("manual node source not found")
	errDuplicateNode        = errors.New("duplicate node")
)

type nodeCreateInput struct {
	Name       string
	Type       string
	Server     string
	ServerPort int
	Username   string
	Password   string
	RawJSON    string
	ImportText string
}

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

func newNodeOperationError(kind, message string, err error) error {
	return apperrors.New(kind, message, err)
}

func (g *Gateway) createNodeSource(input nodeCreateInput) (map[string]any, error) {
	if strings.TrimSpace(input.ImportText) != "" {
		result, err := g.importManualNodes(input.ImportText)
		if err != nil {
			return nil, newNodeOperationError(apperrors.KindBadRequest, err.Error(), err)
		}
		return result, nil
	}
	nodeName := strings.TrimSpace(input.Name)
	id, err := g.nodeManagementService().CreateManual(context.Background(), applicationnodes.OutboundNode{
		Name:       nodeName,
		Type:       input.Type,
		Server:     input.Server,
		ServerPort: input.ServerPort,
		Username:   input.Username,
		Password:   input.Password,
		RawJSON:    input.RawJSON,
	})
	if err != nil {
		return nil, newNodeOperationError(apperrors.KindInternal, "create node", err)
	}
	g.enqueueNodeObservationForManualImport(id, nodeName)
	return map[string]any{"id": id}, nil
}

func (g *Gateway) listNodes(filter applicationnodes.ListFilter) (map[string]any, error) {
	return g.nodeManagementService().List(context.Background(), filter)
}

func (g *Gateway) nodeDetail(nodeID string) (map[string]any, error) {
	detail, err := g.nodeManagementService().Detail(context.Background(), nodeID)
	if err != nil {
		return nil, newNodeOperationError(apperrors.KindNotFound, "node not found", err)
	}
	return detail, nil
}

func (g *Gateway) patchNodeSource(nodeID string, req nodePatchRequest) (map[string]any, error) {
	if req.hasManualNodeFields() {
		updatedNodeID, split, err := g.updateManualNode(nodeID, req)
		if err != nil {
			return nil, nodeErrorFromUpdate(err)
		}
		return map[string]any{"updated": true, "id": updatedNodeID, "split": split}, nil
	}
	result, err := g.nodeManagementService().SetEnabled(context.Background(), nodeID, *req.Enabled)
	if errors.Is(err, applicationnodes.ErrNodeNotFound) {
		return nil, newNodeOperationError(apperrors.KindNotFound, "node not found", errNodeNotFound)
	}
	if err != nil {
		return nil, newNodeOperationError(apperrors.KindInternal, "update node", err)
	}
	if result.RuntimeFingerprint != "" {
		g.invalidateRuntimeFingerprint(result.RuntimeFingerprint)
	}
	return map[string]any{"updated": true, "id": nodeID, "split": false}, nil
}

func nodeErrorFromUpdate(err error) error {
	switch {
	case errors.Is(err, errNodeNotFound):
		return newNodeOperationError(apperrors.KindNotFound, "node not found", err)
	case errors.Is(err, errManualNodeSourceMiss):
		return newNodeOperationError(apperrors.KindBadRequest, "manual node source not found", err)
	case errors.Is(err, errDuplicateNode):
		return newNodeOperationError(apperrors.KindConflict, "duplicate node", err)
	default:
		return newNodeOperationError(apperrors.KindBadRequest, err.Error(), err)
	}
}

func (g *Gateway) importManualNodes(importText string) (map[string]any, error) {
	result, err := appsubscriptions.ManualNodeImportService{
		Runner: g.txRunners,
		NewNodeID: func() (string, error) {
			return prefixedID("node")
		},
	}.Import(context.Background(), appsubscriptions.ManualNodeImportCommand{
		ImportText: importText,
		NowMillis:  unixMillisNow(),
	})
	if errors.Is(err, appsubscriptions.ErrManualImportRequired) {
		return nil, errors.New(validationNodeImportRequired)
	}
	if errors.Is(err, appsubscriptions.ErrNoImportableNodeFound) {
		return nil, errors.New("no importable node found")
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		ids = append(ids, node.ID)
		g.enqueueNodeObservationForManualImport(node.ID, node.Name)
	}
	return map[string]any{
		"id":                    ids[0],
		"ids":                   ids,
		"imported_nodes":        len(ids),
		"skipped_entries":       result.SkippedEntries,
		"skipped_entry_summary": result.SkippedEntrySummary,
		"parse_error":           "",
	}, nil
}

func parseManualNodeImport(importText string) ([]parsedSubscriptionNode, skippedEntrySummarySet, error) {
	nodes, skippedSummary, err := appsubscriptions.ParseManualNodeImport(importText)
	if errors.Is(err, appsubscriptions.ErrManualImportRequired) {
		return nil, skippedEntrySummarySet{}, errors.New(validationNodeImportRequired)
	}
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
	_, _ = g.createNodeObservationRun(maintenanceapp.TriggerManualNodeImport, maintenanceapp.NodeObservationScopeSingleNode, []nodeRecord{{ID: nodeID, Name: nodeName, Enabled: true}}, probeURL)
	g.notifyMaintenanceRunner()
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
	node, err := applicationnodes.BuildStructuredManualNode(applicationnodes.StructuredManualInput{
		Name:       req.Name,
		Type:       req.Type,
		Server:     req.Server,
		ServerPort: req.ServerPort,
		Username:   req.Username,
		Password:   req.Password,
	})
	if err != nil {
		return parsedSubscriptionNode{}, manualNodeValidationError(err)
	}
	return parsedSubscriptionNodeFromOutboundNode(node), nil
}

func (g *Gateway) updateManualNode(nodeID string, req nodePatchRequest) (string, bool, error) {
	node, err := req.manualNode()
	if err != nil {
		return "", false, err
	}
	result, err := g.nodeManagementService().UpdateManual(context.Background(), applicationnodes.ManualUpdateCommand{
		NodeID:  nodeID,
		Node:    parsedSubscriptionNodeToOutboundNode(node),
		Enabled: req.Enabled,
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
	g.invalidateRuntimeFingerprints(result.InvalidatedFingerprints)
	g.enqueueNodeObservationForManualImport(result.NodeID, node.Name)
	return result.NodeID, result.Split, nil
}

func (g *Gateway) deleteManualNodeSource(nodeID string) error {
	result, err := g.nodeManagementService().DeleteManualSource(context.Background(), nodeID)
	if errors.Is(err, applicationnodes.ErrManualNodeSourceMissing) {
		return errors.New("manual node source not found")
	}
	if err != nil {
		return err
	}
	g.invalidateRuntimeFingerprints(result.DeletedFingerprints)
	g.notifyMaintenanceRunner()
	return nil
}

func (g *Gateway) loadNode(id string) (nodeRecord, error) {
	return g.loadNodeWithContext(context.Background(), id)
}

func (g *Gateway) loadNodeWithContext(ctx context.Context, id string) (nodeRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	record, found, err := g.nodeRepo.Load(ctx, id)
	if err != nil {
		return nodeRecord{}, err
	}
	if !found {
		return nodeRecord{}, applicationnodes.ErrNodeNotFound
	}
	return nodeRecordFromApplication(record), nil
}

func nodeRecordFromApplication(record applicationnodes.Record) nodeRecord {
	return nodeRecord{
		ID:           record.ID,
		Name:         record.Name,
		Type:         record.Type,
		Server:       record.Server,
		ServerPort:   record.ServerPort,
		Username:     record.Username,
		Password:     record.Password,
		RawJSON:      record.RawJSON,
		OutboundJSON: record.OutboundJSON,
		Enabled:      record.Enabled,
	}
}

func nodeRecordToApplication(record nodeRecord) applicationnodes.Record {
	return applicationnodes.Record{
		ID:           record.ID,
		Name:         record.Name,
		Type:         record.Type,
		Server:       record.Server,
		ServerPort:   record.ServerPort,
		Username:     record.Username,
		Password:     record.Password,
		RawJSON:      record.RawJSON,
		OutboundJSON: record.OutboundJSON,
		Enabled:      record.Enabled,
	}
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
	return applicationnodes.RuntimeFingerprint(nodeRecordToApplication(node))
}

func parsedSubscriptionNodeToOutboundNode(node parsedSubscriptionNode) applicationnodes.OutboundNode {
	return applicationnodes.OutboundNode{
		Name:          node.Name,
		Type:          node.Type,
		Server:        node.Server,
		ServerPort:    node.ServerPort,
		Method:        node.Method,
		UUID:          node.UUID,
		Flow:          node.Flow,
		Security:      node.Security,
		AlterID:       node.AlterID,
		TLSJSON:       node.TLSJSON,
		TransportJSON: node.TransportJSON,
		Username:      node.Username,
		Password:      node.Password,
		RawJSON:       node.RawJSON,
		OutboundJSON:  node.OutboundJSON,
	}
}

func parsedSubscriptionNodeFromOutboundNode(node applicationnodes.OutboundNode) parsedSubscriptionNode {
	return parsedSubscriptionNode{
		Name:          node.Name,
		Type:          node.Type,
		Server:        node.Server,
		ServerPort:    node.ServerPort,
		Method:        node.Method,
		UUID:          node.UUID,
		Flow:          node.Flow,
		Security:      node.Security,
		AlterID:       node.AlterID,
		TLSJSON:       node.TLSJSON,
		TransportJSON: node.TransportJSON,
		Username:      node.Username,
		Password:      node.Password,
		RawJSON:       node.RawJSON,
		OutboundJSON:  node.OutboundJSON,
	}
}

func manualNodeValidationError(err error) error {
	switch {
	case errors.Is(err, applicationnodes.ErrManualNodeFieldsRequired):
		return errors.New(validationNodeManualFieldsRequired)
	case errors.Is(err, applicationnodes.ErrManualNodeNameRequired):
		return errors.New(validationNodeNameRequired)
	case errors.Is(err, applicationnodes.ErrManualNodeTypeUnsupported):
		return errors.New(validationNodeTypeSupported)
	case errors.Is(err, applicationnodes.ErrManualNodeEndpointInvalid):
		return errors.New(validationNodeEndpointRequired)
	default:
		return err
	}
}
