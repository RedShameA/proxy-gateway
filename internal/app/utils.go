package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func randomToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func prefixedID(prefix string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	return prefix + "_" + token[:16], nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func unmarshalStringSlice(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	if values == nil {
		return []string{}
	}
	return values
}

func normalizeStringList(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func normalizeUpperStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalizeStringList(normalized)
}

func normalizeEgressCountryValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "__unknown__") {
		return "__unknown__"
	}
	return strings.ToUpper(value)
}

func normalizeEgressCountryList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeEgressCountryValue(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalizeStringList(normalized)
}

func normalizeLowerStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalizeStringList(normalized)
}

func stringSliceJSON(values []string) string {
	if values == nil {
		values = []string{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func stringInSlice(value string, values []string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
