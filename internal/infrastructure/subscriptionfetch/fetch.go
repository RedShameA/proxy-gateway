package subscriptionfetch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

const timeout = 30 * time.Second
const maxBodyBytes = 8 * 1024 * 1024

var client = &http.Client{Timeout: timeout}

func Fetch(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("fetch subscription")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("fetch subscription status")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", errors.New("read subscription")
	}
	return string(raw), nil
}
