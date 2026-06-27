package proxy

import (
	"errors"
	"testing"

	appprofiles "proxygateway/internal/application/profiles"
	domainprofile "proxygateway/internal/domain/profile"
)

func TestSelectPathForCredentialSelectsRandomUsableNode(t *testing.T) {
	credential := CredentialRecord{ID: "cred_1", Remark: "client", ProfileID: "profile_1"}
	cfg := appprofiles.ConfigRecord{
		ID:                "profile_1",
		Name:              "Random",
		ProfileIdentifier: "client-a",
		Type:              "random",
	}

	path, err := SelectPathForCredential(credential, cfg, PathSelectionDeps{
		CandidateNodes: func(filter domainprofile.CandidateFilter) ([]Node, error) {
			return []Node{{ID: "node_disabled"}, {ID: "node_usable", Enabled: true}}, nil
		},
		UsableNodes: func(nodes []Node) []Node {
			return nodes[1:]
		},
		RandomIndex: func(n int) (int, error) {
			if n != 1 {
				t.Fatalf("random range = %d, want 1", n)
			}
			return 0, nil
		},
	})
	if err != nil {
		t.Fatalf("SelectPathForCredential = %v", err)
	}
	if path.Credential.ID != "cred_1" || path.ProfileID != "profile_1" || path.ProfileIdentifier != "client-a" {
		t.Fatalf("path identity = %#v", path)
	}
	if path.Node.ID != "node_usable" {
		t.Fatalf("selected node = %#v", path.Node)
	}
}

func TestSelectPathForCredentialRejectsFastestNodeOutsideCandidateFilter(t *testing.T) {
	credential := CredentialRecord{ProfileID: "profile_1"}
	cfg := appprofiles.ConfigRecord{
		ID:            "profile_1",
		Name:          "Fastest",
		Type:          "fastest",
		State:         "ready",
		CurrentNodeID: "node_current",
	}

	_, err := SelectPathForCredential(credential, cfg, PathSelectionDeps{
		ProfileNodeMatchesCandidateFilter: func(profileID, nodeID string, filter domainprofile.CandidateFilter) bool {
			return false
		},
		LoadUsableNode: func(nodeID string) (Node, error) {
			t.Fatal("LoadUsableNode should not be called when filter rejects current node")
			return Node{}, nil
		},
	})
	if !errors.Is(err, ErrNoUsableProxyPath) {
		t.Fatalf("SelectPathForCredential = %v, want ErrNoUsableProxyPath", err)
	}
}

func TestSelectPathForCredentialSelectsChainPathWithSingleExitFallback(t *testing.T) {
	credential := CredentialRecord{ID: "cred_1", ProfileID: "profile_1"}
	cfg := appprofiles.ConfigRecord{
		ID:            "profile_1",
		Name:          "Chain",
		Type:          "chain",
		State:         "ready",
		CurrentNodeID: "front_1",
		ExitNodeIDs:   []string{"exit_1"},
	}

	path, err := SelectPathForCredential(credential, cfg, PathSelectionDeps{
		ChainPathMatchesProfile: func(cfg appprofiles.ConfigRecord, frontNodeID, exitNodeID string) bool {
			if frontNodeID != "front_1" || exitNodeID != "exit_1" {
				t.Fatalf("chain path = %s -> %s", frontNodeID, exitNodeID)
			}
			return true
		},
		LoadUsableNode: func(nodeID string) (Node, error) {
			return Node{ID: nodeID, Enabled: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("SelectPathForCredential = %v", err)
	}
	if path.FrontNode.ID != "front_1" || path.ExitNode.ID != "exit_1" || path.Node.ID != "" {
		t.Fatalf("chain path = %#v", path)
	}
}

func TestSelectPathForCredentialRejectsStaleChainPath(t *testing.T) {
	credential := CredentialRecord{ProfileID: "profile_1"}
	cfg := appprofiles.ConfigRecord{
		ID:                "profile_1",
		Type:              "chain",
		State:             "ready",
		CurrentNodeID:     "front_1",
		CurrentExitNodeID: "exit_1",
		ExitNodeIDs:       []string{"exit_1"},
	}

	_, err := SelectPathForCredential(credential, cfg, PathSelectionDeps{
		ChainPathMatchesProfile: func(cfg appprofiles.ConfigRecord, frontNodeID, exitNodeID string) bool {
			return false
		},
		LoadUsableNode: func(nodeID string) (Node, error) {
			t.Fatal("LoadUsableNode should not be called when chain path is stale")
			return Node{}, nil
		},
	})
	if !errors.Is(err, ErrNoUsableProxyPath) {
		t.Fatalf("SelectPathForCredential = %v, want ErrNoUsableProxyPath", err)
	}
}
