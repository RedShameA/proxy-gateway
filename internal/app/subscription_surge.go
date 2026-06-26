package app

import (
	"bufio"
	"strconv"
	"strings"
)

func parseSurgeSubscription(text string) ([]parsedSubscriptionNode, skippedEntrySummarySet, bool) {
	if !strings.Contains(strings.ToLower(text), "[proxy") {
		return nil, nil, false
	}
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var proxies []map[string]any
	inProxy := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.Contains(line, "]") {
			section := strings.ToLower(strings.TrimSpace(line[1:strings.Index(line, "]")]))
			inProxy = section == "proxy"
			continue
		}
		if !inProxy {
			continue
		}
		name, body, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if proxy, ok := parseSurgeProxyLine(strings.TrimSpace(name), strings.TrimSpace(body)); ok {
			proxies = append(proxies, proxy)
		}
	}
	nodes, summary := parseClashProxyMaps(proxies)
	return nodes, summary, true
}

func parseSurgeProxyLine(name, body string) (map[string]any, bool) {
	parts := splitCommaRespectQuotes(body)
	if len(parts) < 3 {
		return nil, false
	}
	proto := strings.ToLower(strings.TrimSpace(parts[0]))
	server := strings.TrimSpace(parts[1])
	port := anyInt(strings.TrimSpace(parts[2]))
	if server == "" || port <= 0 {
		return nil, false
	}
	options := parseKeyValueOptions(parts[3:])
	proxy := map[string]any{
		"name":   name,
		"server": server,
		"port":   port,
	}
	if skipVerify, ok := parseBoolString(optionValue(options, "skip-cert-verify")); ok {
		proxy["skip-cert-verify"] = skipVerify
	}
	if udp, ok := parseBoolString(optionValue(options, "udp-relay", "udp")); ok {
		proxy["udp"] = udp
	}
	if tfo, ok := parseBoolString(optionValue(options, "tfo", "fast-open")); ok {
		proxy["fast-open"] = tfo
	}

	switch proto {
	case "ss", "shadowsocks":
		method := optionValue(options, "encrypt-method", "method", "cipher")
		password := optionValue(options, "password")
		if method == "" || password == "" {
			return nil, false
		}
		proxy["type"] = "ss"
		proxy["cipher"] = method
		proxy["password"] = password
		copyOption(proxy, options, "obfs", "obfs")
		copyOption(proxy, options, "obfs-host", "obfs-host")
		copyOption(proxy, options, "plugin", "plugin")
		copyOption(proxy, options, "plugin-opts", "plugin-opts", "plugin_opts", "plugin-option")
	case "vmess", "vmess-aead":
		uuid := optionValue(options, "username", "uuid", "id")
		if uuid == "" {
			return nil, false
		}
		proxy["type"] = "vmess"
		proxy["uuid"] = uuid
		copyOption(proxy, options, "cipher", "encrypt-method", "method", "cipher")
		copyOption(proxy, options, "alterId", "alterid", "alter-id", "alter_id")
		copyOption(proxy, options, "sni", "sni", "servername", "peer")
		if tls, ok := parseBoolString(optionValue(options, "tls")); ok {
			proxy["tls"] = tls
		}
		applySurgeTransportOptions(proxy, options)
	case "vless":
		uuid := optionValue(options, "username", "uuid", "id")
		if uuid == "" {
			return nil, false
		}
		proxy["type"] = "vless"
		proxy["uuid"] = uuid
		copyOption(proxy, options, "flow", "flow")
		copyOption(proxy, options, "sni", "sni", "servername", "peer")
		if tls, ok := parseBoolString(optionValue(options, "tls")); ok {
			proxy["tls"] = tls
		}
		applySurgeTransportOptions(proxy, options)
	case "trojan":
		password := optionValue(options, "password")
		if password == "" {
			return nil, false
		}
		proxy["type"] = "trojan"
		proxy["password"] = password
		copyOption(proxy, options, "sni", "sni", "servername", "peer")
		if tls, ok := parseBoolString(optionValue(options, "tls")); ok {
			proxy["tls"] = tls
		}
		applySurgeTransportOptions(proxy, options)
	case "http", "https":
		proxy["type"] = "http"
		if proto == "https" {
			proxy["tls"] = true
		}
		copyOption(proxy, options, "username", "username")
		copyOption(proxy, options, "password", "password")
		copyOption(proxy, options, "sni", "sni", "servername", "peer")
	case "socks5", "socks":
		proxy["type"] = "socks5"
		copyOption(proxy, options, "username", "username")
		copyOption(proxy, options, "password", "password")
	case "hysteria", "hysteria2", "hy2", "tuic", "ssh", "wireguard", "wg":
		proxy["type"] = proto
		for key, value := range options {
			if value != "" {
				proxy[key] = value
			}
		}
		if proto == "hy2" {
			proxy["type"] = "hysteria2"
		}
		if proto == "wg" {
			proxy["type"] = "wireguard"
		}
	default:
		return nil, false
	}
	return proxy, true
}

func parseKeyValueOptions(parts []string) map[string]string {
	out := make(map[string]string, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			out[strings.ToLower(part)] = "true"
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" {
			out[key] = value
		}
	}
	return out
}

func optionValue(options map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(options[strings.ToLower(key)]); value != "" {
			return value
		}
	}
	return ""
}

func copyOption(proxy map[string]any, options map[string]string, target string, keys ...string) {
	if value := optionValue(options, keys...); value != "" {
		proxy[target] = value
	}
}

func applySurgeTransportOptions(proxy map[string]any, options map[string]string) {
	network := optionValue(options, "network")
	if ws, ok := parseBoolString(optionValue(options, "ws")); ok && ws {
		network = "ws"
	}
	if grpc, ok := parseBoolString(optionValue(options, "grpc")); ok && grpc {
		network = "grpc"
	}
	if h2, ok := parseBoolString(optionValue(options, "h2", "http2")); ok && h2 {
		network = "h2"
	}
	network = normalizeV2RayNetwork(network)
	if network == "" {
		return
	}
	proxy["network"] = network
	switch network {
	case "ws":
		opts := map[string]any{}
		if path := optionValue(options, "ws-path", "path", "wspath"); path != "" {
			opts["path"] = path
		}
		if host := optionValue(options, "host", "ws-host", "ws.host", "obfs-host"); host != "" {
			opts["headers"] = map[string]any{"Host": host}
		}
		if len(opts) > 0 {
			proxy["ws-opts"] = opts
		}
	case "grpc":
		if service := optionValue(options, "grpc-service-name", "service-name", "service_name"); service != "" {
			proxy["grpc-opts"] = map[string]any{"grpc-service-name": service}
		}
	case "h2", "http":
		opts := map[string]any{}
		if path := optionValue(options, "h2-path", "http-path", "path"); path != "" {
			opts["path"] = path
		}
		if host := optionValue(options, "h2-host", "http-host", "host"); host != "" {
			opts["host"] = splitCommaList(host)
		}
		if len(opts) > 0 {
			proxy["h2-opts"] = opts
		}
	}
}

func splitCommaRespectQuotes(input string) []string {
	var out []string
	var token strings.Builder
	var quote rune
	for _, r := range input {
		switch r {
		case '"', '\'':
			if quote == 0 {
				quote = r
			} else if quote == r {
				quote = 0
			}
			token.WriteRune(r)
		case ',':
			if quote != 0 {
				token.WriteRune(r)
				continue
			}
			out = append(out, strings.TrimSpace(token.String()))
			token.Reset()
		default:
			token.WriteRune(r)
		}
	}
	out = append(out, strings.TrimSpace(token.String()))
	return out
}

func parseBoolString(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			return n != 0, true
		}
		return false, false
	}
}
