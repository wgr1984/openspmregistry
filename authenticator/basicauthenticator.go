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

func (a *BasicAuthenticator) Authenticate(username string, password string) (error, string) {
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

func (a *BasicAuthenticator) SkipAuth() bool {
	return false
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}
