package nodes

import "testing"

func TestBuildUpsertInputNormalizesOutboundAndSource(t *testing.T) {
	input, err := BuildUpsertInput(OutboundNode{
		Name:       "manual",
		Type:       "socks",
		Server:     "127.0.0.1",
		ServerPort: 1080,
		Username:   "user",
		Password:   "pass",
	}, SourceInput{ID: "manual", Name: "Manual", Type: "manual"}, 1234)
	if err != nil {
		t.Fatal(err)
	}

	if input.Type != "socks5" || input.SourceID != "manual" || input.SourceName != "Manual" || input.SourceType != "manual" {
		t.Fatalf("input identity = %#v", input)
	}
	if input.OutboundJSON == "" || input.Fingerprint != OutboundFingerprint(input.OutboundJSON) {
		t.Fatalf("outbound/fingerprint = %#v", input)
	}
	if input.NowMillis != 1234 {
		t.Fatalf("NowMillis = %d", input.NowMillis)
	}
}

func TestBuildManualUpdateInputPreservesEnabledPointer(t *testing.T) {
	enabled := false

	input, err := BuildManualUpdateInput("node_1", OutboundNode{
		Name:       "manual",
		Type:       "http",
		Server:     "127.0.0.1",
		ServerPort: 18080,
	}, &enabled, 5678)
	if err != nil {
		t.Fatal(err)
	}

	if input.NodeID != "node_1" || input.Enabled == nil || *input.Enabled != false {
		t.Fatalf("manual update input = %#v", input)
	}
	if input.OutboundJSON == "" || input.Fingerprint != OutboundFingerprint(input.OutboundJSON) {
		t.Fatalf("outbound/fingerprint = %#v", input)
	}
}
