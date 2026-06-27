package app

import (
	"net"
	"testing"
)

func TestUpdateManualNodeInvalidatesOldRuntimeFingerprint(t *testing.T) {
	g := NewForTest(t)
	if engine, ok := g.protocolEngine.(closeableNodeProtocolEngine); ok {
		_ = engine.Close()
	}
	fake := &trackingNodeProtocolEngine{}
	g.protocolEngine = fake

	created, err := g.createNodeSource(nodeCreateInput{
		Name:       "manual-original",
		Type:       "http",
		Server:     "127.0.0.1",
		ServerPort: 19083,
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeID, _ := created["id"].(string)
	if nodeID == "" {
		t.Fatalf("created node response = %#v", created)
	}
	original, err := g.loadNode(nodeID)
	if err != nil {
		t.Fatal(err)
	}
	oldFingerprint := runtimeFingerprintForNode(original)
	if oldFingerprint == "" {
		t.Fatal("expected non-empty runtime fingerprint")
	}

	_, split, err := g.updateManualNode(nodeID, nodePatchRequest{
		Name:       stringPtr("manual-updated"),
		Type:       stringPtr("socks5"),
		Server:     stringPtr("127.0.0.2"),
		ServerPort: intPtr(19084),
	})
	if err != nil {
		t.Fatal(err)
	}
	if split {
		t.Fatalf("split = true, want false")
	}
	if len(fake.invalidated) != 1 || fake.invalidated[0] != oldFingerprint {
		t.Fatalf("invalidated fingerprints = %#v, want [%q]", fake.invalidated, oldFingerprint)
	}
}

type trackingNodeProtocolEngine struct {
	invalidated []string
}

func (t *trackingNodeProtocolEngine) DialNode(node nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	panic("unexpected DialNode call")
}

func (t *trackingNodeProtocolEngine) DialChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	panic("unexpected DialChain call")
}

func (t *trackingNodeProtocolEngine) Close() error {
	return nil
}

func (t *trackingNodeProtocolEngine) InvalidateFingerprint(fingerprint string) {
	t.invalidated = append(t.invalidated, fingerprint)
}

func stringPtr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}
