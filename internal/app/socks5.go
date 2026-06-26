package app

import (
	"bufio"
	"io"
	"net"
	"strconv"
	"time"
)

const socks5HandshakeTimeout = 10 * time.Second

func (g *Gateway) handleSOCKS5Conn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(socks5HandshakeTimeout))
	reader := bufio.NewReader(conn)
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
	authSupported := false
	for _, method := range methods {
		if method == 0x02 {
			authSupported = true
			break
		}
	}
	if !authSupported {
		_, _ = conn.Write([]byte{0x05, 0xff})
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x02}); err != nil {
		return
	}
	username, password, ok := readSOCKS5UsernamePassword(reader, conn)
	if !ok {
		return
	}
	credential, ok := g.proxyCredentialForUsernamePassword(conn, username, password)
	if !ok {
		return
	}
	path, err := g.proxyPathForCredential(credential)
	if err != nil {
		writeSOCKS5Failure(conn, 0x01)
		return
	}
	target, ok := readSOCKS5ConnectTarget(reader, conn)
	if !ok {
		return
	}
	_ = conn.SetDeadline(time.Time{})
	start := time.Now()
	logID := g.startProxyRequest(path, target, start)
	targetConn, err := g.dialProxyPath(path, target)
	if err != nil {
		g.finishProxyRequest(logID, false, requestFailureStageDial, err.Error(), 0, elapsedMilliseconds(start), 0, 0)
		writeSOCKS5Failure(conn, 0x04)
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		_ = targetConn.Close()
		g.finishProxyRequest(logID, false, requestFailureStageProxyHandshake, "write socks5 success response", 0, elapsedMilliseconds(start), 0, 0)
		return
	}
	ingressBytes, egressBytes := pipeConns(conn, targetConn)
	g.finishProxyRequest(logID, true, "", "", 0, elapsedMilliseconds(start), ingressBytes, egressBytes)
}

func readSOCKS5UsernamePassword(reader *bufio.Reader, conn net.Conn) (string, string, bool) {
	version, err := reader.ReadByte()
	if err != nil || version != 0x01 {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return "", "", false
	}
	userLen, err := reader.ReadByte()
	if err != nil {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return "", "", false
	}
	userRaw := make([]byte, int(userLen))
	if _, err := io.ReadFull(reader, userRaw); err != nil {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return "", "", false
	}
	passLen, err := reader.ReadByte()
	if err != nil {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return "", "", false
	}
	passRaw := make([]byte, int(passLen))
	if _, err := io.ReadFull(reader, passRaw); err != nil {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return "", "", false
	}
	return string(userRaw), string(passRaw), true
}

func readSOCKS5ConnectTarget(reader *bufio.Reader, conn net.Conn) (string, bool) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		writeSOCKS5Failure(conn, 0x01)
		return "", false
	}
	if header[0] != 0x05 || header[1] != 0x01 || header[2] != 0x00 {
		writeSOCKS5Failure(conn, 0x07)
		return "", false
	}
	var host string
	switch header[3] {
	case 0x01:
		raw := make([]byte, 4)
		if _, err := io.ReadFull(reader, raw); err != nil {
			writeSOCKS5Failure(conn, 0x01)
			return "", false
		}
		host = net.IP(raw).String()
	case 0x03:
		l, err := reader.ReadByte()
		if err != nil {
			writeSOCKS5Failure(conn, 0x01)
			return "", false
		}
		raw := make([]byte, int(l))
		if _, err := io.ReadFull(reader, raw); err != nil {
			writeSOCKS5Failure(conn, 0x01)
			return "", false
		}
		host = string(raw)
	case 0x04:
		raw := make([]byte, 16)
		if _, err := io.ReadFull(reader, raw); err != nil {
			writeSOCKS5Failure(conn, 0x01)
			return "", false
		}
		host = net.IP(raw).String()
	default:
		writeSOCKS5Failure(conn, 0x08)
		return "", false
	}
	portRaw := make([]byte, 2)
	if _, err := io.ReadFull(reader, portRaw); err != nil {
		writeSOCKS5Failure(conn, 0x01)
		return "", false
	}
	port := int(portRaw[0])<<8 | int(portRaw[1])
	return net.JoinHostPort(host, strconv.Itoa(port)), true
}

func writeSOCKS5Failure(conn net.Conn, code byte) {
	_, _ = conn.Write([]byte{0x05, code, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
}
