package subscriptions

import (
	"context"
	"errors"
	"testing"
)

func TestContentForImportReturnsLocalContentWithoutFetcher(t *testing.T) {
	content, err := ContentForImport(context.Background(), ContentSource{
		SourceType: "local",
		Content:    "local-content",
	}, nil)
	if err != nil {
		t.Fatalf("ContentForImport() error = %v", err)
	}
	if content != "local-content" {
		t.Fatalf("content = %q", content)
	}
}

func TestContentForImportFetchesRemoteContent(t *testing.T) {
	content, err := ContentForImport(context.Background(), ContentSource{
		SourceType: "remote",
		URL:        "https://example.test/sub",
	}, func(_ context.Context, rawURL string) (string, error) {
		if rawURL != "https://example.test/sub" {
			t.Fatalf("rawURL = %q", rawURL)
		}
		return "remote-content", nil
	})
	if err != nil {
		t.Fatalf("ContentForImport() error = %v", err)
	}
	if content != "remote-content" {
		t.Fatalf("content = %q", content)
	}
}

func TestContentForImportRequiresRemoteURL(t *testing.T) {
	_, err := ContentForImport(context.Background(), ContentSource{SourceType: "remote"}, nil)
	if !errors.Is(err, ErrRemoteSubscriptionURLRequired) {
		t.Fatalf("ContentForImport() error = %v, want ErrRemoteSubscriptionURLRequired", err)
	}
}
