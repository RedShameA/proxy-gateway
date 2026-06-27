package app_test

import (
	"net/http"
	"net/http/httptest"
	"proxygateway/internal/app"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
	"proxygateway/internal/testsupport/apptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestFirstRunAdminCredentialFlow(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	var setupStatus struct {
		RequiresSetup bool `json:"requires_setup"`
		Build         struct {
			Version  string `json:"version"`
			Revision string `json:"revision"`
			Source   string `json:"source"`
			License  string `json:"license"`
		} `json:"build"`
	}
	getJSON(t, srv.URL+"/api/system/setup-status", "", &setupStatus)
	if !setupStatus.RequiresSetup {
		t.Fatal("fresh data store should require setup")
	}
	if setupStatus.Build.Version == "" || setupStatus.Build.Revision == "" || setupStatus.Build.Source == "" || setupStatus.Build.License != "AGPL-3.0-or-later" {
		t.Fatalf("setup status build info = %#v", setupStatus.Build)
	}

	setupResp := postJSON(t, srv.URL+"/api/admin/setup", "", map[string]string{
		"password": "correct horse battery staple",
	})
	if setupResp.StatusCode != http.StatusOK {
		t.Fatalf("setup status = %d", setupResp.StatusCode)
	}
	var setupBody struct {
		Token string `json:"token"`
	}
	decodeJSON(t, setupResp, &setupBody)
	if setupBody.Token == "" {
		t.Fatal("setup should return an admin token")
	}

	meResp := get(t, srv.URL+"/api/admin/me", "")
	if meResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/admin/me status = %d", meResp.StatusCode)
	}
	_ = meResp.Body.Close()

	var me struct {
		Authenticated bool `json:"authenticated"`
	}
	getJSON(t, srv.URL+"/api/admin/me", setupBody.Token, &me)
	if !me.Authenticated {
		t.Fatal("authenticated /api/admin/me should return authenticated: true")
	}

	var afterSetup struct {
		RequiresSetup bool `json:"requires_setup"`
	}
	getJSON(t, srv.URL+"/api/system/setup-status", "", &afterSetup)
	if afterSetup.RequiresSetup {
		t.Fatal("store should not require setup after admin is created")
	}
}

func TestAdminPasswordChangeInvalidatesAllSessions(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	firstToken := setupAdmin(t, srv.URL)

	var secondLogin struct {
		Token string `json:"token"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/admin/login", "", map[string]string{
		"password": "correct horse battery staple",
	}), &secondLogin)
	if secondLogin.Token == "" {
		t.Fatal("second login token is empty")
	}

	decodeOK(t, postJSON(t, srv.URL+"/api/admin/password", firstToken, map[string]string{
		"current_password": "correct horse battery staple",
		"new_password":     "new secure password here",
	}), &struct{}{})

	for _, token := range []string{firstToken, secondLogin.Token} {
		resp := get(t, srv.URL+"/api/admin/me", token)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("old token status = %d, want 401", resp.StatusCode)
		}
		_ = resp.Body.Close()
	}

	resp := postJSON(t, srv.URL+"/api/admin/login", "", map[string]string{
		"password": "new secure password here",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new password login status = %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAdminSessionExpires(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	gw, err := app.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	token := setupAdmin(t, srv.URL)

	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	expired := time.Now().Add(-31 * 24 * time.Hour).UnixMilli()
	if _, err := db.Exec(`UPDATE admin_sessions SET created_at = ?`, expired); err != nil {
		t.Fatal(err)
	}

	resp := get(t, srv.URL+"/api/admin/me", token)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired session status = %d, want 401", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAdminLoginRateLimit(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	setupAdmin(t, srv.URL)

	for i := 0; i < 5; i++ {
		resp := postJSON(t, srv.URL+"/api/admin/login", "", map[string]string{
			"password": "wrong password",
		})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("failed login %d status = %d, want 401", i+1, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
	resp := postJSON(t, srv.URL+"/api/admin/login", "", map[string]string{
		"password": "wrong password",
	})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate limited login status = %d, want 429", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAdminCredentialStoresOnlyPasswordHash(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	gw, err := app.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	password := "correct horse battery staple"
	setupResp := postJSON(t, srv.URL+"/api/admin/setup", "", map[string]string{
		"password": password,
	})
	decodeOK(t, setupResp, &struct{}{})

	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var passwordHash string
	if err := db.QueryRow(`SELECT password_hash FROM admin_credentials LIMIT 1`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if passwordHash == password || strings.Contains(passwordHash, password) {
		t.Fatalf("admin password was stored in plaintext-like form: %q", passwordHash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		t.Fatalf("stored admin password is not a valid bcrypt hash: %v", err)
	}
}
