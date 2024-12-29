package authenticator

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/utils"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"log/slog"
	"time"
)

const CacheSize = 100
const CacheTtl = 24 * time.Hour // 1 hour

// OidcAuthenticator is an authenticator that uses OpenID Connect
type OidcAuthenticator struct {
	cache    *utils.LRUCache[string]
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
	ctx      context.Context
	config   oauth2.Config
}

// NewOIDCAuthenticator creates a new OIDC authenticator
// based on the provided configuration
func NewOIDCAuthenticator(ctx context.Context, config config.ServerConfig) *OidcAuthenticator {
	provider, err := oidc.NewProvider(ctx, config.Auth.Issuer)
	if err != nil {
		slog.Error("Failed to create OIDC provider", err)
		return nil
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: config.Auth.ClientId})

	oauthConfig := oauth2.Config{
		ClientID:     config.Auth.ClientId,
		ClientSecret: config.Auth.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  utils.BaseUrl(config) + "/callback",
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	return &OidcAuthenticator{
		ctx:      ctx,
		config:   oauthConfig,
		verifier: verifier,
		provider: provider,
		cache:    utils.NewLRUCache[string](CacheSize, CacheTtl),
	}
}

func (a *OidcAuthenticator) Authenticate(username string, password string) error {
	// generate cache key
	hashedPassword := sha256.Sum256([]byte(password))
	key := username + ":" + base64.StdEncoding.EncodeToString(hashedPassword[:])

	// check cache
	if token, ok := a.cache.Get(key); ok {
		// check token
		if _, err := a.verifier.Verify(a.ctx, token); err != nil {
			return err
		} else {
			if slog.Default().Enabled(nil, slog.LevelDebug) {
				slog.Debug("JWT token still valid")
			}
			return nil
		}
	}

	// request token from auth provider
	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Debug("Requesting token from auth provider")
	}

	idToken, err := a.requestToken(username, password)
	if err != nil {
		return err
	}

	// store token in cache
	a.cache.Add(key, idToken)

	return nil
}

func (a *OidcAuthenticator) requestToken(username string, password string) (string, error) {
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
