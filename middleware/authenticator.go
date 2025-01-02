package middleware

import (
	"OpenSPMRegistry/authenticator"
	"fmt"
	"log/slog"
	"net/http"
)

type Authentication struct {
	auth  authenticator.Authenticator
	muxer *http.ServeMux
}

// NewAuthentication creates a new authentication middleware
// based on the provided authenticator
// supported authenticators are: TokenAuthenticator and UsernamePasswordAuthenticator
// every other authenticator will be treated as a no-op authenticator
func NewAuthentication(auth authenticator.Authenticator, router *http.ServeMux) *Authentication {
	a := &Authentication{
		auth:  auth,
		muxer: router,
	}

	// Register the callback handler for the token authenticator
	tokenAuth, ok := auth.(interface{}).(authenticator.OidcAuthenticatorCode)
	if ok {
		router.HandleFunc("GET /callback", tokenAuth.Callback)
	}
	oidcAuth, ok := auth.(interface{}).(authenticator.OidcAuthenticator)
	if ok {
		router.HandleFunc("GET /login", oidcAuth.Login)
	}

	return a
}

func (a *Authentication) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.muxer.ServeHTTP(w, r)
}

func (a *Authentication) HandleFunc(pattern string, handler http.HandlerFunc) {
	a.muxer.HandleFunc(pattern, a.authenticate(handler))
}

// authenticate is a middleware that checks if the request is authorized
// based on the provided authenticator
// if the request is not authorized, it returns a 401 status code
// if the request is authorized, it calls the next handler
func (a *Authentication) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err, _ := a.auth.Authenticate(w, r)
		if err != nil {
			writeAuthorizationHeaderError(w, err)
			return
		}

		if slog.Default().Enabled(nil, slog.LevelDebug) {
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
