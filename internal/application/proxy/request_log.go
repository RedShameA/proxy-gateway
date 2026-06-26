package proxy

import "encoding/json"

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
