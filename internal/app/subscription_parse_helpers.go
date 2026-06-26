package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyValue(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(anyString(value)); text != "" {
			return text
		}
		if list := stringListValue(value); len(list) > 0 {
			return list[0]
		}
	}
	return ""
}

func tryBase64Text(text string) (string, bool) {
	compact := strings.Join(strings.Fields(text), "")
	if compact == "" {
		return "", false
	}
	decoders := []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding}
	for _, enc := range decoders {
		raw, err := enc.DecodeString(compact)
		if err != nil {
			continue
		}
		decoded := strings.TrimSpace(string(raw))
		if decoded == "" {
			continue
		}
		if strings.Contains(decoded, "://") || strings.HasPrefix(decoded, "{") || strings.HasPrefix(decoded, "[") || strings.Contains(decoded, "proxies:") {
			return decoded, true
		}
	}
	return "", false
}

func mustMarshalRawMessage(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return raw
}

func decodeBase64Component(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	decoders := []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding}
	for _, enc := range decoders {
		raw, err := enc.DecodeString(text)
		if err != nil {
			continue
		}
		decoded := strings.TrimSpace(string(raw))
		if decoded != "" {
			return decoded, true
		}
	}
	return "", false
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
	case string:
		n, _ := strconv.Atoi(x)
		return n
	default:
		return 0
	}
}

func anyMap(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	return nil, false
}

func anyBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes", "on", "tls":
			return true, true
		case "0", "false", "no", "off", "none", "":
			return false, true
		default:
			return false, false
		}
	case int:
		return x != 0, true
	case int64:
		return x != 0, true
	case float64:
		return x != 0, true
	default:
		return false, false
	}
}

func mapString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := strings.TrimSpace(anyString(m[key])); text != "" {
			return text
		}
	}
	return ""
}

func mapBool(m map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if parsed, ok := anyBool(value); ok {
				return parsed, true
			}
		}
	}
	return false, false
}

func mapAnyMap(m map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if value, ok := anyMap(m[key]); ok {
			return value, true
		}
	}
	return nil, false
}

func stringListValue(value any) []string {
	switch x := value.(type) {
	case []string:
		return normalizeStringList(x)
	case []any:
		items := make([]string, 0, len(x))
		for _, item := range x {
			if text := strings.TrimSpace(anyString(item)); text != "" {
				items = append(items, text)
			}
		}
		return items
	case string:
		return splitCommaList(x)
	default:
		return nil
	}
}

func mapStringList(m map[string]any, keys ...string) []string {
	for _, key := range keys {
		if values := stringListValue(m[key]); len(values) > 0 {
			return values
		}
	}
	return nil
}

func splitCommaList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := strings.TrimSpace(part); text != "" {
			out = append(out, text)
		}
	}
	return out
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

func normalizeShadowsocksMethod(raw string) string {
	method := strings.TrimSpace(raw)
	if method == "" {
		return ""
	}
	upper := strings.ToUpper(strings.ReplaceAll(method, "-", "_"))
	switch upper {
	case "AEAD_CHACHA20_POLY1305":
		return "chacha20-ietf-poly1305"
	case "AEAD_AES_128_GCM":
		return "aes-128-gcm"
	case "AEAD_AES_192_GCM":
		return "aes-192-gcm"
	case "AEAD_AES_256_GCM":
		return "aes-256-gcm"
	default:
		if strings.HasPrefix(upper, "AEAD_") {
			return strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(upper, "AEAD_"), "_", "-"))
		}
		return strings.ToLower(method)
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

func subscriptionNodeTypeSupported(t string) bool {
	switch t {
	case "http", "socks5", "shadowsocks", "vmess",
		"trojan", "naive", "wireguard", "hysteria", "shadowtls", "vless", "tuic", "hysteria2", "anytls", "tor", "ssh":
		return true
	default:
		return false
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

func isFunctionalOutboundType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "block", "dns", "selector", "urltest":
		return true
	default:
		return false
	}
}

func truthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "tls":
			return true
		default:
			return false
		}
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	default:
		return false
	}
}
