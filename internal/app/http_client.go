package app

import (
	"net/http"
	"time"
)

const externalHTTPTimeout = 30 * time.Second

var externalHTTPClient = &http.Client{Timeout: externalHTTPTimeout}

func externalHTTPGet(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return externalHTTPClient.Do(req)
}
