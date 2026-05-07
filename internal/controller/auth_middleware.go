package controller

import (
	"net/http"

	"github.com/michaelquigley/df/dl"
)

const (
	accountTokenHeader = "X-TOKEN"
	sessionCookieName  = "agora-session"
)

var optionalPrincipalAttachPaths = map[string]bool{
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
