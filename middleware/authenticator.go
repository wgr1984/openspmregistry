package middleware

import (
	"OpenSPMRegistry/authenticator"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

type Authentication struct {
	auth *authenticator.Authenticator
}

func NewAuthentication(auth *authenticator.Authenticator) *Authentication {
	return &Authentication{auth: auth}
}

func (a *Authentication) Authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if the request is authorized
		authorizationHeader := r.Header.Get("Authorization")
		if authorizationHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		username, password, err := getBasicAuthCredentials(authorizationHeader)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := a.auth.Authenticate(username, password); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// If authorized, call the next handler
		next.ServeHTTP(w, r)
	}
}

// getBasicAuthCredentials extracts username and password from a basic auth header
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Authorization
// Authorization: Basic base64("username:password")
// returns a slice with two elements: username and password
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
