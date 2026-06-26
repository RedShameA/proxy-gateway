package nodes

import "testing"

func TestUpsertReusesExistingFingerprintAndBindsSource(t *testing.T) {
	repo := &fakeUpsertRepo{
		existingID: "node-existing",
	}
	service := Service{
		NewNodeID: func() (string, error) {
			t.Fatal("NewNodeID should not be called when fingerprint already exists")
			return "", nil
		},
	}

	id, err := service.Upsert(repo, UpsertInput{
		Fingerprint:  "fp-1",
		Name:         "shared",
		Type:         "http",
		Server:       "127.0.0.1",
		ServerPort:   19080,
		SourceID:     "sub-1",
		SourceName:   "Subscription 1",
		SourceType:   "subscription",
		OutboundJSON: `{"type":"http"}`,
		NowMillis:    123,
	})
	if err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if id != "node-existing" {
		t.Fatalf("Upsert id = %q, want node-existing", id)
	}
	if repo.created != nil {
		t.Fatalf("created record = %#v, want nil", repo.created)
	}
	if repo.bound == nil {
		t.Fatal("expected source binding")
	}
	if repo.bound.NodeID != "node-existing" || repo.bound.SourceID != "sub-1" || repo.bound.SourceType != "subscription" {
		t.Fatalf("source binding = %#v", repo.bound)
	}
}

func TestUpsertCreatesNodeForNewFingerprint(t *testing.T) {
	repo := &fakeUpsertRepo{}
	service := Service{
		NewNodeID: func() (string, error) {
			return "node-new", nil
		},
	}

	id, err := service.Upsert(repo, UpsertInput{
		Fingerprint:  "fp-new",
		Name:         "manual-new",
		Type:         "socks5",
		Server:       "127.0.0.2",
		ServerPort:   19081,
		Username:     "user",
		Password:     "pass",
		RawJSON:      "socks5://user:pass@127.0.0.2:19081#manual-new",
		OutboundJSON: `{"type":"socks"}`,
		SourceID:     "manual",
		SourceName:   "Manual",
		SourceType:   "manual",
		NowMillis:    456,
	})
	if err != nil {
		t.Fatalf("Upsert error = %v", err)
	}
	if id != "node-new" {
		t.Fatalf("Upsert id = %q, want node-new", id)
	}
	if repo.created == nil {
		t.Fatal("expected created record")
	}
	if repo.created.Name != "manual-new" || repo.created.SourceID != "manual" || repo.created.CreatedAt != 456 {
		t.Fatalf("created record = %#v", repo.created)
	}
	if repo.bound == nil || repo.bound.NodeID != "node-new" || repo.bound.DisplayName != "manual-new" {
		t.Fatalf("source binding = %#v", repo.bound)
	}
}

type fakeUpsertRepo struct {
	existingID string
	created    *CreateNodeRecord
	bound      *BindSourceRecord
}

func (f *fakeUpsertRepo) FindNodeIDByFingerprint(fingerprint string) (string, error) {
	return f.existingID, nil
}

func (f *fakeUpsertRepo) CreateNode(record CreateNodeRecord) error {
	f.created = &record
	return nil
}

func (f *fakeUpsertRepo) BindNodeSource(record BindSourceRecord) error {
	f.bound = &record
	return nil
}
