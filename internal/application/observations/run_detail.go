package observations

import "strings"

const (
	ScopeSingleNode = "single_node"
	ScopeAllNodes   = "all_nodes"
)

type RunCreatePlan struct {
	TargetID    string
	TargetLabel string
	TotalCount  int
	Detail      map[string]any
}

func BuildRunCreatePlan(scope string, targets []NodeTarget, probeURL string) RunCreatePlan {
	targetID, targetLabel := "", ""
	if scope == ScopeSingleNode && len(targets) == 1 {
		targetID = targets[0].ID
		targetLabel = targets[0].Name
	}
	return RunCreatePlan{
		TargetID:    targetID,
		TargetLabel: targetLabel,
		TotalCount:  len(targets),
		Detail: map[string]any{
			"target_scope": scope,
			"probe_url":    probeURL,
			"node_ids":     NodeIDsForTargets(targets),
		},
	}
}

func NodeIDsForTargets(targets []NodeTarget) []string {
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.ID)
	}
	return ids
}

func NodeIDsFromRunDetail(targetID string, detail map[string]any) []string {
	ids := stringSliceFromDetail(detail["node_ids"])
	if len(ids) == 0 && strings.TrimSpace(targetID) != "" {
		ids = []string{strings.TrimSpace(targetID)}
	}
	return ids
}

func EffectiveProbeURL(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringSliceFromDetail(value any) []string {
	switch typed := value.(type) {
	case []string:
		return normalizeStringList(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		return normalizeStringList(values)
	default:
		return []string{}
	}
}
