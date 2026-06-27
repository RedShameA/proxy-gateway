package profiles

import (
	"errors"
	"testing"

	domainprofile "proxygateway/internal/domain/profile"
)

func TestValidateConfigNormalizesFastestProfile(t *testing.T) {
	cfg := DefaultConfig("profile_1")
	cfg.Name = "  Fastest  "
	cfg.TestURL = "example.com/probe"
	cfg.EgressCountries = []string{" jp ", "JP", "__unknown__"}
	cfg.Protocols = []string{" HTTP ", "http", "SOCKS5"}
	cfg.SourceIDs = []string{"sub_1"}
	cfg.NodeSourceMode = "selected_sources"

	err := ValidateConfig(&cfg, ConfigValidationDeps{
		DefaultTestURL: "https://default.test/generate_204",
		IdentifierExists: func(identifier, excludeProfileID string) (bool, error) {
			if identifier != "profile_1" || excludeProfileID != "profile_1" {
				t.Fatalf("identifier check = %q / %q", identifier, excludeProfileID)
			}
			return false, nil
		},
	})
	if err != nil {
		t.Fatalf("ValidateConfig = %v", err)
	}
	if cfg.Name != "Fastest" || cfg.ProfileIdentifier != "profile_1" {
		t.Fatalf("identity fields = %#v", cfg)
	}
	if cfg.TestURL != "https://example.com/probe" {
		t.Fatalf("TestURL = %q, want https://example.com/probe", cfg.TestURL)
	}
	if got := cfg.EgressCountries; len(got) != 2 || got[0] != "JP" || got[1] != "__unknown__" {
		t.Fatalf("EgressCountries = %#v", got)
	}
	if cfg.EgressCountry != "JP" {
		t.Fatalf("EgressCountry = %q, want JP", cfg.EgressCountry)
	}
	if got := cfg.Protocols; len(got) != 2 || got[0] != "http" || got[1] != "socks5" {
		t.Fatalf("Protocols = %#v", got)
	}
	if cfg.NodeSourceMode != "specific_subscriptions" || cfg.State != "running" {
		t.Fatalf("source/state = %q / %q", cfg.NodeSourceMode, cfg.State)
	}
}

func TestValidateConfigReportsDuplicateIdentifier(t *testing.T) {
	cfg := DefaultConfig("profile_1")
	cfg.Name = "Fastest"
	cfg.ProfileIdentifier = "client-a"

	err := ValidateConfig(&cfg, ConfigValidationDeps{
		IdentifierExists: func(identifier, excludeProfileID string) (bool, error) {
			return true, nil
		},
	})
	if !errors.Is(err, ErrIdentifierDuplicate) {
		t.Fatalf("ValidateConfig = %v, want ErrIdentifierDuplicate", err)
	}
}

func TestValidateConfigValidatesFixedNode(t *testing.T) {
	cfg := DefaultConfig("profile_1")
	cfg.Name = "Fixed"
	cfg.Type = "fixed_node"
	cfg.FixedNodeID = "node_missing"

	err := ValidateConfig(&cfg, ConfigValidationDeps{
		NodeExists: func(nodeID string) (bool, error) {
			if nodeID != "node_missing" {
				t.Fatalf("node check = %q", nodeID)
			}
			return false, nil
		},
	})
	if !errors.Is(err, ErrFixedNodeNotFound) {
		t.Fatalf("ValidateConfig = %v, want ErrFixedNodeNotFound", err)
	}
}

func TestValidateConfigNormalizesChainProfile(t *testing.T) {
	cfg := DefaultConfig("profile_1")
	cfg.Name = "Chain"
	cfg.Type = "chain"
	cfg.FixedNodeID = " exit_1 "
	cfg.ChainEvaluationMode = " chain_link "
	cfg.CurrentNodeID = "front_1"

	err := ValidateConfig(&cfg, ConfigValidationDeps{
		NodeExists: func(nodeID string) (bool, error) {
			return nodeID == "exit_1", nil
		},
	})
	if err != nil {
		t.Fatalf("ValidateConfig = %v", err)
	}
	if cfg.FixedNodeID != "exit_1" || len(cfg.ExitNodeIDs) != 1 || cfg.ExitNodeIDs[0] != "exit_1" {
		t.Fatalf("exit fields = %#v", cfg)
	}
	if cfg.ChainEvaluationMode != "chain_link" || cfg.CurrentExitNodeID != "exit_1" {
		t.Fatalf("chain mode/current exit = %q / %q", cfg.ChainEvaluationMode, cfg.CurrentExitNodeID)
	}
}

func TestValidateConfigRejectsMultipleChainLinkExits(t *testing.T) {
	cfg := DefaultConfig("profile_1")
	cfg.Name = "Chain"
	cfg.Type = "chain"
	cfg.ExitNodeIDs = []string{"exit_1", "exit_2"}
	cfg.ChainEvaluationMode = "chain_link"

	err := ValidateConfig(&cfg, ConfigValidationDeps{
		NodeExists: func(nodeID string) (bool, error) {
			return true, nil
		},
	})
	if !errors.Is(err, domainprofile.ErrChainLinkSingleExitRequired) {
		t.Fatalf("ValidateConfig = %v, want ErrChainLinkSingleExitRequired", err)
	}
}

func TestValidateConfigRejectsInvalidTestURL(t *testing.T) {
	cfg := DefaultConfig("profile_1")
	cfg.Name = "Fastest"
	cfg.TestURL = "ftp://example.com"

	if err := ValidateConfig(&cfg, ConfigValidationDeps{}); !errors.Is(err, ErrTestURLScheme) {
		t.Fatalf("scheme error = %v, want ErrTestURLScheme", err)
	}

	cfg.TestURL = "https://"
	if err := ValidateConfig(&cfg, ConfigValidationDeps{}); !errors.Is(err, ErrTestURLHostRequired) {
		t.Fatalf("host error = %v, want ErrTestURLHostRequired", err)
	}
}
