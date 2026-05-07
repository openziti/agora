package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCookieToHeaderMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		header     string
		cookie     string
		wantHeader string
	}{
		{
			name:       "cookie present sets header",
			cookie:     "cookie-token",
			wantHeader: "cookie-token",
		},
		{
			name:       "cookie absent leaves header unset",
			wantHeader: "",
		},
		{
			name:       "header preserved over cookie",
			header:     "header-token",
			cookie:     "cookie-token",
			wantHeader: "header-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotHeader string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeader = r.Header.Get(accountTokenHeader)
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/summary", nil)
			if tt.header != "" {
				req.Header.Set(accountTokenHeader, tt.header)
			}
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tt.cookie})
			}
			rr := httptest.NewRecorder()

			cookieToHeaderMiddleware(next).ServeHTTP(rr, req)

			if rr.Code != http.StatusNoContent {
				t.Fatalf("expected forwarded response 204, got %d", rr.Code)
			}
			if gotHeader != tt.wantHeader {
				t.Fatalf("expected header %q, got %q", tt.wantHeader, gotHeader)
			}
		})
	}
}

func TestPrincipalAttachMiddleware(t *testing.T) {
	t.Parallel()

	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgID, accountID, token := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	middleware := principalAttachMiddleware(env.service, optionalPrincipalAttachPaths)

	tests := []struct {
		name          string
		path          string
		token         string
		wantPrincipal bool
	}{
		{
			name:          "allowlisted path with valid token attaches principal",
			path:          "/v1/account/logout",
			token:         token,
			wantPrincipal: true,
		},
		{
			name:  "allowlisted path with invalid token forwards without principal",
			path:  "/v1/account/logout",
			token: "not-a-real-token",
		},
		{
			name: "allowlisted path with no token forwards without principal",
			path: "/v1/account/logout",
		},
		{
			name:  "non-allowlisted path is a no-op",
			path:  "/v1/dashboard/summary",
			token: token,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPrincipal *accountPrincipal
			var gotPrincipalErr error
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPrincipal, gotPrincipalErr = requireAccountPrincipal(r.Context())
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			if tt.token != "" {
				req.Header.Set(accountTokenHeader, tt.token)
			}
			rr := httptest.NewRecorder()

			middleware(next).ServeHTTP(rr, req)

			if rr.Code != http.StatusNoContent {
				t.Fatalf("expected forwarded response 204, got %d", rr.Code)
			}
			if tt.wantPrincipal {
				if gotPrincipalErr != nil {
					t.Fatalf("expected principal, got error: %v", gotPrincipalErr)
				}
				if gotPrincipal.AccountID != accountID || gotPrincipal.OrganizationID != orgID || gotPrincipal.Email != "alice@example.com" {
					t.Fatalf("unexpected principal: %#v", gotPrincipal)
				}
				return
			}
			if gotPrincipalErr == nil || gotPrincipal != nil {
				t.Fatalf("expected no principal, got principal=%#v err=%v", gotPrincipal, gotPrincipalErr)
			}
		})
	}
}

func TestCSRFMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		method        string
		path          string
		sessionCookie string
		csrfCookie    string
		csrfHeader    string
		wantStatus    int
		wantForwarded bool
	}{
		{
			name:          "safe method passes through",
			method:        http.MethodGet,
			path:          "/v1/sessions/ses_123/close",
			sessionCookie: "session-token",
			wantStatus:    http.StatusNoContent,
			wantForwarded: true,
		},
		{
			name:          "login path passes through",
			method:        http.MethodPost,
			path:          "/v1/account/login",
			sessionCookie: "session-token",
			wantStatus:    http.StatusNoContent,
			wantForwarded: true,
		},
		{
			name:          "logout path passes through",
			method:        http.MethodPost,
			path:          "/v1/account/logout",
			sessionCookie: "session-token",
			wantStatus:    http.StatusNoContent,
			wantForwarded: true,
		},
		{
			name:          "no session cookie passes through",
			method:        http.MethodPost,
			path:          "/v1/sessions/ses_123/close",
			wantStatus:    http.StatusNoContent,
			wantForwarded: true,
		},
		{
			name:          "matching csrf passes through",
			method:        http.MethodPost,
			path:          "/v1/sessions/ses_123/close",
			sessionCookie: "session-token",
			csrfCookie:    "csrf-token",
			csrfHeader:    "csrf-token",
			wantStatus:    http.StatusNoContent,
			wantForwarded: true,
		},
		{
			name:          "mismatched csrf rejects",
			method:        http.MethodPost,
			path:          "/v1/sessions/ses_123/close",
			sessionCookie: "session-token",
			csrfCookie:    "csrf-token",
			csrfHeader:    "wrong-token",
			wantStatus:    http.StatusForbidden,
		},
		{
			name:          "missing csrf cookie rejects",
			method:        http.MethodPost,
			path:          "/v1/sessions/ses_123/close",
			sessionCookie: "session-token",
			csrfHeader:    "csrf-token",
			wantStatus:    http.StatusForbidden,
		},
		{
			name:          "missing csrf header rejects",
			method:        http.MethodPost,
			path:          "/v1/sessions/ses_123/close",
			sessionCookie: "session-token",
			csrfCookie:    "csrf-token",
			wantStatus:    http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var forwarded bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				forwarded = true
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.sessionCookie != "" {
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tt.sessionCookie})
			}
			if tt.csrfCookie != "" {
				req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: tt.csrfCookie})
			}
			if tt.csrfHeader != "" {
				req.Header.Set(csrfHeader, tt.csrfHeader)
			}
			rr := httptest.NewRecorder()

			csrfMiddleware(csrfSkipPaths)(next).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
			if forwarded != tt.wantForwarded {
				t.Fatalf("expected forwarded=%v, got %v", tt.wantForwarded, forwarded)
			}
			if tt.wantStatus == http.StatusForbidden && rr.Body.String() != `{"error":"csrf_mismatch"}` {
				t.Fatalf("unexpected csrf response body: %q", rr.Body.String())
			}
		})
	}
}
