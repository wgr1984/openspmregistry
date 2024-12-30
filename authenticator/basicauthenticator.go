package authenticator

import (
	"OpenSPMRegistry/config"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
)

type BasicAuthenticator struct {
	users []config.User
}

func NewBasicAuthenticator(users []config.User) *BasicAuthenticator {
	return &BasicAuthenticator{users: users}
}

func (a *BasicAuthenticator) Authenticate(username string, password string) error {
	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Debug("Basic authentication")
	}
	for _, user := range a.users {
		if user.Username == username && user.Password == hashPassword(password) {
			return nil
		}
	}
	return errors.New("invalid username or password")
}

func (a *BasicAuthenticator) SkipAuth() bool {
	return false
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}
