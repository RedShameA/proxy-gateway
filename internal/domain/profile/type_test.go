package profile

import (
	"errors"
	"testing"
)

func TestNormalizeChainEvaluationMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want string
	}{
		{name: "chain link", mode: " chain_link ", want: "chain_link"},
		{name: "default", mode: "", want: "end_to_end"},
		{name: "unknown", mode: "fastest", want: "end_to_end"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeChainEvaluationMode(tt.mode); got != tt.want {
				t.Fatalf("NormalizeChainEvaluationMode(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestValidateChainExitNodes(t *testing.T) {
	if err := ValidateChainExitNodes([]string{"exit-a"}, "chain_link"); err != nil {
		t.Fatalf("single chain_link exit = %v", err)
	}
	if err := ValidateChainExitNodes([]string{"exit-a", "exit-b"}, "end_to_end"); err != nil {
		t.Fatalf("multiple end_to_end exits = %v", err)
	}
	if err := ValidateChainExitNodes(nil, "end_to_end"); !errors.Is(err, ErrExitNodesRequired) {
		t.Fatalf("missing exits = %v, want ErrExitNodesRequired", err)
	}
	if err := ValidateChainExitNodes([]string{"exit-a", "exit-b"}, "chain_link"); !errors.Is(err, ErrChainLinkSingleExitRequired) {
		t.Fatalf("multiple chain_link exits = %v, want ErrChainLinkSingleExitRequired", err)
	}
}
