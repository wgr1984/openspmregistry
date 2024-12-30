package authenticator

import (
	"OpenSPMRegistry/config"
	"context"
	"errors"
	"golang.org/x/oauth2"
	"log/slog"
)

// OidcAuthenticatorCode is an authenticator that uses OpenID Connect with code grant
type OidcAuthenticatorCode struct {
	*OidcAuthenticator
}

// NewOIDCAuthenticatorCode  creates a new OIDC authenticator with code grant
// based on the provided configuration
func NewOIDCAuthenticatorCode(ctx context.Context, config config.ServerConfig) *OidcAuthenticatorCode {
	return &OidcAuthenticatorCode{
		NewOIDCAuthenticator(ctx, config),
	}
}

func (a *OidcAuthenticator) AuthenticateToken(token string) error {
	if a.grantType != "code" {
		return errors.New("invalid grant type")
	}

	_, err := a.verifier.Verify(a.ctx, token)
	if err == nil {
		if slog.Default().Enabled(nil, slog.LevelDebug) {
			slog.Debug("Token still valid")
		}
		return nil
	}
	return err
}

func (a *OidcAuthenticator) AuthCodeURL(state string, nonce oauth2.AuthCodeOption) string {
	return a.config.AuthCodeURL(state, nonce)
}

func (a *OidcAuthenticator) Callback(state string, code string) (string, error) {
	token, err := a.config.Exchange(a.ctx, code)
	if err != nil {
		return "", err
	}
	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", errors.New("missing id token")
	}
	return idToken, nil
}
