package proxy

import (
	"bufio"
	"io"
	"net"
	"strconv"
	"time"

	appproxy "proxygateway/internal/application/proxy"

	"go.uber.org/zap"
)

const SOCKS5HandshakeTimeout = 10 * time.Second

type SOCKS5Adapter[C any, P any] struct {
	Authenticate        func(username, password string) AuthResult[C]
	SelectPath          func(C) (P, error)
	CredentialProfileID func(C) string
	Dial                func(P, string) (net.Conn, error)
	StartRequest        func(P, string, time.Time) string
	FinishRequest       func(logID string, success bool, failureStage, errorText string, httpStatus int, durationMS, ingressBytes, egressBytes int64)
	Logger              *zap.Logger
}

func (h SOCKS5Adapter[C, P]) ServeConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(SOCKS5HandshakeTimeout))
	reader := bufio.NewReader(conn)
	version, err := reader.ReadByte()
	if err != nil || version != 0x05 {
		h.debug("socks5 handshake rejected", conn, err)
		return
	}
	methodCount, err := reader.ReadByte()
	if err != nil {
		h.debug("socks5 method count read failed", conn, err)
		return
	}
	methods := make([]byte, int(methodCount))
	if _, err := io.ReadFull(reader, methods); err != nil {
		h.debug("socks5 methods read failed", conn, err)
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
		h.warn("socks5 authentication method unsupported", conn)
		_, _ = conn.Write([]byte{0x05, 0xff})
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x02}); err != nil {
		h.debug("socks5 method selection write failed", conn, err)
		return
	}
	username, password, ok := readSOCKS5UsernamePassword(reader, conn)
	if !ok {
		h.warn("socks5 username password auth read failed", conn)
		return
	}
	auth := h.Authenticate(username, password)
	if !auth.OK {
		_, _ = conn.Write([]byte{0x01, 0x01})
		h.warnFields("socks5 authentication failed", conn, zap.String("profile_identifier", username))
		return
	}
	_, _ = conn.Write([]byte{0x01, 0x00})
	path, err := h.SelectPath(auth.Credential)
	if err != nil {
		h.warnFields("socks5 proxy path selection failed", conn,
			zap.String("profile_id", h.credentialProfileID(auth.Credential)),
			zap.Error(err),
		)
		writeSOCKS5Failure(conn, 0x01)
		return
	}
	target, ok := readSOCKS5ConnectTarget(reader, conn)
	if !ok {
		h.warn("socks5 target read failed", conn)
		return
	}
	_ = conn.SetDeadline(time.Time{})
	start := time.Now()
	logID := h.startRequest(path, target, start)
	targetConn, err := h.Dial(path, target)
	if err != nil {
		h.finishRequest(logID, false, appproxy.FailureStageDial, err.Error(), 0, elapsedMilliseconds(start), 0, 0)
		writeSOCKS5Failure(conn, 0x04)
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		_ = targetConn.Close()
		h.finishRequest(logID, false, appproxy.FailureStageProxyHandshake, "write socks5 success response", 0, elapsedMilliseconds(start), 0, 0)
		return
	}
	ingressBytes, egressBytes := PipeConns(conn, targetConn)
	h.finishRequest(logID, true, "", "", 0, elapsedMilliseconds(start), ingressBytes, egressBytes)
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

func (h SOCKS5Adapter[C, P]) credentialProfileID(credential C) string {
	if h.CredentialProfileID == nil {
		return ""
	}
	return h.CredentialProfileID(credential)
}

func (h SOCKS5Adapter[C, P]) startRequest(path P, targetHost string, startedAt time.Time) string {
	if h.StartRequest == nil {
		return ""
	}
	return h.StartRequest(path, targetHost, startedAt)
}

func (h SOCKS5Adapter[C, P]) finishRequest(logID string, success bool, failureStage, errorText string, httpStatus int, durationMS, ingressBytes, egressBytes int64) {
	if h.FinishRequest != nil {
		h.FinishRequest(logID, success, failureStage, errorText, httpStatus, durationMS, ingressBytes, egressBytes)
	}
}

func (h SOCKS5Adapter[C, P]) debug(message string, conn net.Conn, err error) {
	if h.Logger != nil {
		h.Logger.Debug(message, zap.String("remote_addr", conn.RemoteAddr().String()), zap.Error(err))
	}
}

func (h SOCKS5Adapter[C, P]) warn(message string, conn net.Conn) {
	h.warnFields(message, conn)
}

func (h SOCKS5Adapter[C, P]) warnFields(message string, conn net.Conn, fields ...zap.Field) {
	if h.Logger == nil {
		return
	}
	fields = append([]zap.Field{zap.String("remote_addr", conn.RemoteAddr().String())}, fields...)
	h.Logger.Warn(message, fields...)
}
