package authenticator

import (
	"OpenSPMRegistry/config"
	"context"
	"golang.org/x/oauth2"
)

type UsernamePasswordAuthenticator interface {
	// Authenticate authenticates a user based on their username and password
	// returns an error if the authentication fails else returns the token
	Authenticate(username string, password string) (error, string)
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

func CreateAuthenticator(config config.ServerConfig) interface{} {
	if !config.Auth.Enabled {
		return &NoOpAuthenticator{}
	}

	switch config.Auth.Type {
	case "oidc":
		switch config.Auth.GrantType {
		case "code":
			return NewOIDCAuthenticatorCode(context.Background(), config)
		default:
			return NewOIDCAuthenticatorPassword(context.Background(), config)
		}
	case "basic":
		return NewBasicAuthenticator(config.Auth.Users)
	default:
		return &NoOpAuthenticator{}
	}
}
