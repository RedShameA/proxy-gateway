package nodes

import "testing"

func TestNormalizeNodeTypeAppliesSupportedAliases(t *testing.T) {
	tests := map[string]string{
		" socks ": "socks5",
		"https":   "http",
		"SS":      "shadowsocks",
		"hy2":     "hysteria2",
	}
	for input, want := range tests {
		if got := NormalizeNodeType(input); got != want {
			t.Fatalf("NormalizeNodeType(%q) = %q, want %q", input, got, want)
		}
	}
}
