package authenticator

import (
	"OpenSPMRegistry/config"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
)

type BasicAuthenticator struct {
	users []config.User
}

func NewBasicAuthenticator(users []config.User) *BasicAuthenticator {
	return &BasicAuthenticator{users: users}
}

func (a *BasicAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (error, string) {
	authorizationHeader := r.Header.Get("Authorization")
	if authorizationHeader == "" {
		return errors.New("authorization header not found"), ""
	}

	username, password, ok := r.BasicAuth()
	if !ok {
		return errors.New("missing credentials"), ""
	}

	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Debug("Basic authentication")
	}
	for _, user := range a.users {
		if hashedPwd := hashPassword(password); user.Password == hashedPwd && user.Username == username {
			return nil, hashedPwd
		}
	}
	return errors.New("invalid username or password"), ""
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}
