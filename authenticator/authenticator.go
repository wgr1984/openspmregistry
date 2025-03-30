package authenticator

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/controller"
	"context"
	"net/http"
)

type Authenticator interface {
	// Authenticate authenticates a user based on their username and password
	// returns the token and an error if the authentication fails
	Authenticate(w http.ResponseWriter, r *http.Request) (string, error)
}

// writeTokenOutput writes the token to the response
// to be used by the client to authenticate via --token flag
func writeTokenOutput(w http.ResponseWriter, token string, templateParser controller.TemplateParser) {
	if templateParser == nil {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(token))
		return
	}
	files, err := templateParser.ParseFiles("static/token.gohtml")
	if err != nil {
		http.Error(w, "Error parsing template", http.StatusInternalServerError)
		return
	}
	err = files.Execute(w, struct {
		Token string
	}{Token: token})
	if err != nil {
		http.Error(w, "Error executing template", http.StatusInternalServerError)
		return
	}
}

// CreateAuthenticator creates an authenticator based on the provided configuration
// if authentication is disabled, it returns a NoOpAuthenticator
// if the authentication type is oidc, it returns an OIDCAuthenticator (code or password)
// if the authentication type is basic, it returns a BasicAuthenticator
// else it returns a NoOpAuthenticator
func CreateAuthenticator(config config.ServerConfig) Authenticator {
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
