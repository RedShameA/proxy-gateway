package subscriptions

import (
	"context"
	"errors"
	"strings"
)

var ErrRemoteSubscriptionURLRequired = errors.New("remote subscription url required")

type ContentFetcher func(ctx context.Context, rawURL string) (string, error)

type ContentSource struct {
	SourceType string
	URL        string
	Content    string
}

func ContentForImport(ctx context.Context, source ContentSource, fetch ContentFetcher) (string, error) {
	if source.SourceType != SourceTypeRemote {
		return source.Content, nil
	}
	if strings.TrimSpace(source.URL) == "" {
		return "", ErrRemoteSubscriptionURLRequired
	}
	if fetch == nil {
		return "", errors.New("subscription fetcher unavailable")
	}
	return fetch(ctx, source.URL)
}
