package authenticator

import (
	"OpenSPMRegistry/config"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
)

// OidcAuthenticatorPassword is an authenticator that uses OpenID Connect with password grant
type OidcAuthenticatorPassword struct {
	*OidcAuthenticator
}

// NewOIDCAuthenticatorPassword OidcAuthenticatorPassword creates a new OIDC authenticator
// based on the provided configuration
func NewOIDCAuthenticatorPassword(ctx context.Context, config config.ServerConfig) *OidcAuthenticatorPassword {
	return &OidcAuthenticatorPassword{
		NewOIDCAuthenticator(ctx, config),
	}
}

func (a *OidcAuthenticatorPassword) Authenticate(username string, password string) (error, string) {
	if a.grantType != "password" {
		return errors.New("invalid grant type"), ""
	}

	// generate cache key
	hashedPassword := sha256.Sum256([]byte(password))
	key := username + ":" + base64.StdEncoding.EncodeToString(hashedPassword[:])

	// check cache
	if token, ok := a.cache.Get(key); ok {
		// check token
		if _, err := a.verifier.Verify(a.ctx, token); err != nil {
			return err, ""
		} else {
			if slog.Default().Enabled(nil, slog.LevelDebug) {
				slog.Debug("Token still valid")
			}
			return nil, token
		}
	}

	// request token from auth provider
	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Debug("Requesting token from auth provider")
	}

	idToken, err := a.requestToken(username, password)
	if err != nil {
		return err, ""
	}

	// store token in cache
	a.cache.Add(key, idToken)

	return nil, idToken
}

func (a *OidcAuthenticatorPassword) requestToken(username string, password string) (string, error) {
	// request token from auth provider
	token, err := a.config.PasswordCredentialsToken(a.ctx, username, password)
	if err != nil {
		return "", err
	}
	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", errors.New("missing id token")
	}
	return idToken, nil
}
