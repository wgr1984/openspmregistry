package authenticator

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/utils"
	"context"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"log/slog"
	"time"
)

// OidcAuthenticator is an authenticator that uses OpenID Connect
type OidcAuthenticator struct {
	cache     *utils.LRUCache[string]
	verifier  *oidc.IDTokenVerifier
	provider  *oidc.Provider
	ctx       context.Context
	config    oauth2.Config
	grantType string
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
		ctx:       ctx,
		config:    oauthConfig,
		grantType: config.Auth.GrantType,
		verifier:  verifier,
		provider:  provider,
		cache:     utils.NewLRUCache[string](config.Auth.JWTCacheSize, time.Duration(config.Auth.JWTCacheTTLHours)*time.Hour),
	}
}
