//go:build !no_agora_ui

package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ctrlcfg "github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
	"github.com/openziti/agora/internal/persistence/testutil"
)

func TestControllerAuthMiddlewareStackHTTPFlowAndRouting(t *testing.T) {
	t.Parallel()

	env, cleanup := newAuthStackTestEnv(t)
	defer cleanup()

	client := env.ts.Client()

	resp, body := doAuthStackRequest(t, client, http.MethodGet, env.ts.URL+"/health", "")
	if resp.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("expected health 200 ok, got status=%d body=%q", resp.StatusCode, body)
	}
	assertNoAuthSetCookies(t, resp)

	resp, body = doAuthStackRequest(t, client, http.MethodGet, env.ts.URL+"/ready", "")
	if resp.StatusCode != http.StatusOK || body != "ready" {
		t.Fatalf("expected ready 200 ready, got status=%d body=%q", resp.StatusCode, body)
	}
	assertNoAuthSetCookies(t, resp)

	for _, path := range []string{"/", "/some-spa-route"} {
		resp, body = doAuthStackRequest(t, client, http.MethodGet, env.ts.URL+path, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("expected %s content type text/html, got %q", path, ct)
		}
		if !strings.Contains(body, `<div id="root"></div>`) {
			t.Fatalf("expected %s to return SPA index.html, got %q", path, body)
		}
	}

	resp, _ = doAuthStackRequest(t, client, http.MethodGet, env.ts.URL+"/v1/sessions", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated sessions request to return 401, got %d", resp.StatusCode)
	}
	assertNoAuthSetCookies(t, resp)

	orgID, accountID, accountToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")

	resp, _ = doAuthStackRequest(
		t,
		client,
		http.MethodPost,
		env.ts.URL+"/v1/account/login",
		`{"email":"alice@example.com","password":"test-password-1"}`,
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected login 200, got %d", resp.StatusCode)
	}
	loginCookies := cookiesByName(resp.Cookies())
	sessionCookie := loginCookies[sessionCookieName]
	csrfCookie := loginCookies[csrfCookieName]
	if sessionCookie == nil || csrfCookie == nil {
		t.Fatalf("expected login to set auth cookies, got session=%#v csrf=%#v", sessionCookie, csrfCookie)
	}
	if sessionCookie.Value != accountToken {
		t.Fatalf("expected session cookie to match account token")
	}
	if csrfCookie.Value == "" || csrfCookie.Value == sessionCookie.Value {
		t.Fatalf("unexpected csrf cookie value %q", csrfCookie.Value)
	}

	resp, _ = doAuthStackRequest(t, client, http.MethodGet, env.ts.URL+"/v1/sessions", "", sessionCookie, csrfCookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected cookie-authenticated sessions request to return 200, got %d", resp.StatusCode)
	}

	resp, body = doAuthStackRequest(t, client, http.MethodPost, env.ts.URL+"/v1/account/logout", "", sessionCookie, csrfCookie)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(body) != "" {
		t.Fatalf("expected logout 200 with empty body, got status=%d body=%q", resp.StatusCode, body)
	}
	assertClearedAuthCookies(t, resp, false)
	logoutEvents := auditEventsByType(t, env, persistence.AuditEventAccountLogout)
	assertOnePartyAuditEvent(t, logoutEvents, orgID, accountID)

	resp, body = doAuthStackRequest(t, client, http.MethodPost, env.ts.URL+"/v1/account/logout", "")
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(body) != "" {
		t.Fatalf("expected no-session logout 200 with empty body, got status=%d body=%q", resp.StatusCode, body)
	}
	assertClearedAuthCookies(t, resp, false)
	if got := len(auditEventsByType(t, env, persistence.AuditEventAccountLogout)); got != 1 {
		t.Fatalf("expected second logout with no session to skip audit, got %d rows", got)
	}
}

func newAuthStackTestEnv(t *testing.T) (*workgroupTestEnv, func()) {
	t.Helper()
	ctx := context.Background()

	store, err := persistence.Open(ctx, persistence.Config{
		DSN:             testutil.StartPostgres(t),
		MaxOpenConns:    4,
		MaxIdleConns:    4,
		ConnMaxLifetime: time.Hour,
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := persistence.MigrateUp(ctx, store); err != nil {
		_ = store.Close()
		t.Fatalf("migrate up: %v", err)
	}

	cfg := ctrlcfg.DefaultConfig()
	cfg.AdminTokens = []string{"admin-token"}

	service := NewService(cfg, store)
	envLifecycle := &fakeEnvironmentLifecycle{enableResult: &automation.ProvisionedEnvironment{IdentityID: "ziti-env-1", EnrollmentJSON: []byte(`{}`), PolicyID: "erp-1"}}
	tunnelLC := &fakeTunnelLifecycle{}
	service.lifecycleFactory = func(context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		return envLifecycle, tunnelLC, nil
	}
	handler, err := newControllerHTTPHandler(&Controller{cfg: cfg, store: store, service: service})
	if err != nil {
		_ = store.Close()
		t.Fatalf("new controller http handler: %v", err)
	}
	ts := httptest.NewServer(handler)

	cleanup := func() {
		ts.Close()
		_ = store.Close()
	}

	return &workgroupTestEnv{
		ctx:     ctx,
		store:   store,
		service: service,
		ts:      ts,
		baseURL: ts.URL + "/v1",
	}, cleanup
}

func doAuthStackRequest(t *testing.T, client *http.Client, method, url, body string, cookies ...*http.Cookie) (*http.Response, string) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, string(respBody)
}

func assertNoAuthSetCookies(t *testing.T, resp *http.Response) {
	t.Helper()
	headers := resp.Header.Values("Set-Cookie")
	if session := setCookieHeader(headers, sessionCookieName); session != "" {
		t.Fatalf("expected no session Set-Cookie header, got %q", session)
	}
	if csrf := setCookieHeader(headers, csrfCookieName); csrf != "" {
		t.Fatalf("expected no csrf Set-Cookie header, got %q", csrf)
	}
}

func assertClearedAuthCookies(t *testing.T, resp *http.Response, secure bool) {
	t.Helper()

	cookies := cookiesByName(resp.Cookies())
	sessionCookie := cookies[sessionCookieName]
	csrfCookie := cookies[csrfCookieName]
	if sessionCookie == nil || csrfCookie == nil {
		t.Fatalf("expected cleared auth cookies, got session=%#v csrf=%#v", sessionCookie, csrfCookie)
	}
	if sessionCookie.Value != "" || csrfCookie.Value != "" {
		t.Fatalf("expected cleared cookie values, got session=%q csrf=%q", sessionCookie.Value, csrfCookie.Value)
	}
	if sessionCookie.MaxAge >= 0 || csrfCookie.MaxAge >= 0 {
		t.Fatalf("expected clearing max-age, got session=%d csrf=%d", sessionCookie.MaxAge, csrfCookie.MaxAge)
	}
	if sessionCookie.Secure != secure || csrfCookie.Secure != secure {
		t.Fatalf("expected secure=%v, got session=%v csrf=%v", secure, sessionCookie.Secure, csrfCookie.Secure)
	}

	headers := resp.Header.Values("Set-Cookie")
	for _, name := range []string{sessionCookieName, csrfCookieName} {
		header := setCookieHeader(headers, name)
		if header == "" {
			t.Fatalf("expected Set-Cookie header for %q in %#v", name, headers)
		}
		if !strings.Contains(header, "Max-Age=0") {
			t.Fatalf("expected %q to contain Max-Age=0, got %q", name, header)
		}
	}
}
