package proxy

import (
	"encoding/base64"
	"strings"
)

func ParseBasicProxyAuthorization(auth string) (string, string, bool) {
	fields := strings.Fields(auth)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Basic") {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return "", "", false
	}
	username, password, ok := strings.Cut(string(raw), ":")
	if !ok {
		return "", "", false
	}
	return username, password, true
}
