package authenticator

import (
	"OpenSPMRegistry/config"
	"context"
	"log/slog"
)

// Authenticator is an interface for authenticating users
// based on their username and password
type Authenticator interface {
	// Authenticate authenticates a user based on their username and password
	// returns an error if the authentication fails
	Authenticate(username string, password string) error
}

type NoOpAuthenticator struct{}

func (a *NoOpAuthenticator) Authenticate(username string, password string) error {
	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Debug("Authentication disabled")
	}
	return nil
}

func CreateAuthenticator(config config.ServerConfig) Authenticator {
	if !config.Auth.Enabled {
		return &NoOpAuthenticator{}
	}

	switch config.Auth.Type {
	case "oidc":
		return NewOIDCAuthenticator(context.Background(), config)
	default:
		return &NoOpAuthenticator{}
	}
}
