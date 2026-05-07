package controller

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
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

var loginCookieEmitPaths = map[string]bool{
	"/v1/account/login":            true,
	"/v1/account/regenerate-token": true,
}

var logoutCookieClearPaths = map[string]bool{
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

func loginCookieEmitMiddleware(secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || !loginCookieEmitPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			recorder := newBufferedResponseWriter()
			next.ServeHTTP(recorder, r)

			copyHeaders(w.Header(), recorder.Header())
			if recorder.StatusCode() == http.StatusOK {
				if token := accountTokenFromBody(recorder.Body()); token != "" {
					setSessionCookies(w, token, secure)
				}
			}
			w.WriteHeader(recorder.StatusCode())
			_, _ = w.Write(recorder.Body())
		})
	}
}

func logoutCookieClearMiddleware(secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || !logoutCookieClearPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			recorder := newBufferedResponseWriter()
			next.ServeHTTP(recorder, r)

			copyHeaders(w.Header(), recorder.Header())
			if recorder.StatusCode() == http.StatusOK {
				clearSessionCookies(w, secure)
			}
			w.WriteHeader(recorder.StatusCode())
			_, _ = w.Write(recorder.Body())
		})
	}
}

func setSessionCookies(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    newToken(),
		Path:     "/",
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookies(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func accountTokenFromBody(body []byte) string {
	var parsed struct {
		AccountToken string `json:"accountToken"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	return parsed.AccountToken
}

type bufferedResponseWriter struct {
	header     http.Header
	statusCode int
	body       bytes.Buffer
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{header: http.Header{}}
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.header
}

func (w *bufferedResponseWriter) WriteHeader(statusCode int) {
	if w.statusCode == 0 {
		w.statusCode = statusCode
	}
}

func (w *bufferedResponseWriter) Write(body []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.body.Write(body)
}

func (w *bufferedResponseWriter) StatusCode() int {
	if w.statusCode == 0 {
		return http.StatusOK
	}
	return w.statusCode
}

func (w *bufferedResponseWriter) Body() []byte {
	return w.body.Bytes()
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
}
