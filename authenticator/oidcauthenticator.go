package authenticator

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/utils"
	"context"
	"errors"
	"fmt"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"log/slog"
	"net/http"
	"strings"
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
	grantType string
}

// NewOIDCAuthenticator creates a new OIDC authenticator
// based on the provided configuration
func NewOIDCAuthenticator(ctx context.Context, config config.ServerConfig) *OidcAuthenticatorImpl {
	provider, err := oidc.NewProvider(ctx, config.Auth.Issuer)
	if err != nil {
		slog.Error("Failed to create OIDC provider", "err", err)
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

	return &OidcAuthenticatorImpl{
		ctx:       ctx,
		config:    oauthConfig,
		grantType: config.Auth.GrantType,
		verifier:  verifier,
		provider:  provider,
	}
}

func (a *OidcAuthenticatorImpl) Authenticate(w http.ResponseWriter, r *http.Request) (error, string) {
	authorizationHeader := r.Header.Get("Authorization")

	if authorizationHeader == "" {
		return errors.New("authorization header not found"), ""
	}

	token, err := getBearerToken(authorizationHeader)
	if err != nil {
		return err, ""
	}

	_, err = a.verifier.Verify(a.ctx, token)
	if err == nil {
		if slog.Default().Enabled(nil, slog.LevelDebug) {
			slog.Debug("Token still valid")
		}
		return nil, token
	}
	return err, ""
}

func (a *OidcAuthenticatorImpl) Login(_ http.ResponseWriter, _ *http.Request) {}

func (a *OidcAuthenticatorImpl) CheckAuthHeaderPresent(w http.ResponseWriter, r *http.Request) bool {
	// check if the request already has an authentication header
	// if it does, write the token to the response
	authenticationHeader := r.Header.Get("Authorization")
	if authenticationHeader != "" {
		writeTokenOutput(w, authenticationHeader)
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
