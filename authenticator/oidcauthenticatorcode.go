package authenticator

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/controller"
	"OpenSPMRegistry/utils"
	"context"
	"log/slog"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OidcAuthenticatorCode is an authenticator that uses OpenID Connect with code grant
type OidcAuthenticatorCode interface {
	OidcAuthenticator
	AuthCodeURL(state string, nonce oauth2.AuthCodeOption) string
	Callback(w http.ResponseWriter, r *http.Request)
}

type randomStringGenerator interface {
	RandomString(length int) (string, error)
}

type defaultRandomStringGenerator struct{}

func (d *defaultRandomStringGenerator) RandomString(length int) (string, error) {
	return utils.RandomString(length)
}

type OidcAuthenticatorCodeImpl struct {
	*OidcAuthenticatorImpl
	randomStringGenerator randomStringGenerator
}

// NewOIDCAuthenticatorCodeWithConfig creates a new OIDC authenticator with code grant
// based on the provided configuration
func NewOIDCAuthenticatorCodeWithConfig(
	ctx context.Context,
	config config.ServerConfig,
	oidcConfig *oidc.Config,
	template controller.TemplateParser,
) *OidcAuthenticatorCodeImpl {
	base := NewOIDCAuthenticatorWithConfig(ctx, config, oidcConfig, template)
	if base == nil {
		return nil
	}
	return &OidcAuthenticatorCodeImpl{
		OidcAuthenticatorImpl: base,
		randomStringGenerator: &defaultRandomStringGenerator{},
	}
}

// NewOIDCAuthenticatorCode creates a new OIDC authenticator with code grant
// based on the provided configuration
func NewOIDCAuthenticatorCode(ctx context.Context, config config.ServerConfig) *OidcAuthenticatorCodeImpl {
	return &OidcAuthenticatorCodeImpl{
		OidcAuthenticatorImpl: NewOIDCAuthenticator(ctx, config),
		randomStringGenerator: &defaultRandomStringGenerator{},
	}
}

func (a *OidcAuthenticatorCodeImpl) AuthCodeURL(state string, nonce oauth2.AuthCodeOption) string {
	return a.config.AuthCodeURL(state, nonce)
}

func (a *OidcAuthenticatorCodeImpl) Callback(w http.ResponseWriter, r *http.Request) {
	stateParam := r.URL.Query().Get("state")
	if stateParam == "" {
		slog.Error("State parameter not found in URL")
		http.Error(w, "state not found", http.StatusUnauthorized)
		return
	}

	state, err := r.Cookie("state")
	if err != nil {
		slog.Error("Error getting state cookie:", "error", err)
		http.Error(w, "state not found", http.StatusUnauthorized)
		return
	}

	if stateParam != state.Value {
		slog.Error("State did not match")
		http.Error(w, "state did not match", http.StatusUnauthorized)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		slog.Error("Code parameter not found in URL")
		http.Error(w, "code not found", http.StatusUnauthorized)
		return
	}

	token, err := a.config.Exchange(a.ctx, code)
	if err != nil {
		slog.Error("Failed to exchange code for token", "err", err)
		http.Error(w, "Failed to exchange code for token", http.StatusUnauthorized)
		return
	}
	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		slog.Error("Failed to get id token")
		http.Error(w, "Failed to get id token", http.StatusUnauthorized)
		return
	}

	writeTokenOutput(w, idToken, a.template)
}

func (a *OidcAuthenticatorCodeImpl) Login(w http.ResponseWriter, r *http.Request) {
	if a.CheckAuthHeaderPresent(w, r) {
		return
	}

	// Otherwise redirect to oauth login
	state, err := a.randomStringGenerator.RandomString(16)
	if err != nil {
		utils.WriteAuthorizationHeaderError(w, err)
		return
	}
	nonce, err := a.randomStringGenerator.RandomString(16)
	if err != nil {
		utils.WriteAuthorizationHeaderError(w, err)
		return
	}
	setCallbackCookie(w, r, "state", state)
	setCallbackCookie(w, r, "nonce", nonce)

	http.Redirect(w, r, a.config.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
	return
}

// setCallbackCookie sets a cookie with the provided name and value
// the cookie is set to expire in 1 hour
// the cookie is secure if the request is over TLS
// the cookie is http only
// the cookie is set to SameSiteStrictMode
// the cookie is set on the response writer
func setCallbackCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   3600, // 1 hour in seconds
		Secure:   r.TLS != nil,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, c)
}
