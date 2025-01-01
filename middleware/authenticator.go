package middleware

import (
	"OpenSPMRegistry/authenticator"
	"OpenSPMRegistry/utils"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/coreos/go-oidc/v3/oidc"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Authentication struct {
	auth  interface{}
	muxer *http.ServeMux
}

// NewAuthentication creates a new authentication middleware
// based on the provided authenticator
// supported authenticators are: TokenAuthenticator and UsernamePasswordAuthenticator
// every other authenticator will be treated as a no-op authenticator
func NewAuthentication(auth interface{}, router *http.ServeMux) *Authentication {
	return &Authentication{
		auth:  auth,
		muxer: router,
	}
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
		// Check if the request is authorized
		authorizationHeader := r.Header.Get("Authorization")

		tokenAuthentication, isTokenAuth := a.auth.(authenticator.TokenAuthenticator)
		usernamePasswordAuthenticator, isPwdAuth := a.auth.(authenticator.UsernamePasswordAuthenticator)

		// check if running on no-op authenticator
		if !isTokenAuth && !isPwdAuth {
			next.ServeHTTP(w, r)
			return
		}

		if authorizationHeader == "" {
			// Password authentication needs authorization header
			// Token authentication can be skipped if the request is for the login endpoint
			if !isTokenAuth || r.RequestURI != "/login" {
				slog.Error("Authorization header not found")
				http.Error(w, "Authorization header not found", http.StatusUnauthorized)
				return
			}

			// redirect to oauth login
			state, err := utils.RandomString(16)
			if err != nil {
				writeAuthorizationHeaderError(w, err)
				return
			}
			nonce, err := utils.RandomString(16)
			if err != nil {
				writeAuthorizationHeaderError(w, err)
				return
			}
			setCallbackCookie(w, r, "state", state)
			setCallbackCookie(w, r, "nonce", nonce)

			http.Redirect(w, r, tokenAuthentication.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
			return
		}

		// handle token authentication
		if isTokenAuth {
			token, err := getBearerToken(authorizationHeader)
			if err != nil {
				writeAuthorizationHeaderError(w, err)
				return
			}

			if err := tokenAuthentication.AuthenticateToken(token); err != nil {
				writeAuthorizationHeaderError(w, err)
				return
			}
		} else if isPwdAuth {
			username, password, err := getBasicAuthCredentials(authorizationHeader)
			if err != nil {
				writeAuthorizationHeaderError(w, err)
				return
			}

			err, _ = usernamePasswordAuthenticator.Authenticate(username, password)
			if err != nil {
				writeAuthorizationHeaderError(w, err)
				return
			}
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

func (a *Authentication) CallbackAction(w http.ResponseWriter, r *http.Request) {
	tokenAuthentication, ok := a.auth.(interface{}).(authenticator.TokenAuthenticator)
	if !ok {
		slog.Error("Callback not supported")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	state, err := r.Cookie("state")
	if err != nil {
		slog.Error("Error getting state cookie:", "error", err)
		http.Error(w, "state not found", http.StatusUnauthorized)
		return
	}
	if r.URL.Query().Get("state") != state.Value {
		slog.Error("State did not match")
		http.Error(w, "state did not match", http.StatusUnauthorized)
		return
	}

	code := r.URL.Query().Get("code")
	token, err := tokenAuthentication.Callback(state.Value, code)
	if err != nil {
		slog.Error("Error getting token:", "error", err)
		http.Error(w, "Authentication callback failed", http.StatusUnauthorized)
		return
	}
	header := w.Header()
	header.Set("Authorization", "Bearer "+token)
	header.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"token": token,
	}); err != nil {
		slog.Error("Error encoding JSON:", "error", err)
	}
}

func setCallbackCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   int(time.Hour.Seconds()),
		Secure:   r.TLS != nil,
		HttpOnly: true,
	}
	http.SetCookie(w, c)
}

// getBasicAuthCredentials extracts username and password from a basic auth header
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Authorization
// Authorization: Basic base64("username:password")
// returns a slice with two elements: username and password or an error if the header is invalid
func getBasicAuthCredentials(authHeader string) (string, string, error) {
	const prefix = "Basic "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", "", fmt.Errorf("invalid authorization header")
	}
	decoded, err := base64.StdEncoding.DecodeString(authHeader[len(prefix):])
	if err != nil {
		return "", "", err
	}
	credentials := strings.SplitN(string(decoded), ":", 2)
	if len(credentials) != 2 {
		return "", "", fmt.Errorf("invalid authorization header")
	}
	return credentials[0], credentials[1], nil
}

// getBearerToken extracts a bearer token from an auth header
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Authorization
// Authorization: Bearer token
// returns the token or an error if the header is invalid
func getBearerToken(authHeader string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", fmt.Errorf("invalid authorization header")
	}
	return authHeader[len(prefix):], nil
}
