package authenticator

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/controller"
	"OpenSPMRegistry/utils"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OidcAuthenticator interface {
	Authenticator

	// Login logs the user in using the OIDC authenticator
	// this is used to redirect the user to the OIDC provider (code grant)
	// or to display a login form (password grant)
	Login(w http.ResponseWriter, r *http.Request)

	// CheckAuthHeaderPresent checks if the request has an authentication header
	// if it does, it writes the token to the response, to be used by the client
	CheckAuthHeaderPresent(w http.ResponseWriter, r *http.Request) bool
}

// OidcAuthenticatorImpl is an authenticator that uses OpenID Connect
type OidcAuthenticatorImpl struct {
	verifier  *oidc.IDTokenVerifier
	provider  *oidc.Provider
	ctx       context.Context
	config    oauth2.Config
	template  controller.TemplateParser
	grantType string
}

// NewOIDCAuthenticatorWithConfig creates a new OIDC authenticator
// based on the provided configuration
// ctx is the context to use for the OIDC provider
// config is the server configuration
// oidcConfig is the OIDC configuration can be nil in which case the default configuration is used
// if oidcConfig is provided, the client ID not taken from the server config
func NewOIDCAuthenticatorWithConfig(
	ctx context.Context,
	config config.ServerConfig,
	oidcConfig *oidc.Config,
	template controller.TemplateParser,
) *OidcAuthenticatorImpl {
	provider, err := oidc.NewProvider(ctx, config.Auth.Issuer)
	if err != nil {
		slog.Error("Failed to create OIDC provider", "err", err)
		return nil
	}

	var oidcConfigToUse *oidc.Config
	if oidcConfig != nil {
		oidcConfigToUse = oidcConfig
	} else {
		oidcConfigToUse = &oidc.Config{ClientID: config.Auth.ClientId}
	}
	verifier := provider.Verifier(oidcConfigToUse)

	oauthConfig := oauth2.Config{
		ClientID:     config.Auth.ClientId,
		ClientSecret: config.Auth.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  utils.BaseUrl(config) + "/callback",
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	return &OidcAuthenticatorImpl{
		ctx:       ctx,
		config:    oauthConfig,
		grantType: config.Auth.GrantType,
		verifier:  verifier,
		provider:  provider,
		template:  template,
	}
}

// NewOIDCAuthenticator creates a new OIDC authenticator
// based on the provided configuration
func NewOIDCAuthenticator(ctx context.Context, config config.ServerConfig) *OidcAuthenticatorImpl {
	t := template.New("token.gohtml")
	return NewOIDCAuthenticatorWithConfig(ctx, config, nil, t)
}

func (a *OidcAuthenticatorImpl) Authenticate(w http.ResponseWriter, r *http.Request) (string, error) {
	authorizationHeader := r.Header.Get("Authorization")

	if authorizationHeader == "" {
		return "", errors.New("authorization header not found")
	}

	token, err := getBearerToken(authorizationHeader)
	if err != nil {
		return "", err
	}

	_, err = a.verifier.Verify(a.ctx, token)
	if err == nil {
		if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
			slog.Debug("Token still valid")
		}
		return token, nil
	}
	return "", err
}

func (a *OidcAuthenticatorImpl) Login(_ http.ResponseWriter, _ *http.Request) {}

func (a *OidcAuthenticatorImpl) CheckAuthHeaderPresent(w http.ResponseWriter, r *http.Request) bool {
	// check if the request already has an authentication header
	// if it does, write the token to the response
	authenticationHeader := r.Header.Get("Authorization")
	if authenticationHeader != "" {
		if a.template == nil {
			// If no template is available, write token directly
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(authenticationHeader))
		} else {
			writeTokenOutput(w, authenticationHeader, a.template)
		}
		return true
	}
	return false
}

// getBearerToken extracts a bearer token from an auth header
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Authorization
// Authorization: Bearer token
// returns the token or an error if the header is invalid
func getBearerToken(authHeader string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", fmt.Errorf("invalid authorization header")
	}
	return authHeader[len(prefix):], nil
}
