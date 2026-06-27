package profiles

import (
	"reflect"
	"testing"
)

func TestBuildSummaryAppliesStableReadModelDefaults(t *testing.T) {
	currentPath := map[string]any{"path_type": "single"}

	summary := BuildSummary(SummaryInput{
		ID:                      "profile_1",
		Name:                    "default",
		Type:                    "fixed_node",
		State:                   "ready",
		CurrentNodeID:           "node_1",
		NodeStickyEnabled:       true,
		ConfigVersion:           3,
		CurrentPath:             currentPath,
		ProxyCredentialsCount:   2,
		EnabledCredentialsCount: 1,
	})

	if summary.ProfileIdentifier != "profile_1" {
		t.Fatalf("ProfileIdentifier = %q, want profile_1", summary.ProfileIdentifier)
	}
	if summary.NodeStickyEnabled {
		t.Fatalf("NodeStickyEnabled = true, want false for fixed_node")
	}
	if !reflect.DeepEqual(summary.CurrentPath, currentPath) {
		t.Fatalf("CurrentPath = %#v, want supplied path", summary.CurrentPath)
	}
	if summary.ProxyCredentialsCount != 2 || summary.EnabledProxyCredentialsCount != 1 {
		t.Fatalf("credential counts = %d/%d, want 2/1", summary.ProxyCredentialsCount, summary.EnabledProxyCredentialsCount)
	}
	if summary.LastEvaluatedAt != nil {
		t.Fatalf("LastEvaluatedAt = %#v, want nil", summary.LastEvaluatedAt)
	}
}

func TestBuildSummaryListUsesStableAliases(t *testing.T) {
	summary := Summary{ID: "profile_1", Name: "default"}

	list := BuildSummaryList([]Summary{summary}, 3)

	if list.Total != 3 || len(list.Items) != 1 || len(list.AccessProfiles) != 1 {
		t.Fatalf("summary list shape = %#v", list)
	}
	if !reflect.DeepEqual(list.Items, list.AccessProfiles) {
		t.Fatalf("summary list aliases differ: %#v", list)
	}
}

func TestBuildSummaryListReturnsEmptySlices(t *testing.T) {
	list := BuildSummaryList(nil, 0)

	if list.Items == nil || list.AccessProfiles == nil {
		t.Fatalf("summary list slices = %#v, want non-nil empty slices", list)
	}
}
