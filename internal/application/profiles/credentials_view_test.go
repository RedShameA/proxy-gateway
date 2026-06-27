package profiles

import "testing"

func TestBuildCreatedCredentialPreservesCreateResponseShape(t *testing.T) {
	credential := BuildCreatedCredential(CreatedCredentialInput{
		ID:                "cred_1",
		ProfileID:         "profile_1",
		Remark:            "client",
		Password:          "secret",
		ProfileIdentifier: "work",
		Endpoint:          "127.0.0.1:28080",
	})

	if credential.ID != "cred_1" || credential.AccessProfileID != "profile_1" || credential.Remark != "client" {
		t.Fatalf("credential identity fields = %#v", credential)
	}
	if !credential.Enabled || credential.CreatedAt != 0 || credential.LastUsedAt != nil {
		t.Fatalf("created credential defaults = %#v", credential)
	}
	if credential.HTTPProxyURL != "http://work:secret@127.0.0.1:28080" {
		t.Fatalf("HTTPProxyURL = %q", credential.HTTPProxyURL)
	}
	if credential.HTTPSProxyURL != "https://work:secret@127.0.0.1:28080" {
		t.Fatalf("HTTPSProxyURL = %q", credential.HTTPSProxyURL)
	}
	if credential.SOCKS5ProxyURL != "socks5://work:secret@127.0.0.1:28080" {
		t.Fatalf("SOCKS5ProxyURL = %q", credential.SOCKS5ProxyURL)
	}
}

func TestBuildCredentialListUsesStableAliases(t *testing.T) {
	records := []CredentialRecord{
		{
			ID:         "cred_1",
			ProfileID:  "profile_1",
			Remark:     "client",
			Password:   "secret",
			Enabled:    true,
			CreatedAt:  100,
			LastUsedAt: 200,
		},
	}

	list := BuildCredentialList(records, "work", "127.0.0.1:28080")

	if list.Total != 1 || len(list.Items) != 1 || len(list.ProxyCredentials) != 1 {
		t.Fatalf("list shape = %#v", list)
	}
	if list.Items[0] != list.ProxyCredentials[0] {
		t.Fatalf("items/proxy_credentials mismatch = %#v", list)
	}
	credential := list.ProxyCredentials[0]
	if credential.LastUsedAt != 200 || credential.CreatedAt != 100 || !credential.Enabled {
		t.Fatalf("credential fields = %#v", credential)
	}
	if credential.HTTPProxyURL != "http://work:secret@127.0.0.1:28080" {
		t.Fatalf("HTTPProxyURL = %q", credential.HTTPProxyURL)
	}
}
