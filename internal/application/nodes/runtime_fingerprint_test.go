package nodes

import "testing"

func TestRuntimeFingerprintUsesStoredOutboundJSON(t *testing.T) {
	record := Record{
		Type:         "http",
		Server:       "ignored.example",
		ServerPort:   80,
		OutboundJSON: `{"type":"http","server":"127.0.0.1","server_port":18080}`,
	}

	got := RuntimeFingerprint(record)
	want := OutboundFingerprint(record.OutboundJSON)
	if got != want {
		t.Fatalf("RuntimeFingerprint = %q, want %q", got, want)
	}
}

func TestRuntimeFingerprintBuildsOutboundWhenMissing(t *testing.T) {
	record := Record{
		Type:       "socks5",
		Server:     "127.0.0.1",
		ServerPort: 1080,
		Username:   "user",
		Password:   "pass",
	}

	got := RuntimeFingerprint(record)
	outbound, err := NormalizeOutboundJSON(OutboundNode{
		Type:       "socks5",
		Server:     "127.0.0.1",
		ServerPort: 1080,
		Username:   "user",
		Password:   "pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != OutboundFingerprint(outbound) {
		t.Fatalf("RuntimeFingerprint = %q, want fingerprint of %s", got, outbound)
	}
}
