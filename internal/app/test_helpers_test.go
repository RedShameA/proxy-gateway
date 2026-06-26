package app_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

const testChainLinkProbeTarget = "www.gstatic.com:443"

func get(t *testing.T, url string, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func getJSON(t *testing.T, url string, token string, out any) {
	t.Helper()
	resp := get(t, url, token)
	decodeJSON(t, resp, out)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d", url, resp.StatusCode)
	}
}

func waitForRequestLogs(t *testing.T, url string, token string, minCount int) []map[string]any {
	t.Helper()
	var logs struct {
		RequestLogs []map[string]any `json:"request_logs"`
	}
	for i := 0; i < 50; i++ {
		getJSON(t, url, token, &logs)
		if len(logs.RequestLogs) >= minCount {
			return logs.RequestLogs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("request log count = %d, want at least %d", len(logs.RequestLogs), minCount)
	return nil
}

func postJSON(t *testing.T, url string, token string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func patchJSON(t *testing.T, url string, token string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func deleteRequest(t *testing.T, url string, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode status %d: %v", resp.StatusCode, err)
	}
}

func decodeOK(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	decodeJSON(t, resp, out)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func setupAdmin(t *testing.T, baseURL string) string {
	t.Helper()
	resp := postJSON(t, baseURL+"/api/admin/setup", "", map[string]string{
		"password": "correct horse battery staple",
	})
	var body struct {
		Token string `json:"token"`
	}
	decodeOK(t, resp, &body)
	return body.Token
}

func createFixedHTTPProxyAccess(t *testing.T, baseURL, adminToken string, upstreamProxy *testHTTPConnectProxy) (string, string) {
	t.Helper()
	nodeResp := postJSON(t, baseURL+"/api/nodes", adminToken, map[string]any{
		"name":        "upstream-http",
		"type":        "http",
		"server":      upstreamProxy.host,
		"server_port": upstreamProxy.port,
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	profileResp := postJSON(t, baseURL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	password := "proxy-password-123"
	credentialResp := postJSON(t, baseURL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "fixed client",
		"password": password,
	})
	decodeOK(t, credentialResp, &struct{}{})
	return profile.ID, password
}

func createHTTPNode(t *testing.T, baseURL, adminToken, name string, upstreamProxy *testHTTPConnectProxy) string {
	t.Helper()
	nodeResp := postJSON(t, baseURL+"/api/nodes", adminToken, map[string]any{
		"name":        name,
		"type":        "http",
		"server":      upstreamProxy.host,
		"server_port": upstreamProxy.port,
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	return node.ID
}

type testHTTPConnectProxy struct {
	host              string
	port              int
	connects          int
	close             func()
	delay             time.Duration
	interceptHost     string
	interceptCountry  string
	connectOnlyHosts  map[string]bool
	closeAfterConnect bool
}

type testSOCKS5Proxy struct {
	host               string
	port               int
	connects           int
	close              func()
	delay              time.Duration
	connectOnlyTargets map[string]bool
}

func (p *testHTTPConnectProxy) portText() string {
	return fmt.Sprintf("%d", p.port)
}

func (p *testSOCKS5Proxy) portText() string {
	return fmt.Sprintf("%d", p.port)
}

func newHTTPConnectProxy(t *testing.T) *testHTTPConnectProxy {
	return newDelayedHTTPConnectProxy(t, 0)
}

func newBrokenHTTPConnectProxy(t *testing.T) *testHTTPConnectProxy {
	t.Helper()
	p := newDelayedHTTPConnectProxy(t, 0)
	p.closeAfterConnect = true
	return p
}

func newDelayedHTTPConnectProxy(t *testing.T, delay time.Duration) *testHTTPConnectProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	p := &testHTTPConnectProxy{
		host:  "127.0.0.1",
		port:  port,
		close: func() { _ = ln.Close() },
		delay: delay,
	}
	t.Cleanup(p.close)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go p.handle(conn)
		}
	}()
	return p
}

func (p *testHTTPConnectProxy) allowChainLinkProbeTarget() {
	if p.connectOnlyHosts == nil {
		p.connectOnlyHosts = map[string]bool{}
	}
	p.connectOnlyHosts[testChainLinkProbeTarget] = true
}

func newCountryHTTPConnectProxy(t *testing.T, interceptHost, country string) *testHTTPConnectProxy {
	p := newDelayedHTTPConnectProxy(t, 0)
	p.interceptHost = interceptHost
	p.interceptCountry = country
	return p
}

func (p *testHTTPConnectProxy) handle(conn net.Conn) {
	defer conn.Close()
	req, err := http.ReadRequest(bufioNewReader(conn))
	if err != nil {
		return
	}
	if req.Method != http.MethodConnect {
		return
	}
	p.connects++
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	if p.connectOnlyHosts[req.Host] {
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		return
	}
	if p.interceptHost != "" && req.Host == p.interceptHost {
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		inner, err := http.ReadRequest(bufioNewReader(conn))
		if err != nil {
			return
		}
		_ = inner.Body.Close()
		body := []byte(`{"ip":"198.51.100.1","country":"` + p.interceptCountry + `"}`)
		_, _ = conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n", len(body))))
		_, _ = conn.Write(body)
		return
	}
	if p.closeAfterConnect {
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		return
	}
	target, err := net.Dial("tcp", req.Host)
	if err != nil {
		_, _ = conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer target.Close()
	_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	go func() { _, _ = io.Copy(target, conn) }()
	_, _ = io.Copy(conn, target)
}

func bufioNewReader(r io.Reader) *bufio.Reader {
	return bufio.NewReader(r)
}

func newSOCKS5Proxy(t *testing.T) *testSOCKS5Proxy {
	return newDelayedSOCKS5Proxy(t, 0)
}

func newDelayedSOCKS5Proxy(t *testing.T, delay time.Duration) *testSOCKS5Proxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	p := &testSOCKS5Proxy{
		host:  "127.0.0.1",
		port:  port,
		close: func() { _ = ln.Close() },
		delay: delay,
	}
	t.Cleanup(p.close)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go p.handle(conn)
		}
	}()
	return p
}

func (p *testSOCKS5Proxy) allowChainLinkProbeTarget() {
	if p.connectOnlyTargets == nil {
		p.connectOnlyTargets = map[string]bool{}
	}
	p.connectOnlyTargets[testChainLinkProbeTarget] = true
}

func (p *testSOCKS5Proxy) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufioNewReader(conn)
	version, err := reader.ReadByte()
	if err != nil || version != 0x05 {
		return
	}
	methodCount, err := reader.ReadByte()
	if err != nil {
		return
	}
	methods := make([]byte, int(methodCount))
	if _, err := io.ReadFull(reader, methods); err != nil {
		return
	}
	noAuth := false
	for _, method := range methods {
		if method == 0x00 {
			noAuth = true
			break
		}
	}
	if !noAuth {
		_, _ = conn.Write([]byte{0x05, 0xff})
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}
	host, port, ok := readSOCKS5ProxyConnectRequest(reader, conn)
	if !ok {
		return
	}
	p.connects++
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	if p.connectOnlyTargets[net.JoinHostPort(host, fmt.Sprintf("%d", port))] {
		_, _ = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	target, err := net.Dial("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		writeSOCKS5ProxyFailure(conn, 0x04)
		return
	}
	defer target.Close()
	_, _ = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	go func() { _, _ = io.Copy(target, conn) }()
	_, _ = io.Copy(conn, target)
}

func readSOCKS5ProxyConnectRequest(reader *bufio.Reader, conn net.Conn) (string, int, bool) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		writeSOCKS5ProxyFailure(conn, 0x01)
		return "", 0, false
	}
	if header[0] != 0x05 || header[1] != 0x01 || header[2] != 0x00 {
		writeSOCKS5ProxyFailure(conn, 0x07)
		return "", 0, false
	}
	var host string
	switch header[3] {
	case 0x01:
		raw := make([]byte, 4)
		if _, err := io.ReadFull(reader, raw); err != nil {
			writeSOCKS5ProxyFailure(conn, 0x01)
			return "", 0, false
		}
		host = net.IP(raw).String()
	case 0x03:
		l, err := reader.ReadByte()
		if err != nil {
			writeSOCKS5ProxyFailure(conn, 0x01)
			return "", 0, false
		}
		raw := make([]byte, int(l))
		if _, err := io.ReadFull(reader, raw); err != nil {
			writeSOCKS5ProxyFailure(conn, 0x01)
			return "", 0, false
		}
		host = string(raw)
	case 0x04:
		raw := make([]byte, 16)
		if _, err := io.ReadFull(reader, raw); err != nil {
			writeSOCKS5ProxyFailure(conn, 0x01)
			return "", 0, false
		}
		host = net.IP(raw).String()
	default:
		writeSOCKS5ProxyFailure(conn, 0x08)
		return "", 0, false
	}
	portRaw := make([]byte, 2)
	if _, err := io.ReadFull(reader, portRaw); err != nil {
		writeSOCKS5ProxyFailure(conn, 0x01)
		return "", 0, false
	}
	port := int(portRaw[0])<<8 | int(portRaw[1])
	return host, port, true
}

func writeSOCKS5ProxyFailure(conn net.Conn, code byte) {
	_, _ = conn.Write([]byte{0x05, code, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
}

func socks5Connect(proxyAddr, username, password, targetHost string, targetPort int) (net.Conn, error) {
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (net.Conn, error) {
		_ = conn.Close()
		return nil, err
	}
	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		return fail(err)
	}
	var method [2]byte
	if _, err := io.ReadFull(conn, method[:]); err != nil {
		return fail(err)
	}
	if method != [2]byte{0x05, 0x02} {
		return fail(fmt.Errorf("socks method response = %v", method))
	}
	auth := []byte{0x01, byte(len(username))}
	auth = append(auth, []byte(username)...)
	auth = append(auth, byte(len(password)))
	auth = append(auth, []byte(password)...)
	if _, err := conn.Write(auth); err != nil {
		return fail(err)
	}
	var authResp [2]byte
	if _, err := io.ReadFull(conn, authResp[:]); err != nil {
		return fail(err)
	}
	if authResp != [2]byte{0x01, 0x00} {
		return fail(fmt.Errorf("socks auth response = %v", authResp))
	}
	hostBytes := []byte(targetHost)
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(hostBytes))}
	req = append(req, hostBytes...)
	req = append(req, byte(targetPort>>8), byte(targetPort))
	if _, err := conn.Write(req); err != nil {
		return fail(err)
	}
	var head [4]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return fail(err)
	}
	if head[1] != 0x00 {
		return fail(fmt.Errorf("socks connect failed: %v", head))
	}
	switch head[3] {
	case 0x01:
		if _, err := io.ReadFull(conn, make([]byte, 6)); err != nil {
			return fail(err)
		}
	case 0x03:
		var l [1]byte
		if _, err := io.ReadFull(conn, l[:]); err != nil {
			return fail(err)
		}
		if _, err := io.ReadFull(conn, make([]byte, int(l[0])+2)); err != nil {
			return fail(err)
		}
	case 0x04:
		if _, err := io.ReadFull(conn, make([]byte, 18)); err != nil {
			return fail(err)
		}
	default:
		return fail(fmt.Errorf("unexpected socks atyp %d", head[3]))
	}
	return conn, nil
}

func socks5ConnectExpectFailure(proxyAddr, username, password, targetHost string, targetPort int) error {
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		return err
	}
	var method [2]byte
	if _, err := io.ReadFull(conn, method[:]); err != nil {
		return err
	}
	if method != [2]byte{0x05, 0x02} {
		return fmt.Errorf("socks method response = %v", method)
	}
	auth := []byte{0x01, byte(len(username))}
	auth = append(auth, []byte(username)...)
	auth = append(auth, byte(len(password)))
	auth = append(auth, []byte(password)...)
	if _, err := conn.Write(auth); err != nil {
		return err
	}
	var authResp [2]byte
	if _, err := io.ReadFull(conn, authResp[:]); err != nil {
		return err
	}
	if authResp != [2]byte{0x01, 0x00} {
		return fmt.Errorf("socks auth response = %v", authResp)
	}
	hostBytes := []byte(targetHost)
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(hostBytes))}
	req = append(req, hostBytes...)
	req = append(req, byte(targetPort>>8), byte(targetPort))
	if _, err := conn.Write(req); err != nil {
		return err
	}
	var head [4]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return err
	}
	if head[1] == 0x00 {
		return fmt.Errorf("socks connect unexpectedly succeeded: %v", head)
	}
	return nil
}

func socks5CommandReplyCode(proxyAddr, username, password string, command byte, targetHost string, targetPort int) (byte, error) {
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		return 0, err
	}
	var method [2]byte
	if _, err := io.ReadFull(conn, method[:]); err != nil {
		return 0, err
	}
	if method != [2]byte{0x05, 0x02} {
		return 0, fmt.Errorf("socks method response = %v", method)
	}
	auth := []byte{0x01, byte(len(username))}
	auth = append(auth, []byte(username)...)
	auth = append(auth, byte(len(password)))
	auth = append(auth, []byte(password)...)
	if _, err := conn.Write(auth); err != nil {
		return 0, err
	}
	var authResp [2]byte
	if _, err := io.ReadFull(conn, authResp[:]); err != nil {
		return 0, err
	}
	if authResp != [2]byte{0x01, 0x00} {
		return 0, fmt.Errorf("socks auth response = %v", authResp)
	}
	hostBytes := []byte(targetHost)
	req := []byte{0x05, command, 0x00, 0x03, byte(len(hostBytes))}
	req = append(req, hostBytes...)
	req = append(req, byte(targetPort>>8), byte(targetPort))
	if _, err := conn.Write(req); err != nil {
		return 0, err
	}
	var head [4]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return 0, err
	}
	return head[1], nil
}
