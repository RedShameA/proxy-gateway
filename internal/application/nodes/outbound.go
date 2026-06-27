package nodes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	validationNodeEndpointRequired       = "节点服务器不能为空，端口需为 1-65535"
	validationNodeProtocolFieldsRequired = "协议必填字段不完整"
)

type OutboundNode struct {
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

func OutboundFingerprint(outboundJSON string) string {
	sum := sha256.Sum256([]byte(outboundJSON))
	return hex.EncodeToString(sum[:])
}

func NormalizeNodeType(t string) string {
	return normalizeNodeType(t)
}

func NormalizeOutboundJSON(n OutboundNode) (string, error) {
	if strings.TrimSpace(n.OutboundJSON) != "" {
		return CanonicalOutboundJSON(n.OutboundJSON)
	}
	nodeType := normalizeNodeType(n.Type)
	outboundType, err := SingBoxOutboundType(nodeType)
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
	return CanonicalOutboundJSON(string(raw))
}

func SingBoxOutboundType(nodeType string) (string, error) {
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

func CanonicalOutboundJSON(text string) (string, error) {
	var value map[string]any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return "", err
	}
	if nodeType := normalizeNodeType(anyString(value["type"])); nodeType != "" {
		outboundType, err := SingBoxOutboundType(nodeType)
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

func normalizeNodeType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "socks5", "socks":
		return "socks5"
	case "http", "https":
		return "http"
	case "ss":
		return "shadowsocks"
	case "hy2":
		return "hysteria2"
	default:
		return strings.ToLower(strings.TrimSpace(t))
	}
}

func normalizeVLESSFlow(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none", "null":
		return ""
	case "xtls-rprx-vision":
		return "xtls-rprx-vision"
	default:
		return ""
	}
}

func missingProtocolRequiredField(nodeType, method, password, uuid string) bool {
	switch normalizeNodeType(nodeType) {
	case "shadowsocks":
		return strings.TrimSpace(method) == "" || password == ""
	case "vmess":
		return strings.TrimSpace(uuid) == ""
	default:
		return false
	}
}

func anyString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return ""
	}
}

func anyInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		i, _ := x.Int64()
		return int(i)
	default:
		return 0
	}
}
