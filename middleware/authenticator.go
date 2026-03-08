package middleware

import (
	"OpenSPMRegistry/authenticator"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

type Authentication struct {
	auth  authenticator.Authenticator
	muxer *http.ServeMux
}

// NewAuthentication creates a new authentication middleware based on the provided authenticator.
// Use WrapHandler(handler, allowAuthQueryParam) to allow ?auth= for specific handlers (e.g. collection endpoints).
// Supported authenticators are: TokenAuthenticator and UsernamePasswordAuthenticator;
// every other authenticator is treated as a no-op authenticator.
func NewAuthentication(auth authenticator.Authenticator, router *http.ServeMux) *Authentication {
	a := &Authentication{
		auth:  auth,
		muxer: router,
	}

	// Register the callback handler for the token authenticator
	tokenAuth, ok := auth.(any).(authenticator.OidcAuthenticatorCode)
	if ok {
		router.HandleFunc("GET /callback", tokenAuth.Callback)
	}
	oidcAuth, ok := auth.(any).(authenticator.OidcAuthenticator)
	if ok {
		router.HandleFunc("GET /login", oidcAuth.Login)
	}

	return a
}

func (a *Authentication) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.muxer.ServeHTTP(w, r)
}

func (a *Authentication) HandleFunc(pattern string, handler http.HandlerFunc) {
	a.muxer.HandleFunc(pattern, a.authenticate(handler, false))
}

// WrapHandler returns the handler wrapped with authentication (for use on another mux).
// When allowAuthQueryParam is true, requests may send credentials via ?auth=<base64(Authorization)>;
// decoded value must start with "Basic " or "Bearer ". Pass true only for handlers that require it (e.g. collection endpoints).
func (a *Authentication) WrapHandler(h http.HandlerFunc, allowAuthQueryParam bool) http.HandlerFunc {
	return a.authenticate(h, allowAuthQueryParam)
}

// authenticate is a middleware that checks if the request is authorized
// based on the provided authenticator
// if the request is not authorized, it returns a 401 status code
// if the request is authorized, it calls the next handler
func (a *Authentication) authenticate(next http.HandlerFunc, allowAuthQueryParam bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if allowAuthQueryParam && r.Header.Get("Authorization") == "" {
			if authParam := strings.TrimSpace(r.URL.Query().Get("auth")); authParam != "" {
				if decoded, err := base64.StdEncoding.DecodeString(authParam); err == nil {
					s := string(decoded)
					if strings.HasPrefix(s, "Basic ") || strings.HasPrefix(s, "Bearer ") {
						r.Header.Set("Authorization", s)
					}
				}
			}
		}
		_, err := a.auth.Authenticate(w, r)
		if err != nil {
			writeAuthorizationHeaderError(w, err)
			return
		}

		if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
			slog.Debug("Request authorized")
		}
		// Once authorization checked, call the next handler
		next.ServeHTTP(w, r)
	}
}

func writeAuthorizationHeaderError(w http.ResponseWriter, err error) {
	slog.Error("Error parsing authorization header:", "error", err)
	http.Error(w, fmt.Sprintf("Authentication failed: %s", err), http.StatusUnauthorized)
}
