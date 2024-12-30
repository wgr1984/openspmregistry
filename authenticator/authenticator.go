package authenticator

import (
	"OpenSPMRegistry/config"
	"context"
	"golang.org/x/oauth2"
	"log/slog"
)

// Authenticator is an interface for authenticating users
// based on their username and password
type Authenticator interface {
	// Authenticate authenticates a user based on their username and password
	// returns an error if the authentication fails
	Authenticate(username string, password string) error
}

type TokenAuthenticator interface {
	// AuthenticateToken authenticates a user based on their token
	// returns an error if the authentication fails
	AuthenticateToken(token string) error

	// AuthCodeURL returns the URL for the OAuth2 authorization endpoint
	// based on the provided state and nonce
	AuthCodeURL(state string, nonce oauth2.AuthCodeOption) string

	// Callback returns the token based on the provided state and code
	// returns an error if the token cannot be retrieved
	Callback(state string, code string) (string, error)
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
	case "basic":
		return NewBasicAuthenticator(config.Auth.Users)
	default:
		return &NoOpAuthenticator{}
	}
}
