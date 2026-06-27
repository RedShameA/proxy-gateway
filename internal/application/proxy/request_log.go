package proxy

import (
	"context"
	"encoding/json"
	"strings"
)

type ProxyCredentialSnapshot struct {
	ID     string
	Remark string
}

type AccessProfileSnapshot struct {
	ID         string
	Name       string
	Identifier string
}

type NodeSnapshot struct {
	ID         string
	Name       string
	Protocol   string
	Server     string
	ServerPort int
}

type ProxyPathSnapshot struct {
	Node      NodeSnapshot
	FrontNode NodeSnapshot
	ExitNode  NodeSnapshot
}

type RequestLogStartInput struct {
	ID         string
	Timestamp  int64
	TargetHost string
	Credential ProxyCredentialSnapshot
	Profile    AccessProfileSnapshot
	Path       ProxyPathSnapshot
}

type RequestLogStartRecord struct {
	ID                      string
	Timestamp               int64
	ProxyCredentialID       string
	ProxyCredential         string
	AccessProfileID         string
	AccessProfile           string
	AccessProfileIdentifier string
	TargetHost              string
	ProxyPath               string
	ProxyPathJSON           string
}

type RequestLogFinishInput struct {
	ID           string
	Success      bool
	FailureStage string
	Error        string
	HTTPStatus   int
	DurationMS   int64
	IngressBytes int64
	EgressBytes  int64
}

type RequestLogFinishRecord struct {
	ID           string
	Success      bool
	FailureStage string
	Error        string
	HTTPStatus   int
	DurationMS   int64
	IngressBytes int64
	EgressBytes  int64
}

type RequestLogFailureInput struct {
	ID                string
	Timestamp         int64
	TargetHost        string
	ProfileIdentifier string
	FailureStage      string
	Error             string
	HTTPStatus        int
	DurationMS        int64
}

type RequestLogFailureRecord struct {
	ID                      string
	Timestamp               int64
	AccessProfile           string
	AccessProfileIdentifier string
	TargetHost              string
	FailureStage            string
	Error                   string
	HTTPStatus              int
	DurationMS              int64
}

type RequestLogEntry struct {
	ID                      string
	Timestamp               int64
	ProxyCredentialID       string
	ProxyCredential         string
	AccessProfileID         string
	AccessProfile           string
	AccessProfileIdentifier string
	TargetHost              string
	ProxyPath               string
	ProxyPathJSON           string
	State                   string
	Success                 *bool
	FailureStage            string
	Error                   string
	DurationMS              int64
	IngressBytes            int64
	EgressBytes             int64
	HTTPStatus              int
}

type RequestLogListFilter struct {
	AccessProfile string
	Credential    string
	NodeID        string
	Target        string
	State         string
	Result        string
	Page          int
	PageSize      int
}

type RequestLogListResult struct {
	Items    []RequestLogEntry
	Total    int
	Page     int
	PageSize int
}

type RequestLogCounts struct {
	Total  int
	Failed int
}

type RequestLogRepository interface {
	InsertStart(ctx context.Context, record RequestLogStartRecord) error
	Finish(ctx context.Context, record RequestLogFinishRecord) error
	InsertFailure(ctx context.Context, record RequestLogFailureRecord) error
	List(ctx context.Context, filter RequestLogListFilter) (RequestLogListResult, error)
	CountSince(ctx context.Context, cutoffMillis int64) (RequestLogCounts, error)
	ListRecentFailures(ctx context.Context, limit int) ([]RequestLogEntry, error)
	DeleteBefore(ctx context.Context, cutoffMillis int64) (int64, error)
}

func RequestLogEntryToMap(item RequestLogEntry, nowMillis int64) map[string]any {
	result, successValue := requestLogResult(item.State, item.Success)
	durationMS := item.DurationMS
	if item.State == "running" && durationMS <= 0 {
		durationMS = nowMillis - item.Timestamp
		if durationMS <= 0 {
			durationMS = 1
		}
	}
	var httpStatusPtr any = nil
	if item.HTTPStatus > 0 {
		httpStatusPtr = item.HTTPStatus
	}
	var credentialID any = item.ProxyCredentialID
	if item.ProxyCredentialID == "" {
		credentialID = nil
	}
	return map[string]any{
		"id":               item.ID,
		"occurred_at":      item.Timestamp,
		"access_profile":   map[string]any{"id": item.AccessProfileID, "name": item.AccessProfile, "profile_identifier": item.AccessProfileIdentifier},
		"proxy_credential": map[string]any{"id": credentialID, "remark": item.ProxyCredential},
		"target_host":      item.TargetHost,
		"target_port":      targetPortFromTarget(item.TargetHost),
		"target":           item.TargetHost,
		"proxy_path":       parseRequestLogProxyPath(item.ProxyPathJSON),
		"proxy_path_label": item.ProxyPath,
		"state":            item.State,
		"result":           result,
		"success":          successValue,
		"failure_stage":    item.FailureStage,
		"error":            item.Error,
		"duration_ms":      durationMS,
		"ingress_bytes":    item.IngressBytes,
		"egress_bytes":     item.EgressBytes,
		"http_status":      httpStatusPtr,
	}
}

func BuildRequestLogStart(input RequestLogStartInput) RequestLogStartRecord {
	return RequestLogStartRecord{
		ID:                      input.ID,
		Timestamp:               input.Timestamp,
		ProxyCredentialID:       input.Credential.ID,
		ProxyCredential:         input.Credential.Remark,
		AccessProfileID:         input.Profile.ID,
		AccessProfile:           input.Profile.Name,
		AccessProfileIdentifier: input.Profile.Identifier,
		TargetHost:              input.TargetHost,
		ProxyPath:               proxyPathLabel(input.Path),
		ProxyPathJSON:           proxyPathJSON(input.Path),
	}
}

func BuildRequestLogFinish(input RequestLogFinishInput) RequestLogFinishRecord {
	failureStage := input.FailureStage
	if input.Success {
		failureStage = ""
	}
	return RequestLogFinishRecord{
		ID:           input.ID,
		Success:      input.Success,
		FailureStage: failureStage,
		Error:        input.Error,
		HTTPStatus:   input.HTTPStatus,
		DurationMS:   input.DurationMS,
		IngressBytes: input.IngressBytes,
		EgressBytes:  input.EgressBytes,
	}
}

func BuildRequestLogFailure(input RequestLogFailureInput) RequestLogFailureRecord {
	return RequestLogFailureRecord{
		ID:                      input.ID,
		Timestamp:               input.Timestamp,
		AccessProfile:           input.ProfileIdentifier,
		AccessProfileIdentifier: input.ProfileIdentifier,
		TargetHost:              input.TargetHost,
		FailureStage:            input.FailureStage,
		Error:                   input.Error,
		HTTPStatus:              input.HTTPStatus,
		DurationMS:              input.DurationMS,
	}
}

func requestLogResult(state string, success *bool) (string, any) {
	if state == "running" {
		return "running", nil
	}
	if success != nil && *success {
		return "success", true
	}
	return "failure", false
}

func parseRequestLogProxyPath(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil
	}
	return value
}

func targetPortFromTarget(target string) int {
	if strings.HasSuffix(target, ":443") {
		return 443
	}
	if strings.HasSuffix(target, ":80") {
		return 80
	}
	return 0
}

func proxyPathLabel(path ProxyPathSnapshot) string {
	if path.FrontNode.ID != "" && path.ExitNode.ID != "" {
		return nodeLabel(path.FrontNode) + " -> " + nodeLabel(path.ExitNode)
	}
	return nodeLabel(path.Node)
}

func proxyPathJSON(path ProxyPathSnapshot) string {
	var value map[string]any
	if path.FrontNode.ID != "" && path.ExitNode.ID != "" {
		value = map[string]any{
			"path_type":             "chain",
			"front_node":            nodePathSummary(path.FrontNode),
			"exit_node":             nodePathSummary(path.ExitNode),
			"final_egress_country":  "__unknown__",
			"chain_evaluation_mode": "end_to_end",
			"latency_ms":            nil,
			"latency_kind":          nil,
			"evaluated_at":          nil,
		}
	} else if path.Node.ID != "" {
		value = map[string]any{
			"path_type":    "single",
			"node":         nodePathSummary(path.Node),
			"latency_ms":   nil,
			"latency_kind": nil,
			"evaluated_at": nil,
		}
	} else {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func nodePathSummary(node NodeSnapshot) map[string]any {
	return map[string]any{
		"id":                     node.ID,
		"name":                   node.Name,
		"protocol":               node.Protocol,
		"server":                 node.Server,
		"server_port":            node.ServerPort,
		"egress_ip":              nil,
		"egress_country":         "__unknown__",
		"observation_latency_ms": nil,
		"last_observed_at":       nil,
	}
}

func nodeLabel(node NodeSnapshot) string {
	if node.Name != "" {
		return node.Name
	}
	return node.ID
}
