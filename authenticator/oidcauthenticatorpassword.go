package authenticator

import (
	"OpenSPMRegistry/config"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const csrfTokenValue = "csrf-token"

// OidcAuthenticatorPassword is an authenticator that uses OpenID Connect with password grant
type OidcAuthenticatorPassword interface {
	OidcAuthenticator
}

type OidcAuthenticatorPasswordImpl struct {
	*OidcAuthenticatorImpl
	sharedEncryptionKey []byte
	signingKey          ed25519.PublicKey
	privateKey          ed25519.PrivateKey
}

// NewOIDCAuthenticatorPassword OidcAuthenticatorPassword creates a new OIDC authenticator
// based on the provided configuration
func NewOIDCAuthenticatorPassword(ctx context.Context, config config.ServerConfig) *OidcAuthenticatorPasswordImpl {
	signingKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)

	return &OidcAuthenticatorPasswordImpl{
		OidcAuthenticatorImpl: NewOIDCAuthenticator(ctx, config),
		sharedEncryptionKey:   make([]byte, 16),
		signingKey:            signingKey,
		privateKey:            privateKey,
	}
}

func (a *OidcAuthenticatorPasswordImpl) Callback(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "callback not supported", http.StatusUnauthorized)
}

func (a *OidcAuthenticatorPasswordImpl) Authenticate(w http.ResponseWriter, r *http.Request) (string, error) {
	username, password, ok := r.BasicAuth()
	// check if this is a basic auth request
	if !ok {
		// if not, try to authenticate using the oidc authenticator
		token, err := a.OidcAuthenticatorImpl.Authenticate(w, r)
		if err != nil {
			return "", err
		}
		return token, nil
	}

	// check x-csrf-token header
	csrfToken := r.Header.Get("x-csrf-token")
	if csrfToken == "" {
		return "", errors.New("missing CSRF token")
	}

	// decrypt and verify CSRF token
	err := a.verifyToken(csrfToken, csrfTokenValue)
	if err != nil {
		return "", err
	}

	idToken, err := a.requestToken(username, password)
	if err != nil {
		return "", err
	}

	writeTokenOutput(w, idToken, a.template)

	return idToken, nil
}

func (a *OidcAuthenticatorPasswordImpl) Login(w http.ResponseWriter, r *http.Request) {
	if a.CheckAuthHeaderPresent(w, r) {
		return
	}

	files, err := template.New("login.gohtml").ParseFiles("static/login.gohtml")
	if err != nil {
		http.Error(w, "Error parsing template", http.StatusInternalServerError)
		return
	}

	csrfToken, err := a.encryptToken(csrfTokenValue)
	if err != nil {
		http.Error(w, "Error encrypting token", http.StatusInternalServerError)
		return
	}

	err = files.Execute(w, struct {
		Title     string
		CsrfToken string
	}{
		Title:     "Login to OpenSPMRegistry",
		CsrfToken: csrfToken,
	})
	if err != nil {
		http.Error(w, "Error executing template", http.StatusInternalServerError)
		return
	}
}

func (a *OidcAuthenticatorPasswordImpl) requestToken(username string, password string) (string, error) {
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

// encryptToken encrypts the provided value into a JWT
// the JWT is encrypted using the shared encryption key
// the JWT has the subject "oidc login nonce"
// the JWT is set to expire in 5 min
// returns the encrypted JWT
func (a *OidcAuthenticatorPasswordImpl) encryptToken(value string) (string, error) {
	enc, err := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       a.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))
	if err != nil {
		fmt.Printf("making encrypter: %s\n", err)
		return "", err
	}

	cl := jwt.Claims{
		Subject: "oidc login nonce",
		Issuer:  "OpenSPMRegistry",
		Expiry:  jwt.NewNumericDate(time.Now().Add(time.Minute * 5)),
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		value,
	}
	raw, err := jwt.Encrypted(enc).Claims(cl).Claims(privateClaim).Serialize()
	if err != nil {
		fmt.Printf("encrypting and signing JWT: %s\n", err)
		return "", err
	}
	return raw, nil
}

// verifyToken decrypts and verifies the provided token
// the token is decrypted using the shared encryption key
// the token is verified to have the subject "oidc login nonce"
// the token is verified to not be expired
// the token is verified to have the provided value
// returns an error if the token is invalid
func (a *OidcAuthenticatorPasswordImpl) verifyToken(token string, value string) error {
	tok, err := jwt.ParseEncrypted(token,
		[]jose.KeyAlgorithm{jose.DIRECT},
		[]jose.ContentEncryption{jose.A128GCM},
	)
	if err != nil {
		slog.Error("parsing JWT: %s", "err", err)
		return err
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		"",
	}

	out := jwt.Claims{}
	if err := tok.Claims(a.sharedEncryptionKey, &out, &privateClaim); err != nil {
		slog.Error("verifying JWT: %s", "err", err)
		return err
	}

	if out.Subject != "oidc login nonce" {
		return errors.New("invalid subject")
	}

	if time.Now().After(out.Expiry.Time()) {
		return errors.New("token expired")
	}

	if privateClaim.Value == "" {
		return errors.New("missing value")
	}

	if privateClaim.Value != value {
		return errors.New("invalid value")
	}

	return nil
}
