package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openziti/agora/internal/api"
)

func TestWhoamiEndpoint(t *testing.T) {
	t.Parallel()

	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgAlpha, alphaID, alphaToken := env.createOrgWithAccount(t, "Alpha Co", "alpha@example.com")
	orgBeta, betaID, betaToken := env.createOrgWithAccount(t, "Beta Co", "beta@example.com")

	alpha := env.accountClient(t, alphaToken)
	alphaRes, err := alpha.Whoami(env.ctx)
	if err != nil {
		t.Fatalf("alpha whoami: %v", err)
	}
	alphaAccount, ok := alphaRes.(*api.DashboardAccount)
	if !ok {
		t.Fatalf("alpha whoami unexpected response: %T", alphaRes)
	}
	assertWhoamiAccount(t, alphaAccount, orgAlpha, alphaID, "alpha@example.com", "Alpha Co")

	beta := env.accountClient(t, betaToken)
	betaRes, err := beta.Whoami(env.ctx)
	if err != nil {
		t.Fatalf("beta whoami: %v", err)
	}
	betaAccount, ok := betaRes.(*api.DashboardAccount)
	if !ok {
		t.Fatalf("beta whoami unexpected response: %T", betaRes)
	}
	assertWhoamiAccount(t, betaAccount, orgBeta, betaID, "beta@example.com", "Beta Co")

	handler, err := newControllerHTTPHandler(&Controller{cfg: env.service.cfg, store: env.store, service: env.service})
	if err != nil {
		t.Fatalf("new controller http handler: %v", err)
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, _ := doWhoamiHTTPGet(t, ts.Client(), ts.URL)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected no-cookie whoami 401, got %d", resp.StatusCode)
	}

	resp, _ = doWhoamiHTTPGet(t, ts.Client(), ts.URL, &http.Cookie{Name: sessionCookieName, Value: "bogus-token"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected bogus-cookie whoami 401, got %d", resp.StatusCode)
	}

	resp, body := doWhoamiHTTPGet(t, ts.Client(), ts.URL, &http.Cookie{Name: sessionCookieName, Value: alphaToken})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected cookie-authenticated whoami 200, got %d body=%q", resp.StatusCode, body)
	}
	var cookieAccount api.DashboardAccount
	if err := cookieAccount.UnmarshalJSON([]byte(body)); err != nil {
		t.Fatalf("decode cookie-authenticated whoami response: %v", err)
	}
	assertWhoamiAccount(t, &cookieAccount, orgAlpha, alphaID, "alpha@example.com", "Alpha Co")
}

func doWhoamiHTTPGet(t *testing.T, client *http.Client, baseURL string, cookies ...*http.Cookie) (*http.Response, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/account/whoami", nil)
	if err != nil {
		t.Fatalf("new whoami request: %v", err)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("whoami request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read whoami response: %v", err)
	}
	return resp, string(body)
}

func assertWhoamiAccount(t *testing.T, got *api.DashboardAccount, organizationID, accountID, email, organizationName string) {
	t.Helper()
	if got.AccountId != accountID {
		t.Fatalf("expected account_id %q, got %q", accountID, got.AccountId)
	}
	if got.Email != email {
		t.Fatalf("expected email %q, got %q", email, got.Email)
	}
	if got.OrganizationId != organizationID {
		t.Fatalf("expected organization_id %q, got %q", organizationID, got.OrganizationId)
	}
	if got.OrganizationName != organizationName {
		t.Fatalf("expected organization_name %q, got %q", organizationName, got.OrganizationName)
	}
	if got.Role != api.DashboardAccountRoleMember {
		t.Fatalf("expected role member, got %q", got.Role)
	}
}
