package subscriptions

import (
	"encoding/json"
	"errors"
	"strings"

	"gopkg.in/yaml.v3"

	appnodes "proxygateway/internal/application/nodes"
)

type ParsedNode struct {
	Name          string
	Type          string
	Server        string
	ServerPort    int
	Method        string
	UUID          string
	Flow          string
	Security      string
	AlterID       int
	TLSJSON       json.RawMessage
	TransportJSON json.RawMessage
	Username      string
	Password      string
	RawJSON       string
	OutboundJSON  string
}

func ParseNodes(data []byte) ([]ParsedNode, SkippedEntrySummarySet, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil, errors.New("subscription is empty")
	}
	if decoded, ok := tryBase64Text(trimmed); ok {
		nodes, skippedSummary, err := ParseNodes([]byte(decoded))
		if err == nil {
			return nodes, skippedSummary, nil
		}
	}
	var rawOutbounds []json.RawMessage
	if strings.HasPrefix(trimmed, "{") {
		var obj struct {
			Outbounds   []json.RawMessage `json:"outbounds"`
			Proxies     []map[string]any  `json:"proxies"`
			ProxyGroups []map[string]any  `json:"proxy-groups"`
		}
		if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
			return nil, nil, err
		}
		if len(obj.Proxies) > 0 {
			nodes, skippedSummary := parseClashProxyMaps(obj.Proxies)
			addClashProxyGroupSkips(skippedSummary, obj.ProxyGroups)
			return deduplicateParsedNodes(nodes, skippedSummary), skippedSummary, nil
		}
		rawOutbounds = obj.Outbounds
	} else if looksLikeJSONArray(trimmed) {
		if err := json.Unmarshal([]byte(trimmed), &rawOutbounds); err != nil {
			return nil, nil, err
		}
	} else {
		if strings.Contains(trimmed, "proxies:") {
			var cfg struct {
				Proxies     []map[string]any `yaml:"proxies"`
				ProxyGroups []map[string]any `yaml:"proxy-groups"`
			}
			if err := yaml.Unmarshal([]byte(trimmed), &cfg); err != nil {
				return nil, nil, err
			}
			if len(cfg.Proxies) > 0 || len(cfg.ProxyGroups) > 0 {
				nodes, skippedSummary := parseClashProxyMaps(cfg.Proxies)
				addClashProxyGroupSkips(skippedSummary, cfg.ProxyGroups)
				return deduplicateParsedNodes(nodes, skippedSummary), skippedSummary, nil
			}
		}
		if nodes, skippedSummary, recognized := parseSurgeSubscription(trimmed); recognized {
			return deduplicateParsedNodes(nodes, skippedSummary), skippedSummary, nil
		}
		nodes, skippedSummary := parseURILines(trimmed)
		if len(nodes) > 0 || skippedSummary.count() > 0 {
			return deduplicateParsedNodes(nodes, skippedSummary), skippedSummary, nil
		}
		return nil, nil, errors.New("unsupported subscription format")
	}
	var nodes []ParsedNode
	skippedSummary := SkippedEntrySummarySet{}
	for _, raw := range rawOutbounds {
		node, reason, detail := parseSingBoxOutboundNodeWithDetail(raw)
		if reason != "" {
			skippedSummary.addDetail(reason, detail)
			continue
		}
		nodes = append(nodes, node)
	}
	return deduplicateParsedNodes(nodes, skippedSummary), skippedSummary, nil
}

func ParseSingBoxOutboundNode(raw json.RawMessage) (ParsedNode, string) {
	return parseSingBoxOutboundNode(raw)
}

func NormalizeNodeType(t string) string {
	return normalizeNodeType(t)
}

func NormalizeNodeOutboundJSON(n ParsedNode) (string, error) {
	return normalizedNodeOutboundJSON(n)
}

func OutboundFingerprint(outboundJSON string) string {
	return outboundFingerprint(outboundJSON)
}

func looksLikeJSONArray(text string) bool {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "[") {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(text, "["))
	return rest == "" || strings.HasPrefix(rest, "{") || strings.HasPrefix(rest, "]")
}

func addClashProxyGroupSkips(summary SkippedEntrySummarySet, groups []map[string]any) {
	for _, group := range groups {
		summary.addDetail(SkippedReasonClashProxyGroupIgnored, SkippedEntryDetail{
			Name:      strings.TrimSpace(anyString(group["name"])),
			EntryType: strings.TrimSpace(anyString(group["type"])),
		})
	}
}

func deduplicateParsedNodes(nodes []ParsedNode, skippedSummary SkippedEntrySummarySet) []ParsedNode {
	if len(nodes) < 2 {
		return nodes
	}
	seen := map[string]struct{}{}
	deduped := make([]ParsedNode, 0, len(nodes))
	for _, node := range nodes {
		outboundJSON, err := normalizedNodeOutboundJSON(node)
		if err != nil {
			skippedSummary.addDetail(SkippedReasonUnsupportedOption, SkippedEntryDetail{
				Name:      node.Name,
				EntryType: node.Type,
				Detail:    err.Error(),
			})
			continue
		}
		fingerprint := outboundFingerprint(outboundJSON)
		if _, ok := seen[fingerprint]; ok {
			skippedSummary.addDetail(SkippedReasonDuplicateNode, SkippedEntryDetail{
				Name:      node.Name,
				EntryType: node.Type,
			})
			continue
		}
		seen[fingerprint] = struct{}{}
		deduped = append(deduped, node)
	}
	return deduped
}

func normalizedNodeOutboundJSON(n ParsedNode) (string, error) {
	return appnodes.NormalizeOutboundJSON(appnodes.OutboundNode{
		Name:          n.Name,
		Type:          n.Type,
		Server:        n.Server,
		ServerPort:    n.ServerPort,
		Method:        n.Method,
		UUID:          n.UUID,
		Flow:          n.Flow,
		Security:      n.Security,
		AlterID:       n.AlterID,
		TLSJSON:       n.TLSJSON,
		TransportJSON: n.TransportJSON,
		Username:      n.Username,
		Password:      n.Password,
		RawJSON:       n.RawJSON,
		OutboundJSON:  n.OutboundJSON,
	})
}

func outboundFingerprint(outboundJSON string) string {
	return appnodes.OutboundFingerprint(outboundJSON)
}

func singBoxOutboundType(nodeType string) (string, error) {
	return appnodes.SingBoxOutboundType(nodeType)
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
