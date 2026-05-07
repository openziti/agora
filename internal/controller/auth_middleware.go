package controller

import (
	"crypto/subtle"
	"net/http"

	"github.com/michaelquigley/df/dl"
)

const (
	accountTokenHeader = "X-TOKEN"
	csrfHeader         = "X-CSRF-Token"
	sessionCookieName  = "agora-session"
	csrfCookieName     = "agora-csrf"
)

var optionalPrincipalAttachPaths = map[string]bool{
	"/v1/account/logout": true,
}

var csrfSkipPaths = map[string]bool{
	"/v1/account/login":  true,
	"/v1/account/logout": true,
}

func cookieToHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(accountTokenHeader) != "" {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		clone := r.Clone(r.Context())
		clone.Header = r.Header.Clone()
		clone.Header.Set(accountTokenHeader, cookie.Value)
		next.ServeHTTP(w, clone)
	})
}

func principalAttachMiddleware(svc *Service, paths map[string]bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !paths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			token := r.Header.Get(accountTokenHeader)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			ctx, err := svc.attachAccountPrincipal(r.Context(), token)
			if err != nil {
				dl.Warnf("optional account principal attachment failed path='%s': %v", r.URL.Path, err)
				next.ServeHTTP(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func csrfMiddleware(skipPaths map[string]bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) || skipPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			if _, err := r.Cookie(sessionCookieName); err != nil {
				next.ServeHTTP(w, r)
				return
			}

			csrfCookie, err := r.Cookie(csrfCookieName)
			if err != nil || !csrfTokensMatch(csrfCookie.Value, r.Header.Get(csrfHeader)) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"csrf_mismatch"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func csrfTokensMatch(cookieValue, headerValue string) bool {
	if cookieValue == "" || headerValue == "" || len(cookieValue) != len(headerValue) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieValue), []byte(headerValue)) == 1
}
