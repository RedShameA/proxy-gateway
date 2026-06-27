package subscriptionfetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchReturnsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("subscription content"))
	}))
	t.Cleanup(server.Close)

	content, err := Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if content != "subscription content" {
		t.Fatalf("content = %q", content)
	}
}

func TestClientHasTimeout(t *testing.T) {
	if client.Timeout <= 0 {
		t.Fatal("subscription fetch client must have a timeout")
	}
}

func TestFetchRejectsNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	_, err := Fetch(context.Background(), server.URL)
	if err == nil || err.Error() != "fetch subscription status" {
		t.Fatalf("Fetch error = %v, want fetch subscription status", err)
	}
}
