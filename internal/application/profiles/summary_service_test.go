package profiles

import (
	"context"
	"testing"
)

func TestSummaryServiceListsSummariesWithCountsAndCurrentPath(t *testing.T) {
	configs := newFakeConfigRepository()
	cfg := DefaultConfig("profile_1")
	cfg.Name = "Work"
	cfg.CurrentNodeID = "node_1"
	cfg.State = "ready"
	configs.records[cfg.ID] = cfg
	credentials := &detailServiceCredentialRepository{
		counts: CredentialCounts{Total: 2, Enabled: 1},
	}
	service := NewSummaryService(SummaryServiceDeps{
		Configs:     configs,
		Credentials: credentials,
		CurrentPath: func(record ConfigRecord) any {
			return map[string]any{"profile_id": record.ID}
		},
	})

	list, err := service.List(context.Background(), ListConfigFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	if list.Total != 1 || len(list.Items) != 1 || len(list.AccessProfiles) != 1 {
		t.Fatalf("list = %#v", list)
	}
	summary := list.Items[0]
	if summary.ID != cfg.ID || summary.ProxyCredentialsCount != 2 || summary.EnabledProxyCredentialsCount != 1 {
		t.Fatalf("summary identity/counts = %#v", summary)
	}
	if summary.CurrentPath == nil {
		t.Fatalf("summary CurrentPath should be populated: %#v", summary)
	}
}

func TestSummaryServiceReturnsEmptySummariesOnEmptyList(t *testing.T) {
	service := NewSummaryService(SummaryServiceDeps{Configs: newFakeConfigRepository()})

	items, err := service.ListSummaries(context.Background(), ListConfigFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("items = %#v, want empty non-nil slice", items)
	}
}
