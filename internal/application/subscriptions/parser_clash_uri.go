package subscriptions

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func parseClashProxyMaps(proxies []map[string]any) ([]ParsedNode, SkippedEntrySummarySet) {
	nodes := make([]ParsedNode, 0, len(proxies))
	skippedSummary := SkippedEntrySummarySet{}
	for _, proxy := range proxies {
		nodeType := normalizeNodeType(anyString(proxy["type"]))
		detail := SkippedEntryDetail{
			Name:      clashEntryName(proxy),
			EntryType: anyString(proxy["type"]),
		}
		if !subscriptionNodeTypeSupported(nodeType) {
			skippedSummary.addDetail(skipReasonUnsupportedNodeType, detail)
			continue
		}
		outbound, ok := clashProxySingBoxOutbound(proxy)
		if !ok {
			skippedSummary.addDetail(skipReasonMissingRequiredField, detail)
			continue
		}
		node, reason, parsedDetail := parsedNodeFromSingBoxOutbound(outbound)
		if reason != "" {
			if parsedDetail.Name == "" {
				parsedDetail.Name = detail.Name
			}
			if parsedDetail.EntryType == "" {
				parsedDetail.EntryType = detail.EntryType
			}
			skippedSummary.addDetail(reason, parsedDetail)
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes, skippedSummary
}

func clashProxySingBoxOutbound(proxy map[string]any) (map[string]any, bool) {
	nodeType := normalizeNodeType(anyString(proxy["type"]))
	server := strings.TrimSpace(anyString(proxy["server"]))
	port := anyInt(proxy["port"])
	if server == "" || port <= 0 {
		return nil, false
	}
	name := strings.TrimSpace(anyString(proxy["name"]))
	if name == "" {
		name = server + ":" + strconv.Itoa(port)
	}
	outboundType, err := singBoxOutboundType(nodeType)
	if err != nil {
		return nil, false
	}
	outbound := map[string]any{
		"type":        outboundType,
		"tag":         name,
		"server":      server,
		"server_port": port,
	}

	switch nodeType {
	case "shadowsocks":
		method := normalizeShadowsocksMethod(firstNonEmpty(anyString(proxy["cipher"]), anyString(proxy["method"])))
		password := strings.TrimSpace(anyString(proxy["password"]))
		if method == "" || password == "" {
			return nil, false
		}
		outbound["method"] = method
		outbound["password"] = password
		applyShadowsocksPluginFromClash(outbound, proxy)
	case "vmess":
		uuid := strings.TrimSpace(anyString(proxy["uuid"]))
		if uuid == "" {
			return nil, false
		}
		outbound["uuid"] = uuid
		outbound["security"] = firstNonEmpty(strings.TrimSpace(anyString(proxy["cipher"])), strings.TrimSpace(anyString(proxy["security"])), "auto")
		outbound["alter_id"] = firstPositive(anyInt(proxy["alterId"]), anyInt(proxy["alter_id"]), anyInt(proxy["aid"]))
		if tls := clashTLSJSON(proxy); len(tls) > 0 {
			outbound["tls"] = rawJSONMap(tls)
		}
		if transport := clashTransportJSON(proxy); len(transport) > 0 {
			outbound["transport"] = rawJSONMap(transport)
		}
	case "vless":
		uuid := strings.TrimSpace(anyString(proxy["uuid"]))
		if uuid == "" {
			return nil, false
		}
		outbound["uuid"] = uuid
		if flow := normalizeVLESSFlow(anyString(proxy["flow"])); flow != "" {
			outbound["flow"] = flow
		}
		if tls := clashTLSJSON(proxy); len(tls) > 0 {
			outbound["tls"] = rawJSONMap(tls)
		}
		if transport := clashTransportJSON(proxy); len(transport) > 0 {
			outbound["transport"] = rawJSONMap(transport)
		}
	case "trojan":
		password := strings.TrimSpace(anyString(proxy["password"]))
		if password == "" {
			return nil, false
		}
		outbound["password"] = password
		tls := rawJSONMap(clashTLSJSON(proxy))
		if tls == nil {
			tls = map[string]any{"enabled": true}
		}
		if _, ok := tls["server_name"]; !ok {
			tls["server_name"] = server
		}
		outbound["tls"] = tls
		if transport := clashTransportJSON(proxy); len(transport) > 0 {
			outbound["transport"] = rawJSONMap(transport)
		}
	case "http", "socks5":
		if username := strings.TrimSpace(anyString(proxy["username"])); username != "" {
			outbound["username"] = username
		}
		if password := strings.TrimSpace(anyString(proxy["password"])); password != "" {
			outbound["password"] = password
		}
		if nodeType == "http" {
			if headers, ok := mapAnyMap(proxy, "headers"); ok && len(headers) > 0 {
				outbound["headers"] = headers
			}
			if tls := clashTLSJSON(proxy); len(tls) > 0 {
				outbound["tls"] = rawJSONMap(tls)
			}
		} else if udp, ok := mapBool(proxy, "udp"); ok && !udp {
			outbound["network"] = "tcp"
		}
	case "hysteria2":
		password := strings.TrimSpace(firstNonEmpty(anyString(proxy["password"]), anyString(proxy["auth"])))
		if password == "" {
			return nil, false
		}
		outbound["password"] = password
		outbound["tls"] = hysteriaLikeTLSFromClash(proxy, server)
		applyHysteria2FieldsFromClash(outbound, proxy)
	case "hysteria":
		auth := strings.TrimSpace(firstNonEmpty(anyString(proxy["auth-str"]), anyString(proxy["auth_str"]), anyString(proxy["auth"])))
		if auth == "" {
			return nil, false
		}
		outbound["auth_str"] = auth
		outbound["tls"] = hysteriaLikeTLSFromClash(proxy, server)
		applyHysteriaFieldsFromClash(outbound, proxy)
	case "tuic":
		uuid := strings.TrimSpace(anyString(proxy["uuid"]))
		password := strings.TrimSpace(anyString(proxy["password"]))
		if uuid == "" || password == "" {
			return nil, false
		}
		outbound["uuid"] = uuid
		outbound["password"] = password
		outbound["tls"] = hysteriaLikeTLSFromClash(proxy, server)
		applyTUICFieldsFromClash(outbound, proxy)
	case "anytls":
		password := strings.TrimSpace(anyString(proxy["password"]))
		if password == "" {
			return nil, false
		}
		outbound["password"] = password
		outbound["tls"] = hysteriaLikeTLSFromClash(proxy, server)
		if interval, ok := durationStringFromMap(proxy, "s", "idle-session-check-interval", "idle_session_check_interval"); ok {
			outbound["idle_session_check_interval"] = interval
		}
		if timeout, ok := durationStringFromMap(proxy, "s", "idle-session-timeout", "idle_session_timeout"); ok {
			outbound["idle_session_timeout"] = timeout
		}
		if minIdle := anyInt(firstNonNil(proxy["min-idle-session"], proxy["min_idle_session"])); minIdle > 0 {
			outbound["min_idle_session"] = minIdle
		}
	case "ssh":
		user := strings.TrimSpace(firstNonEmpty(anyString(proxy["user"]), anyString(proxy["username"])))
		if user == "" {
			return nil, false
		}
		outbound["user"] = user
		if password := strings.TrimSpace(anyString(proxy["password"])); password != "" {
			outbound["password"] = password
		}
		copyStringField(outbound, proxy, "private_key", "private-key", "private_key")
		copyStringField(outbound, proxy, "private_key_passphrase", "private-key-passphrase", "private_key_passphrase")
		if hostKey := mapStringList(proxy, "host-key", "host_key"); len(hostKey) > 0 {
			outbound["host_key"] = hostKey
		}
		if hostKeyAlgorithms := mapStringList(proxy, "host-key-algorithms", "host_key_algorithms"); len(hostKeyAlgorithms) > 0 {
			outbound["host_key_algorithms"] = hostKeyAlgorithms
		}
		copyStringField(outbound, proxy, "client_version", "client-version", "client_version")
	case "wireguard":
		if !applyWireGuardFieldsFromClash(outbound, proxy, server, port) {
			return nil, false
		}
	case "shadowtls":
		password := strings.TrimSpace(anyString(proxy["password"]))
		if password == "" {
			return nil, false
		}
		outbound["password"] = password
		if version := anyInt(proxy["version"]); version > 0 {
			outbound["version"] = version
		}
		if username := strings.TrimSpace(anyString(proxy["username"])); username != "" {
			outbound["username"] = username
		}
	case "naive":
		username := strings.TrimSpace(anyString(proxy["username"]))
		password := strings.TrimSpace(anyString(proxy["password"]))
		if username == "" || password == "" {
			return nil, false
		}
		outbound["username"] = username
		outbound["password"] = password
		if tls := clashTLSJSON(proxy); len(tls) > 0 {
			outbound["tls"] = rawJSONMap(tls)
		}
	default:
		return nil, false
	}
	applyClashDialFields(outbound, proxy)
	return outbound, true
}

func parseURILines(text string) ([]ParsedNode, SkippedEntrySummarySet) {
	var nodes []ParsedNode
	skippedSummary := SkippedEntrySummarySet{}
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		outbounds, recognized := uriLineSingBoxOutbounds(line)
		if !recognized {
			skippedSummary.addDetail(skipReasonMalformedEntry, SkippedEntryDetail{Detail: truncateSkippedDetail(line)})
			continue
		}
		if len(outbounds) == 0 {
			entryType := ""
			if u, err := url.Parse(line); err == nil {
				entryType = u.Scheme
			}
			skippedSummary.addDetail(skipReasonMissingRequiredField, SkippedEntryDetail{EntryType: entryType, Detail: truncateSkippedDetail(line)})
			continue
		}
		for _, outbound := range outbounds {
			node, reason, detail := parsedNodeFromSingBoxOutbound(outbound)
			if reason != "" {
				if detail.Detail == "" {
					detail.Detail = truncateSkippedDetail(line)
				}
				skippedSummary.addDetail(reason, detail)
				continue
			}
			node.RawJSON = line
			nodes = append(nodes, node)
		}
	}
	return nodes, skippedSummary
}

func uriLineSingBoxOutbounds(line string) ([]map[string]any, bool) {
	lower := strings.ToLower(strings.TrimSpace(line))
	switch {
	case strings.HasPrefix(lower, "vmess://"):
		if outbound, ok := vmessURISingBoxOutbound(line); ok {
			return []map[string]any{outbound}, true
		}
		return nil, true
	case strings.HasPrefix(lower, "vless://"):
		if outbound, ok := standardProxyURISingBoxOutbound(line); ok {
			return []map[string]any{outbound}, true
		}
		return nil, true
	case strings.HasPrefix(lower, "trojan://"):
		if outbound, ok := standardProxyURISingBoxOutbound(line); ok {
			return []map[string]any{outbound}, true
		}
		return nil, true
	case strings.HasPrefix(lower, "ss://"):
		if outbound, ok := shadowsocksURISingBoxOutbound(line); ok {
			return []map[string]any{outbound}, true
		}
		return nil, true
	case strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "hy2://"):
		if outbound, ok := standardProxyURISingBoxOutbound(line); ok {
			return []map[string]any{outbound}, true
		}
		return nil, true
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "socks5://"), strings.HasPrefix(lower, "socks5h://"), strings.HasPrefix(lower, "socks://"):
		if outbound, ok := httpSocksURISingBoxOutbound(line); ok {
			return []map[string]any{outbound}, true
		}
		return nil, true
	default:
		if outbound, ok := plainHTTPProxyLineOutbound(line); ok {
			return []map[string]any{outbound}, true
		}
		return nil, false
	}
}

func standardProxyURISingBoxOutbound(line string) (map[string]any, bool) {
	u, err := url.Parse(line)
	if err != nil || u.Scheme == "" {
		return nil, false
	}
	nodeType := normalizeNodeType(u.Scheme)
	server := strings.TrimSpace(u.Hostname())
	port := anyInt(u.Port())
	if server == "" || port <= 0 {
		return nil, false
	}
	outboundType, err := singBoxOutboundType(nodeType)
	if err != nil {
		return nil, false
	}
	name := uriEntryName(u)
	outbound := map[string]any{
		"type":        outboundType,
		"tag":         name,
		"server":      server,
		"server_port": port,
	}
	switch nodeType {
	case "vless":
		if u.User == nil || strings.TrimSpace(u.User.Username()) == "" {
			return nil, false
		}
		outbound["uuid"] = strings.TrimSpace(u.User.Username())
		if flow := normalizeVLESSFlow(u.Query().Get("flow")); flow != "" {
			outbound["flow"] = flow
		}
	case "trojan", "hysteria2", "anytls", "shadowtls":
		if u.User == nil || strings.TrimSpace(u.User.Username()) == "" {
			return nil, false
		}
		outbound["password"] = strings.TrimSpace(u.User.Username())
	case "tuic":
		if u.User == nil || strings.TrimSpace(u.User.Username()) == "" {
			return nil, false
		}
		outbound["uuid"] = strings.TrimSpace(u.User.Username())
		password, _ := u.User.Password()
		if strings.TrimSpace(password) == "" {
			return nil, false
		}
		outbound["password"] = password
	default:
		return nil, false
	}
	if tls := uriTLSJSON(u); len(tls) > 0 {
		outbound["tls"] = rawJSONMap(tls)
	}
	if nodeType == "trojan" || nodeType == "hysteria2" || nodeType == "tuic" || nodeType == "anytls" {
		if _, ok := outbound["tls"]; !ok {
			outbound["tls"] = map[string]any{"enabled": true, "server_name": server}
		}
	}
	if transport := uriTransportJSON(u); len(transport) > 0 {
		outbound["transport"] = rawJSONMap(transport)
	}
	applyHysteria2FieldsFromURI(outbound, u)
	applyTUICFieldsFromURI(outbound, u)
	return outbound, true
}

func shadowsocksURISingBoxOutbound(line string) (map[string]any, bool) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, false
	}
	server, port, method, password, err := parseShadowsocksURIEndpoint(u)
	if err != nil || server == "" || port <= 0 || method == "" || password == "" {
		return nil, false
	}
	outbound := map[string]any{
		"type":        "shadowsocks",
		"tag":         uriEntryName(u),
		"server":      server,
		"server_port": port,
		"method":      normalizeShadowsocksMethod(method),
		"password":    password,
	}
	if plugin, pluginOpts := shadowsocksPluginFromRawQuery(u.RawQuery); plugin != "" {
		outbound["plugin"] = plugin
		if pluginOpts != "" {
			outbound["plugin_opts"] = pluginOpts
		}
	}
	return outbound, true
}

func httpSocksURISingBoxOutbound(line string) (map[string]any, bool) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, false
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	nodeType := "http"
	if strings.HasPrefix(scheme, "socks") {
		nodeType = "socks"
	}
	server := strings.TrimSpace(u.Hostname())
	port := anyInt(u.Port())
	if server == "" || port <= 0 {
		return nil, false
	}
	outbound := map[string]any{
		"type":        nodeType,
		"tag":         uriEntryName(u),
		"server":      server,
		"server_port": port,
	}
	if u.User != nil {
		if username := strings.TrimSpace(u.User.Username()); username != "" {
			outbound["username"] = username
		}
		if password, ok := u.User.Password(); ok && password != "" {
			outbound["password"] = password
		}
	}
	if scheme == "https" {
		tls := map[string]any{"enabled": true}
		if serverName := strings.TrimSpace(firstNonEmpty(u.Query().Get("sni"), u.Query().Get("servername"), u.Query().Get("peer"), server)); serverName != "" {
			tls["server_name"] = serverName
		}
		if queryBool(u.Query(), "allowInsecure", "insecure") {
			tls["insecure"] = true
		}
		outbound["tls"] = tls
	}
	return outbound, true
}

func plainHTTPProxyLineOutbound(line string) (map[string]any, bool) {
	if strings.Contains(line, "://") {
		return nil, false
	}
	if server, port, username, password, ok := parseIPPortUserPass(line); ok {
		return map[string]any{
			"type":        "http",
			"tag":         server + ":" + strconv.Itoa(port),
			"server":      server,
			"server_port": port,
			"username":    username,
			"password":    password,
		}, true
	}
	server, port, ok := parseHostPort(line)
	if !ok || net.ParseIP(server) == nil {
		return nil, false
	}
	return map[string]any{
		"type":        "http",
		"tag":         server + ":" + strconv.Itoa(port),
		"server":      server,
		"server_port": port,
	}, true
}

func parseIPPortUserPass(line string) (string, int, string, string, bool) {
	parts := strings.Split(line, ":")
	if len(parts) < 4 {
		return "", 0, "", "", false
	}
	password := strings.TrimSpace(parts[len(parts)-1])
	username := strings.TrimSpace(parts[len(parts)-2])
	portText := strings.TrimSpace(parts[len(parts)-3])
	server := strings.Trim(strings.TrimSpace(strings.Join(parts[:len(parts)-3], ":")), "[]")
	port := anyInt(portText)
	if server == "" || port <= 0 || username == "" || password == "" || net.ParseIP(server) == nil {
		return "", 0, "", "", false
	}
	return server, port, username, password, true
}

func parseHostPort(hostport string) (string, int, bool) {
	hostport = strings.TrimSpace(hostport)
	if host, portText, err := net.SplitHostPort(hostport); err == nil {
		port := anyInt(portText)
		host = strings.Trim(host, "[]")
		return host, port, host != "" && port > 0
	}
	idx := strings.LastIndex(hostport, ":")
	if idx <= 0 || idx >= len(hostport)-1 {
		return "", 0, false
	}
	host := strings.Trim(strings.TrimSpace(hostport[:idx]), "[]")
	port := anyInt(hostport[idx+1:])
	return host, port, host != "" && port > 0
}

func vmessURISingBoxOutbound(line string) (map[string]any, bool) {
	node, err := parseVMessURI(line)
	if err != nil {
		return nil, false
	}
	var outbound map[string]any
	if node.OutboundJSON != "" {
		_ = json.Unmarshal([]byte(node.OutboundJSON), &outbound)
	}
	if outbound == nil {
		outbound = map[string]any{
			"type":        "vmess",
			"tag":         node.Name,
			"server":      node.Server,
			"server_port": node.ServerPort,
			"uuid":        node.UUID,
			"security":    firstNonEmpty(node.Security, "auto"),
			"alter_id":    node.AlterID,
		}
		if len(node.TLSJSON) > 0 {
			outbound["tls"] = rawJSONMap(node.TLSJSON)
		}
		if len(node.TransportJSON) > 0 {
			outbound["transport"] = rawJSONMap(node.TransportJSON)
		}
	}
	return outbound, true
}

func applyHysteria2FieldsFromURI(outbound map[string]any, u *url.URL) {
	if outbound["type"] != "hysteria2" {
		return
	}
	query := u.Query()
	if ports := normalizeHysteriaPortList(firstNonEmpty(query.Get("ports"), query.Get("mport"))); len(ports) > 0 {
		outbound["server_ports"] = ports
	}
	if upMbps := anyInt(firstNonEmpty(query.Get("upmbps"), query.Get("up_mbps"), query.Get("up"))); upMbps > 0 {
		outbound["up_mbps"] = upMbps
	}
	if downMbps := anyInt(firstNonEmpty(query.Get("downmbps"), query.Get("down_mbps"), query.Get("down"))); downMbps > 0 {
		outbound["down_mbps"] = downMbps
	}
	if hopInterval, ok := normalizeDurationString(firstNonEmpty(query.Get("hopInterval"), query.Get("hop-interval"), query.Get("hop_interval")), "s"); ok {
		outbound["hop_interval"] = hopInterval
	}
	if obfsType := strings.TrimSpace(query.Get("obfs")); obfsType != "" {
		obfs := map[string]any{"type": obfsType}
		if obfsPassword := strings.TrimSpace(firstNonEmpty(query.Get("obfs-password"), query.Get("obfs_password"))); obfsPassword != "" {
			obfs["password"] = obfsPassword
		}
		outbound["obfs"] = obfs
	}
}

func applyTUICFieldsFromURI(outbound map[string]any, u *url.URL) {
	if outbound["type"] != "tuic" {
		return
	}
	query := u.Query()
	if congestion := strings.TrimSpace(firstNonEmpty(query.Get("congestion-controller"), query.Get("congestion_control"))); congestion != "" {
		outbound["congestion_control"] = congestion
	}
	if relay := strings.TrimSpace(firstNonEmpty(query.Get("udp-relay-mode"), query.Get("udp_relay_mode"))); relay != "" {
		outbound["udp_relay_mode"] = relay
	}
	if queryBool(query, "reduce-rtt", "zero-rtt-handshake", "zero_rtt_handshake") {
		outbound["zero_rtt_handshake"] = true
	}
	if heartbeat, ok := normalizeDurationString(firstNonEmpty(query.Get("heartbeat-interval"), query.Get("heartbeat_interval"), query.Get("heartbeat")), "ms"); ok {
		outbound["heartbeat"] = heartbeat
	}
}

func shadowsocksPluginFromRawQuery(rawQuery string) (string, string) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", ""
	}
	spec := strings.TrimSpace(firstNonEmpty(values.Get("plugin"), values.Get("plugin-opts"), values.Get("plugin_opts")))
	if spec == "" {
		return "", ""
	}
	plugin, opts, _ := strings.Cut(spec, ";")
	plugin, opts = normalizeShadowsocksPlugin(plugin, opts)
	return plugin, opts
}

func clashEntryName(proxy map[string]any) string {
	name := strings.TrimSpace(anyString(proxy["name"]))
	if name != "" {
		return name
	}
	server := strings.TrimSpace(anyString(proxy["server"]))
	port := anyInt(proxy["port"])
	if server != "" && port > 0 {
		return server + ":" + strconv.Itoa(port)
	}
	return ""
}

func uriEntryName(u *url.URL) string {
	name, _ := url.QueryUnescape(strings.TrimPrefix(u.Fragment, "#"))
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if u.Host != "" {
		return u.Host
	}
	if u.Opaque != "" {
		return u.Opaque
	}
	return u.String()
}

func missingParsedNodeRequiredField(node ParsedNode) bool {
	return missingSingBoxOutboundRequiredField(node.Type, map[string]any{
		"auth":        node.Password,
		"auth_str":    node.Password,
		"password":    node.Password,
		"private_key": "",
		"user":        node.Username,
		"username":    node.Username,
	}, node)
}

func parsedNodeSingBoxOutboundJSON(node ParsedNode) string {
	outboundType, err := singBoxOutboundType(node.Type)
	if err != nil {
		return ""
	}
	outbound := map[string]any{
		"type":        outboundType,
		"server":      strings.TrimSpace(node.Server),
		"server_port": node.ServerPort,
	}
	if strings.TrimSpace(node.Method) != "" {
		outbound["method"] = strings.TrimSpace(node.Method)
	}
	if strings.TrimSpace(node.UUID) != "" {
		outbound["uuid"] = strings.TrimSpace(node.UUID)
	}
	if flow := normalizeVLESSFlow(node.Flow); flow != "" {
		outbound["flow"] = flow
	}
	if strings.TrimSpace(node.Security) != "" {
		outbound["security"] = strings.TrimSpace(node.Security)
	}
	if node.AlterID > 0 {
		outbound["alter_id"] = node.AlterID
	}
	if node.Username != "" {
		outbound["username"] = node.Username
	}
	if node.Password != "" {
		outbound["password"] = node.Password
	}
	if len(node.TLSJSON) > 0 {
		var tls any
		if err := json.Unmarshal(node.TLSJSON, &tls); err == nil {
			outbound["tls"] = tls
		}
	}
	if len(node.TransportJSON) > 0 {
		var transport any
		if err := json.Unmarshal(node.TransportJSON, &transport); err == nil {
			outbound["transport"] = transport
		}
	}
	raw, err := json.Marshal(outbound)
	if err != nil {
		return ""
	}
	return string(raw)
}

func clashTLSJSON(proxy map[string]any) json.RawMessage {
	if !truthy(proxy["tls"]) &&
		strings.TrimSpace(firstNonEmpty(mapString(proxy, "sni", "servername", "server_name", "server-name", "peer"))) == "" &&
		strings.TrimSpace(firstNonEmpty(mapString(proxy, "fingerprint", "client-fingerprint", "client_fingerprint", "fp"))) == "" &&
		!clashHasReality(proxy) &&
		len(mapStringList(proxy, "alpn")) == 0 {
		return nil
	}
	tls := map[string]any{"enabled": true}
	if serverName := strings.TrimSpace(firstNonEmpty(mapString(proxy, "sni", "servername", "server_name", "server-name", "peer"))); serverName != "" {
		tls["server_name"] = serverName
	}
	if insecure, ok := mapBool(proxy, "skip-cert-verify", "allowInsecure", "insecure"); ok && insecure {
		tls["insecure"] = true
	}
	if alpn := mapStringList(proxy, "alpn"); len(alpn) > 0 {
		tls["alpn"] = alpn
	}
	applyUTLSFromValue(tls, firstNonEmpty(mapString(proxy, "fingerprint", "client-fingerprint", "client_fingerprint", "fp")))
	applyClashRealityToTLS(tls, proxy)
	applyTLSCertificateFromClash(tls, proxy)
	return mustMarshalRawMessage(tls)
}

func clashTransportJSON(proxy map[string]any) json.RawMessage {
	network := strings.TrimSpace(firstNonEmpty(anyString(proxy["network"]), anyString(proxy["type"])))
	if normalizeNodeType(network) != network {
		network = anyString(proxy["network"])
	}
	switch normalizeV2RayNetwork(network) {
	case "", "tcp":
		return nil
	case "ws":
		transport := map[string]any{"type": "ws"}
		if wsOpts, ok := mapAnyMap(proxy, "ws-opts", "ws_opts"); ok {
			if path := firstNonEmptyValue(wsOpts["path"]); path != "" {
				transport["path"] = path
			}
			if headers, ok := mapAnyMap(wsOpts, "headers"); ok && len(headers) > 0 {
				transport["headers"] = headers
			}
		}
		if path := strings.TrimSpace(firstNonEmpty(mapString(proxy, "ws-path", "ws_path", "path"))); path != "" {
			transport["path"] = path
		}
		if headers, ok := mapAnyMap(proxy, "ws-headers", "ws_headers"); ok && len(headers) > 0 {
			transport["headers"] = headers
		}
		if _, ok := transport["headers"]; !ok {
			if host := strings.TrimSpace(mapString(proxy, "host")); host != "" {
				transport["headers"] = map[string]any{"Host": host}
			}
		}
		return mustMarshalRawMessage(transport)
	case "grpc":
		transport := map[string]any{"type": "grpc"}
		if grpcOpts, ok := mapAnyMap(proxy, "grpc-opts", "grpc_opts"); ok {
			if serviceName := strings.TrimSpace(firstNonEmpty(mapString(grpcOpts, "grpc-service-name", "service-name", "service_name"))); serviceName != "" {
				transport["service_name"] = serviceName
			}
		}
		return mustMarshalRawMessage(transport)
	case "h2", "http":
		transport := map[string]any{"type": "http"}
		var opts map[string]any
		if normalizeV2RayNetwork(network) == "h2" {
			opts, _ = mapAnyMap(proxy, "h2-opts", "h2_opts")
		} else {
			opts, _ = mapAnyMap(proxy, "http-opts", "http_opts")
		}
		if opts != nil {
			applyHTTPTransportOptions(transport, opts)
		}
		return mustMarshalRawMessage(transport)
	case "quic":
		return mustMarshalRawMessage(map[string]any{"type": "quic"})
	case "httpupgrade", "http-upgrade":
		transport := map[string]any{"type": "httpupgrade"}
		if opts, ok := mapAnyMap(proxy, "http-upgrade-opts", "http_upgrade_opts", "http-opts", "http_opts"); ok {
			if path := firstNonEmptyValue(opts["path"]); path != "" {
				transport["path"] = path
			}
			if host := firstNonEmptyValue(opts["host"]); host != "" {
				transport["host"] = host
			}
			if headers, ok := mapAnyMap(opts, "headers"); ok && len(headers) > 0 {
				transport["headers"] = headers
			}
		}
		return mustMarshalRawMessage(transport)
	default:
		return nil
	}
}

func uriTLSJSON(u *url.URL) json.RawMessage {
	query := u.Query()
	security := strings.ToLower(strings.TrimSpace(firstNonEmpty(query.Get("security"), query.Get("tls"))))
	serverName := strings.TrimSpace(firstNonEmpty(query.Get("sni"), query.Get("servername"), query.Get("server_name"), query.Get("peer")))
	insecure := queryBool(query, "allowInsecure", "insecure", "skip-cert-verify")
	alpn := splitCommaList(query.Get("alpn"))
	fingerprint := strings.TrimSpace(firstNonEmpty(
		query.Get("fp"),
		query.Get("fingerprint"),
		query.Get("client-fingerprint"),
		query.Get("client_fingerprint"),
	))
	if security != "tls" && security != "true" && security != "reality" && serverName == "" && !insecure && len(alpn) == 0 && fingerprint == "" {
		return nil
	}
	tls := map[string]any{"enabled": true}
	if serverName != "" {
		tls["server_name"] = serverName
	}
	if insecure {
		tls["insecure"] = true
	}
	if len(alpn) > 0 {
		tls["alpn"] = alpn
	}
	if security == "reality" {
		reality := map[string]any{"enabled": true}
		if publicKey := strings.TrimSpace(firstNonEmpty(
			query.Get("pbk"),
			query.Get("publicKey"),
			query.Get("public-key"),
			query.Get("public_key"),
		)); publicKey != "" {
			reality["public_key"] = publicKey
		}
		if shortID := strings.TrimSpace(firstNonEmpty(
			query.Get("sid"),
			query.Get("shortId"),
			query.Get("short-id"),
			query.Get("short_id"),
		)); shortID != "" {
			reality["short_id"] = shortID
		}
		tls["reality"] = reality
		if fingerprint == "" {
			fingerprint = "chrome"
		}
	}
	applyUTLSFromValue(tls, fingerprint)
	return mustMarshalRawMessage(tls)
}

func uriTransportJSON(u *url.URL) json.RawMessage {
	query := u.Query()
	network := firstNonEmpty(query.Get("type"), query.Get("network"))
	if network == "" && queryBool(query, "ws") {
		network = "ws"
	}
	host := firstNonEmpty(query.Get("host"), query.Get("authority"), query.Get("ws.host"), query.Get("obfs-host"))
	path := firstNonEmpty(query.Get("path"), query.Get("wspath"), query.Get("ws-path"))
	serviceName := firstNonEmpty(query.Get("serviceName"), query.Get("service_name"), query.Get("service-name"), query.Get("grpc-service-name"), query.Get("grpc_service_name"))
	return proxyTransportJSON(network, host, path, serviceName)
}

func proxyTransportJSON(network, host, path, serviceName string) json.RawMessage {
	switch normalizeV2RayNetwork(network) {
	case "", "tcp":
		return nil
	case "ws":
		transport := map[string]any{"type": "ws"}
		if strings.TrimSpace(path) != "" {
			transport["path"] = strings.TrimSpace(path)
		}
		if strings.TrimSpace(host) != "" {
			transport["headers"] = map[string]any{"Host": strings.TrimSpace(host)}
		}
		return mustMarshalRawMessage(transport)
	case "h2", "http":
		transport := map[string]any{"type": "http"}
		if strings.TrimSpace(path) != "" {
			transport["path"] = strings.TrimSpace(path)
		}
		if strings.TrimSpace(host) != "" {
			transport["host"] = []string{strings.TrimSpace(host)}
		}
		return mustMarshalRawMessage(transport)
	case "grpc":
		transport := map[string]any{"type": "grpc"}
		serviceName = firstNonEmpty(strings.TrimSpace(serviceName), strings.TrimSpace(path))
		if serviceName != "" {
			transport["service_name"] = serviceName
		}
		return mustMarshalRawMessage(transport)
	case "quic":
		return mustMarshalRawMessage(map[string]any{"type": "quic"})
	case "httpupgrade", "http-upgrade":
		transport := map[string]any{"type": "httpupgrade"}
		if strings.TrimSpace(path) != "" {
			transport["path"] = strings.TrimSpace(path)
		}
		if strings.TrimSpace(host) != "" {
			transport["host"] = strings.TrimSpace(host)
		}
		return mustMarshalRawMessage(transport)
	default:
		return nil
	}
}

func parseVMessURI(line string) (ParsedNode, error) {
	payload := strings.TrimSpace(strings.TrimPrefix(line, "vmess://"))
	if fragmentIndex := strings.IndexByte(payload, '#'); fragmentIndex >= 0 {
		payload = payload[:fragmentIndex]
	}
	if unescaped, err := url.QueryUnescape(payload); err == nil {
		payload = unescaped
	}
	decoded, ok := decodeBase64Component(payload)
	if !ok {
		return ParsedNode{}, errors.New("invalid vmess payload")
	}
	var uri struct {
		Name     string `json:"ps"`
		Server   string `json:"add"`
		Port     string `json:"port"`
		UUID     string `json:"id"`
		AlterID  string `json:"aid"`
		Security string `json:"scy"`
		Network  string `json:"net"`
		TLS      string `json:"tls"`
		SNI      string `json:"sni"`
		Host     string `json:"host"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal([]byte(decoded), &uri); err != nil {
		return ParsedNode{}, err
	}
	port, err := strconv.Atoi(strings.TrimSpace(uri.Port))
	if err != nil || port <= 0 {
		return ParsedNode{}, errors.New("invalid vmess port")
	}
	name := strings.TrimSpace(uri.Name)
	if name == "" {
		name = uri.Server + ":" + strconv.Itoa(port)
	}
	node := ParsedNode{
		Name:       name,
		Type:       "vmess",
		Server:     strings.TrimSpace(uri.Server),
		ServerPort: port,
		UUID:       strings.TrimSpace(uri.UUID),
		Security:   strings.TrimSpace(uri.Security),
		AlterID:    anyInt(uri.AlterID),
		RawJSON:    line,
	}
	if node.Security == "" {
		node.Security = "auto"
	}
	if strings.TrimSpace(uri.TLS) == "tls" {
		tls := map[string]any{"enabled": true}
		if strings.TrimSpace(uri.SNI) != "" {
			tls["server_name"] = strings.TrimSpace(uri.SNI)
		}
		node.TLSJSON = mustMarshalRawMessage(tls)
	}
	if transport := vmessURITransportJSON(uri.Network, uri.Host, uri.Path); len(transport) > 0 {
		node.TransportJSON = transport
	}
	return node, nil
}

func vmessURITransportJSON(network, host, path string) json.RawMessage {
	return proxyTransportJSON(network, host, path, "")
}

func normalizeV2RayNetwork(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "websocket":
		return "ws"
	case "mkcp":
		return "kcp"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func queryBool(values url.Values, keys ...string) bool {
	for _, key := range keys {
		if raw := values.Get(key); raw != "" {
			parsed, ok := anyBool(raw)
			return ok && parsed
		}
	}
	return false
}

func clashHasReality(proxy map[string]any) bool {
	if realityOpts, ok := mapAnyMap(proxy, "reality-opts", "reality_opts"); ok {
		return mapString(realityOpts, "public-key", "public_key", "pbk", "short-id", "short_id", "sid") != ""
	}
	return mapString(proxy, "public-key", "public_key", "pbk", "short-id", "short_id", "sid") != ""
}

func applyUTLSFromValue(tls map[string]any, rawFingerprint string) {
	fingerprint := strings.TrimSpace(rawFingerprint)
	if fingerprint == "" {
		return
	}
	tls["utls"] = map[string]any{
		"enabled":     true,
		"fingerprint": fingerprint,
	}
}

func applyClashRealityToTLS(tls map[string]any, proxy map[string]any) {
	realitySource := proxy
	if realityOpts, ok := mapAnyMap(proxy, "reality-opts", "reality_opts"); ok {
		realitySource = realityOpts
	}
	publicKey := strings.TrimSpace(mapString(realitySource, "public-key", "public_key", "pbk"))
	shortID := strings.TrimSpace(mapString(realitySource, "short-id", "short_id", "sid"))
	if publicKey == "" && shortID == "" {
		return
	}
	reality := map[string]any{"enabled": true}
	if publicKey != "" {
		reality["public_key"] = publicKey
	}
	if shortID != "" {
		reality["short_id"] = shortID
	}
	tls["reality"] = reality
	if _, ok := tls["utls"]; !ok {
		tls["utls"] = map[string]any{
			"enabled":     true,
			"fingerprint": "chrome",
		}
	}
}

func applyTLSCertificateFromClash(tls map[string]any, proxy map[string]any) {
	if certificatePath := strings.TrimSpace(mapString(proxy, "ca")); certificatePath != "" {
		tls["certificate_path"] = certificatePath
	}
	if certificate := strings.TrimSpace(mapString(proxy, "ca-str", "ca_str")); certificate != "" {
		tls["certificate"] = []string{certificate}
	}
}

func rawJSONMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func copyStringField(outbound map[string]any, source map[string]any, targetKey string, sourceKeys ...string) {
	if value := strings.TrimSpace(mapString(source, sourceKeys...)); value != "" {
		outbound[targetKey] = value
	}
}

func applyShadowsocksPluginFromClash(outbound map[string]any, proxy map[string]any) {
	plugin := strings.TrimSpace(mapString(proxy, "plugin"))
	pluginOpts := strings.TrimSpace(mapString(proxy, "plugin-opts-string", "plugin_opts_string", "plugin-opts", "plugin_opts"))
	if pluginOpts == "" {
		if opts, ok := mapAnyMap(proxy, "plugin-opts", "plugin_opts"); ok {
			pluginOpts = pluginOptionsString(opts)
		}
	}
	if plugin == "" {
		if obfsMode := strings.TrimSpace(mapString(proxy, "obfs")); obfsMode != "" {
			plugin = "obfs-local"
			parts := []string{"obfs=" + obfsMode}
			if obfsHost := strings.TrimSpace(mapString(proxy, "obfs-host", "obfs_host")); obfsHost != "" {
				parts = append(parts, "obfs-host="+obfsHost)
			}
			pluginOpts = strings.Join(parts, ";")
		}
	}
	if plugin == "" {
		return
	}
	plugin, pluginOpts = normalizeShadowsocksPlugin(plugin, pluginOpts)
	outbound["plugin"] = plugin
	if pluginOpts != "" {
		outbound["plugin_opts"] = pluginOpts
	}
}

func pluginOptionsString(opts map[string]any) string {
	if len(opts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(opts))
	for key := range opts {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(anyString(opts[key]))
		if value == "" {
			value = strings.TrimSpace(fmt.Sprint(opts[key]))
		}
		if value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, ";")
}

func normalizeShadowsocksPlugin(plugin, pluginOpts string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(plugin)) {
	case "obfs", "simple-obfs", "obfs-local":
		return "obfs-local", normalizeSimpleObfsOptions(pluginOpts)
	default:
		return strings.TrimSpace(plugin), strings.TrimSpace(pluginOpts)
	}
}

func normalizeSimpleObfsOptions(pluginOpts string) string {
	pluginOpts = strings.TrimSpace(pluginOpts)
	if pluginOpts == "" {
		return ""
	}
	parts := strings.Split(pluginOpts, ";")
	var normalized []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || strings.EqualFold(part, "obfs-local") {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			normalized = append(normalized, part)
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch strings.ToLower(key) {
		case "mode":
			key = "obfs"
		case "host", "obfs_host":
			key = "obfs-host"
		}
		normalized = append(normalized, key+"="+value)
	}
	return strings.Join(normalized, ";")
}

func hysteriaLikeTLSFromClash(proxy map[string]any, server string) map[string]any {
	tls := rawJSONMap(clashTLSJSON(proxy))
	if tls == nil {
		tls = map[string]any{"enabled": true}
	}
	if _, ok := tls["server_name"]; !ok && strings.TrimSpace(server) != "" {
		tls["server_name"] = server
	}
	if disableSNI, ok := mapBool(proxy, "disable-sni", "disable_sni"); ok && disableSNI {
		tls["disable_sni"] = true
	}
	return tls
}

func applyHysteriaFieldsFromClash(outbound map[string]any, proxy map[string]any) {
	if ports := normalizeHysteriaPortList(mapString(proxy, "ports", "mport")); len(ports) > 0 {
		outbound["server_ports"] = ports
	}
	if up := normalizeHysteriaRate(mapString(proxy, "up", "up-speed", "up_speed")); up != "" {
		outbound["up"] = up
	}
	if down := normalizeHysteriaRate(mapString(proxy, "down", "down-speed", "down_speed")); down != "" {
		outbound["down"] = down
	}
	if obfs := strings.TrimSpace(mapString(proxy, "obfs")); obfs != "" {
		outbound["obfs"] = obfs
	}
	if recvWindowConn := anyInt(firstNonNil(proxy["recv-window-conn"], proxy["recv_window_conn"])); recvWindowConn > 0 {
		outbound["recv_window_conn"] = recvWindowConn
	}
	if recvWindow := anyInt(firstNonNil(proxy["recv-window"], proxy["recv_window"])); recvWindow > 0 {
		outbound["recv_window"] = recvWindow
	}
	if disableMTUDiscovery, ok := mapBool(proxy, "disable-mtu-discovery", "disable_mtu_discovery"); ok {
		outbound["disable_mtu_discovery"] = disableMTUDiscovery
	}
	if hopInterval, ok := durationStringFromMap(proxy, "s", "hop-interval", "hop_interval", "s"); ok {
		outbound["hop_interval"] = hopInterval
	}
	if strings.EqualFold(strings.TrimSpace(mapString(proxy, "protocol")), "udp") {
		outbound["network"] = "udp"
	}
}

func applyHysteria2FieldsFromClash(outbound map[string]any, proxy map[string]any) {
	if ports := normalizeHysteriaPortList(mapString(proxy, "ports", "mport")); len(ports) > 0 {
		outbound["server_ports"] = ports
	}
	if upMbps := anyInt(firstNonNil(proxy["up-mbps"], proxy["up_mbps"], proxy["up"])); upMbps > 0 {
		outbound["up_mbps"] = upMbps
	}
	if downMbps := anyInt(firstNonNil(proxy["down-mbps"], proxy["down_mbps"], proxy["down"])); downMbps > 0 {
		outbound["down_mbps"] = downMbps
	}
	if hopInterval, ok := durationStringFromMap(proxy, "s", "hop-interval", "hop_interval", "s"); ok {
		outbound["hop_interval"] = hopInterval
	}
	if obfsType := strings.TrimSpace(mapString(proxy, "obfs")); obfsType != "" {
		obfs := map[string]any{"type": obfsType}
		if obfsPassword := strings.TrimSpace(mapString(proxy, "obfs-password", "obfs_password")); obfsPassword != "" {
			obfs["password"] = obfsPassword
		}
		outbound["obfs"] = obfs
	}
}

func applyTUICFieldsFromClash(outbound map[string]any, proxy map[string]any) {
	copyStringField(outbound, proxy, "congestion_control", "congestion-controller", "congestion_control")
	copyStringField(outbound, proxy, "udp_relay_mode", "udp-relay-mode", "udp_relay_mode")
	if zeroRTT, ok := mapBool(proxy, "reduce-rtt", "zero-rtt-handshake", "zero_rtt_handshake"); ok {
		outbound["zero_rtt_handshake"] = zeroRTT
	}
	if heartbeat, ok := durationStringFromMap(proxy, "ms", "heartbeat-interval", "heartbeat_interval", "heartbeat"); ok {
		outbound["heartbeat"] = heartbeat
	}
}

func applyWireGuardFieldsFromClash(outbound map[string]any, proxy map[string]any, server string, port int) bool {
	privateKey := strings.TrimSpace(mapString(proxy, "private-key", "private_key"))
	publicKey := strings.TrimSpace(mapString(proxy, "public-key", "public_key", "peer-public-key", "peer_public_key"))
	localAddress := wireGuardLocalAddress(proxy)
	if privateKey == "" || publicKey == "" || len(localAddress) == 0 {
		return false
	}
	allowedIPs := mapStringList(proxy, "allowed-ips", "allowed_ips")
	if len(allowedIPs) == 0 {
		allowedIPs = []string{"0.0.0.0/0", "::/0"}
	}
	outbound["private_key"] = privateKey
	outbound["local_address"] = localAddress
	outbound["peer_public_key"] = publicKey
	peer := map[string]any{
		"server":      server,
		"server_port": port,
		"public_key":  publicKey,
		"allowed_ips": allowedIPs,
	}
	if preSharedKey := strings.TrimSpace(mapString(proxy, "pre-shared-key", "pre_shared_key", "preshared-key", "preshared_key")); preSharedKey != "" {
		outbound["pre_shared_key"] = preSharedKey
		peer["pre_shared_key"] = preSharedKey
	}
	if reserved := wireGuardReserved(proxy["reserved"]); len(reserved) == 3 {
		outbound["reserved"] = reserved
		peer["reserved"] = reserved
	}
	outbound["peers"] = []map[string]any{peer}
	if mtu := anyInt(proxy["mtu"]); mtu > 0 {
		outbound["mtu"] = mtu
	}
	if udp, ok := mapBool(proxy, "udp"); ok && !udp {
		outbound["network"] = "tcp"
	}
	return true
}

func wireGuardLocalAddress(proxy map[string]any) []string {
	var out []string
	for _, key := range []string{"ip", "self-ip", "self_ip", "self-ipv4", "self_ipv4"} {
		if prefix := normalizeWireGuardPrefix(mapString(proxy, key)); prefix != "" {
			out = append(out, prefix)
			break
		}
	}
	for _, key := range []string{"ipv6", "self-ip-v6", "self_ip_v6", "self-ipv6", "self_ipv6"} {
		if prefix := normalizeWireGuardPrefix(mapString(proxy, key)); prefix != "" {
			out = append(out, prefix)
			break
		}
	}
	if len(out) == 0 {
		out = mapStringList(proxy, "local_address", "address")
	}
	return out
}

func normalizeWireGuardPrefix(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || strings.Contains(value, "/") {
		return value
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return value
	}
	if ip.To4() != nil {
		return ip.String() + "/32"
	}
	return ip.String() + "/128"
}

func wireGuardReserved(value any) []int {
	switch x := value.(type) {
	case []int:
		return x
	case []any:
		out := make([]int, 0, len(x))
		for _, item := range x {
			n := anyInt(item)
			if n < 0 || n > 255 {
				return nil
			}
			out = append(out, n)
		}
		return out
	case string:
		parts := strings.FieldsFunc(x, func(r rune) bool {
			return r == ',' || r == '/' || r == ':' || r == '|' || r == ' '
		})
		out := make([]int, 0, len(parts))
		for _, part := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 0 || n > 255 {
				return nil
			}
			out = append(out, n)
		}
		return out
	default:
		return nil
	}
}

func normalizeHysteriaPortList(raw string) []string {
	ports := splitCommaList(raw)
	if len(ports) == 0 {
		return nil
	}
	out := make([]string, 0, len(ports))
	for _, port := range ports {
		out = append(out, normalizeHysteriaPortRange(port))
	}
	return out
}

func normalizeHysteriaPortRange(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || strings.Contains(value, ":") {
		return value
	}
	compact := strings.ReplaceAll(value, " ", "")
	start, end, ok := strings.Cut(compact, "-")
	if !ok || start == "" || end == "" || !decimalString(start) || !decimalString(end) {
		return value
	}
	return start + ":" + end
}

func normalizeHysteriaRate(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if stringHasLetter(value) {
		return value
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value + " Mbps"
	}
	return value
}

func durationStringFromMap(m map[string]any, defaultUnit string, keys ...string) (string, bool) {
	for _, key := range keys {
		if duration, ok := normalizeDurationString(m[key], defaultUnit); ok {
			return duration, true
		}
	}
	return "", false
}

func normalizeDurationString(value any, defaultUnit string) (string, bool) {
	text := strings.TrimSpace(anyString(value))
	if text == "" && value != nil {
		text = strings.TrimSpace(fmt.Sprint(value))
	}
	if text == "" {
		return "", false
	}
	if stringHasLetter(text) {
		return text, true
	}
	if _, err := strconv.ParseFloat(text, 64); err == nil {
		return text + defaultUnit, true
	}
	return "", false
}

func decimalString(raw string) bool {
	if raw == "" {
		return false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func stringHasLetter(raw string) bool {
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == 'µ' {
			return true
		}
	}
	return false
}

func applyClashDialFields(outbound map[string]any, proxy map[string]any) {
	copyStringField(outbound, proxy, "detour", "dialer-proxy", "dialer_proxy")
	copyStringField(outbound, proxy, "bind_interface", "bind-interface", "bind_interface", "interface-name", "interface_name")
	if routingMark := anyInt(firstNonNil(proxy["routing-mark"], proxy["routing_mark"])); routingMark > 0 {
		outbound["routing_mark"] = routingMark
	}
	if tcpFastOpen, ok := mapBool(proxy, "fast-open", "fast_open", "tfo"); ok {
		outbound["tcp_fast_open"] = tcpFastOpen
	}
	if tcpMultiPath, ok := mapBool(proxy, "mptcp", "tcp-multi-path", "tcp_multi_path"); ok {
		outbound["tcp_multi_path"] = tcpMultiPath
	}
	if udpFragment, ok := mapBool(proxy, "udp-fragment", "udp_fragment"); ok {
		outbound["udp_fragment"] = udpFragment
	}
	if domainStrategy := clashDomainStrategy(mapString(proxy, "ip-version", "ip_version")); domainStrategy != "" {
		outbound["domain_strategy"] = domainStrategy
	}
}

func clashDomainStrategy(raw string) string {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), "_", "-")) {
	case "ipv4":
		return "ipv4_only"
	case "ipv6":
		return "ipv6_only"
	case "prefer-ipv4":
		return "prefer_ipv4"
	case "prefer-ipv6":
		return "prefer_ipv6"
	default:
		return ""
	}
}

func applyHTTPTransportOptions(transport map[string]any, opts map[string]any) {
	if path := firstNonEmptyValue(opts["path"]); path != "" {
		transport["path"] = path
	}
	if hosts := stringListValue(opts["host"]); len(hosts) > 0 {
		transport["host"] = hosts
		return
	}
	headers, ok := mapAnyMap(opts, "headers")
	if !ok {
		return
	}
	if hosts := stringListValue(firstNonNil(headers["Host"], headers["host"])); len(hosts) > 0 {
		transport["host"] = hosts
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func parseNodeURIEndpoint(nodeType string, u *url.URL) (string, int, string, string, error) {
	if normalizeNodeType(nodeType) == "shadowsocks" {
		return parseShadowsocksURIEndpoint(u)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 {
		return "", 0, "", "", err
	}
	server := u.Hostname()
	if server == "" {
		return "", 0, "", "", errors.New("missing host")
	}
	return server, port, "", "", nil
}

func parseShadowsocksURIEndpoint(u *url.URL) (string, int, string, string, error) {
	if u.User != nil {
		method := u.User.Username()
		password, _ := u.User.Password()
		if decoded, ok := decodeBase64Component(method); ok {
			method, password = splitMethodPassword(decoded)
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil || port <= 0 {
			return "", 0, "", "", err
		}
		server := u.Hostname()
		if server == "" {
			return "", 0, "", "", errors.New("missing host")
		}
		return server, port, method, password, nil
	}
	if decoded, ok := decodeBase64Component(u.Host); ok {
		method, password, server, port, err := parseLegacyShadowsocksDecodedURI(decoded)
		if err != nil {
			return "", 0, "", "", err
		}
		return server, port, method, password, nil
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 {
		return "", 0, "", "", err
	}
	server := u.Hostname()
	if server == "" {
		return "", 0, "", "", errors.New("missing host")
	}
	return server, port, "", "", nil
}

func parseLegacyShadowsocksDecodedURI(decoded string) (string, string, string, int, error) {
	at := strings.LastIndex(decoded, "@")
	if at <= 0 || at == len(decoded)-1 {
		return "", "", "", 0, errors.New("missing endpoint")
	}
	credentials := decoded[:at]
	endpoint := decoded[at+1:]
	method, password := splitMethodPassword(credentials)
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", "", "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return "", "", "", 0, err
	}
	return method, password, host, port, nil
}

func splitMethodPassword(text string) (string, string) {
	method, password, _ := strings.Cut(text, ":")
	return method, password
}
