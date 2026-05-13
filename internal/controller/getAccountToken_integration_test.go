package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openziti/agora/internal/api"
)

func TestGetAccountTokenEndpoint(t *testing.T) {
	t.Parallel()

	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	_, _, alphaToken := env.createOrgWithAccount(t, "Alpha Co", "alpha@example.com")

	alpha := env.accountClient(t, alphaToken)
	res, err := alpha.GetAccountToken(env.ctx)
	if err != nil {
		t.Fatalf("alpha get account token: %v", err)
	}
	tokenRes, ok := res.(*api.AccountTokenResponse)
	if !ok {
		t.Fatalf("alpha get account token unexpected response: %T", res)
	}
	if tokenRes.AccountToken != alphaToken {
		t.Fatalf("expected account token %q, got %q", alphaToken, tokenRes.AccountToken)
	}

	handler, err := newControllerHTTPHandler(&Controller{cfg: env.service.cfg, store: env.store, service: env.service})
	if err != nil {
		t.Fatalf("new controller http handler: %v", err)
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, _ := doGetAccountTokenHTTPGet(t, ts.Client(), ts.URL)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected no-cookie get account token 401, got %d", resp.StatusCode)
	}

	resp, _ = doGetAccountTokenHTTPGet(t, ts.Client(), ts.URL, &http.Cookie{Name: sessionCookieName, Value: "bogus-token"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected bogus-cookie get account token 401, got %d", resp.StatusCode)
	}

	resp, body := doGetAccountTokenHTTPGet(t, ts.Client(), ts.URL, &http.Cookie{Name: sessionCookieName, Value: alphaToken})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected cookie-authenticated get account token 200, got %d body=%q", resp.StatusCode, body)
	}
	var cookieToken api.AccountTokenResponse
	if err := cookieToken.UnmarshalJSON([]byte(body)); err != nil {
		t.Fatalf("decode cookie-authenticated get account token response: %v", err)
	}
	if cookieToken.AccountToken != alphaToken {
		t.Fatalf("expected cookie account token %q, got %q", alphaToken, cookieToken.AccountToken)
	}
}

func doGetAccountTokenHTTPGet(t *testing.T, client *http.Client, baseURL string, cookies ...*http.Cookie) (*http.Response, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/account/token", nil)
	if err != nil {
		t.Fatalf("new get account token request: %v", err)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("get account token request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read get account token response: %v", err)
	}
	return resp, string(body)
}
