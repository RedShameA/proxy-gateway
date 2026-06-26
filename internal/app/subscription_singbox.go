package app

import (
	"encoding/json"
	"strconv"
	"strings"
)

func parseSingBoxOutboundNode(raw json.RawMessage) (parsedSubscriptionNode, string) {
	node, reason, _ := parseSingBoxOutboundNodeWithDetail(raw)
	return node, reason
}

func parseSingBoxOutboundNodeWithDetail(raw json.RawMessage) (parsedSubscriptionNode, string, skippedEntryDetail) {
	var outbound map[string]any
	if err := json.Unmarshal(raw, &outbound); err != nil || len(outbound) == 0 {
		return parsedSubscriptionNode{}, skipReasonMalformedEntry, skippedEntryDetail{Detail: truncateSkippedDetail(string(raw))}
	}
	return parsedNodeFromSingBoxOutboundMap(outbound, raw)
}

func parsedNodeFromSingBoxOutboundMap(outbound map[string]any, raw json.RawMessage) (parsedSubscriptionNode, string, skippedEntryDetail) {
	outboundType := anyString(outbound["type"])
	detail := skippedEntryDetail{
		Name:      outboundNodeName(outbound, normalizeNodeType(outboundType), anyString(outbound["server"]), firstPositive(anyInt(outbound["server_port"]), anyInt(outbound["port"]))),
		EntryType: outboundType,
	}
	if isFunctionalOutboundType(outboundType) {
		return parsedSubscriptionNode{}, skipReasonUnsupportedFunctionalOutbound, detail
	}
	nodeType := normalizeNodeType(outboundType)
	if !subscriptionNodeTypeSupported(nodeType) {
		return parsedSubscriptionNode{}, skipReasonUnsupportedNodeType, detail
	}
	port := anyInt(outbound["server_port"])
	if port == 0 {
		port = anyInt(outbound["port"])
	}
	server := anyString(outbound["server"])
	if nodeType == "wireguard" && server == "" {
		server, port = wireGuardPeerEndpoint(outbound)
	}
	node := parsedSubscriptionNode{
		Name:          outboundNodeName(outbound, nodeType, server, port),
		Type:          nodeType,
		Server:        server,
		ServerPort:    port,
		Method:        anyString(outbound["method"]),
		UUID:          anyString(outbound["uuid"]),
		Flow:          anyString(outbound["flow"]),
		Security:      anyString(outbound["security"]),
		AlterID:       firstPositive(anyInt(outbound["alter_id"]), anyInt(outbound["alterId"])),
		TLSJSON:       outboundRawMessage(outbound, "tls"),
		TransportJSON: outboundRawMessage(outbound, "transport"),
		Username:      firstNonEmpty(anyString(outbound["username"]), anyString(outbound["user"])),
		Password:      anyString(outbound["password"]),
		RawJSON:       string(raw),
		OutboundJSON:  string(raw),
	}
	if unsupportedSingBoxOutboundOption(nodeType, outbound) {
		return parsedSubscriptionNode{}, skipReasonUnsupportedOption, skippedEntryDetail{Name: node.Name, EntryType: outboundType}
	}
	if missingSingBoxOutboundRequiredField(nodeType, outbound, node) {
		return parsedSubscriptionNode{}, skipReasonMissingRequiredField, skippedEntryDetail{Name: node.Name, EntryType: outboundType}
	}
	if _, err := normalizedNodeOutboundJSON(node); err != nil {
		return parsedSubscriptionNode{}, skipReasonUnsupportedOption, skippedEntryDetail{Name: node.Name, EntryType: outboundType, Detail: err.Error()}
	}
	return node, "", skippedEntryDetail{}
}

func parsedNodeFromSingBoxOutbound(outbound map[string]any) (parsedSubscriptionNode, string, skippedEntryDetail) {
	raw, err := json.Marshal(outbound)
	if err != nil {
		return parsedSubscriptionNode{}, skipReasonMalformedEntry, skippedEntryDetail{Detail: err.Error()}
	}
	return parsedNodeFromSingBoxOutboundMap(outbound, raw)
}

func outboundNodeName(outbound map[string]any, nodeType, server string, port int) string {
	name := strings.TrimSpace(anyString(outbound["tag"]))
	if name != "" {
		return name
	}
	if server != "" && port > 0 {
		return server + ":" + strconv.Itoa(port)
	}
	return nodeType
}

func outboundRawMessage(outbound map[string]any, key string) json.RawMessage {
	value, ok := outbound[key]
	if !ok || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return raw
}

func wireGuardPeerEndpoint(outbound map[string]any) (string, int) {
	if server := anyString(outbound["server"]); server != "" {
		return server, anyInt(outbound["server_port"])
	}
	peers, ok := outbound["peers"].([]any)
	if !ok || len(peers) == 0 {
		return "", 0
	}
	peer, ok := peers[0].(map[string]any)
	if !ok {
		return "", 0
	}
	if server := anyString(peer["server"]); server != "" {
		return server, anyInt(peer["server_port"])
	}
	return anyString(peer["address"]), anyInt(peer["port"])
}

func missingSingBoxOutboundRequiredField(nodeType string, outbound map[string]any, node parsedSubscriptionNode) bool {
	switch nodeType {
	case "tor":
		return strings.TrimSpace(anyString(outbound["executable_path"])) == ""
	case "wireguard":
		if strings.TrimSpace(anyString(outbound["private_key"])) == "" {
			return true
		}
		if !hasNonEmptyOutboundValue(outbound, "address") && !hasNonEmptyOutboundValue(outbound, "local_address") {
			return true
		}
		if hasNonEmptyOutboundValue(outbound, "peer_public_key") && node.Server != "" && node.ServerPort > 0 {
			return false
		}
		peers, ok := outbound["peers"].([]any)
		if !ok || len(peers) == 0 {
			return true
		}
		for _, item := range peers {
			peer, ok := item.(map[string]any)
			if !ok || anyString(peer["address"]) == "" || anyInt(peer["port"]) <= 0 || anyString(peer["public_key"]) == "" {
				return true
			}
		}
		return false
	case "hysteria":
		if node.Server == "" || node.ServerPort <= 0 {
			return true
		}
		authString := anyString(outbound["auth_str"])
		authBytes := anyString(outbound["auth"])
		return authString == "" && authBytes == ""
	case "naive", "ssh":
		if node.Server == "" || node.ServerPort <= 0 {
			return true
		}
		if node.Username == "" {
			return true
		}
		return node.Password == "" && !hasNonEmptyOutboundValue(outbound, "private_key") && !hasNonEmptyOutboundValue(outbound, "private_key_path")
	case "trojan", "shadowtls", "hysteria2", "anytls":
		return node.Server == "" || node.ServerPort <= 0 || node.Password == ""
	case "vless":
		return node.Server == "" || node.ServerPort <= 0 || node.UUID == ""
	case "tuic":
		return node.Server == "" || node.ServerPort <= 0 || node.UUID == "" || node.Password == ""
	default:
		return node.Server == "" || node.ServerPort <= 0 || missingProtocolRequiredField(nodeType, node.Method, node.Password, node.UUID)
	}
}

func unsupportedSingBoxOutboundOption(nodeType string, outbound map[string]any) bool {
	switch nodeType {
	case "tor":
		return strings.TrimSpace(anyString(outbound["detour"])) != ""
	default:
		return false
	}
}

func hasNonEmptyOutboundValue(outbound map[string]any, key string) bool {
	value, ok := outbound[key]
	if !ok || value == nil {
		return false
	}
	switch x := value.(type) {
	case string:
		return strings.TrimSpace(x) != ""
	case []any:
		return len(x) > 0
	default:
		return true
	}
}
