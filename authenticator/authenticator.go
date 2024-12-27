package authenticator

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/utils"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const CacheSize = 100
const CacheTtl = 24 * time.Hour // 1 hour

type Authenticator struct {
	config config.ServerConfig
	cache  *utils.LRUCache[string]
}

func NewAuthenticator(config config.ServerConfig) *Authenticator {
	return &Authenticator{
		config: config,
		cache:  utils.NewLRUCache[string](CacheSize, CacheTtl),
	}
}

func (a *Authenticator) Authenticate(username string, password string) error {

	// check cache
	if token, ok := a.cache.Get(username); ok {
		// check token
		if valid, err := a.checkJWT(token); err != nil {
			return err
		} else if valid {
			return nil
		}
	}

	// request token from auth provider
	provider := a.config.Auth
	resp, err := requestToken(&provider, username, password)
	if err != nil {
		return err
	}
	// get user info
	if _, ok := resp["access_token"]; !ok {
		return errors.New("missing access token")
	}
	idToken, ok := resp["id_token"]
	idTokenJWT := ""
	if !ok {
		// request user info
		_, err := requestUserInfo(&provider, resp["access_token"].(string))
		if err != nil {
			return err
		}
		idTokenJWT = "TODO"
		// TOD0: create jwt from user info and exp response
	} else {
		idTokenJWT = idToken.(string)
	}

	// store token in cache
	a.cache.Add(username, idTokenJWT)

	return nil
}

func (a *Authenticator) checkJWT(token string) (bool, error) {
	// check JWT still valid
	// extract claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false, errors.New("invalid JWT token format")
	}

	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false, errors.New("failed to decode JWT header")
	}

	var headerMap map[string]interface{}
	if err := json.Unmarshal(header, &headerMap); err != nil {
		return false, errors.New("failed to unmarshal JWT header")
	}

	if headerMap["alg"] != "RS256" {
		return false, errors.New("unexpected JWT algorithm")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false, errors.New("failed to decode JWT payload")
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false, errors.New("failed to unmarshal JWT payload")
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false, errors.New("failed to decode JWT signature")
	}

	parsedKey, err := parseRSAPublicKeyFromPEM(a.config.Auth.PubKey)
	if err != nil {
		return false, errors.New("failed to parse RSA public key")
	}

	h := sha256.New()
	h.Write([]byte(parts[0] + "." + parts[1]))
	hashed := h.Sum(nil)

	err = rsa.VerifyPKCS1v15(parsedKey, crypto.SHA256, hashed, signature)
	if err != nil {
		return false, errors.New("invalid JWT signature")
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Unix(int64(exp), 0).Before(time.Now()) {
			return false, errors.New("JWT token expired")
		}
	}

	return true, nil
}

func parseRSAPublicKeyFromPEM(pubKey string) (*rsa.PublicKey, error) {

	block, _ := pem.Decode([]byte(pubKey))
	if block == nil {
		return nil, errors.New("failed to parse public key block")
	}

	parsedKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := parsedKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("failed to parse RSA public key")
	}
	return rsaKey, nil
}

func requestToken(provider *config.AuthConfig, username string, password string) (map[string]interface{}, error) {
	data := url.Values{}
	// TODO: add support for other grant types
	data.Set("grant_type", "password")
	data.Set("scope", "openid email")
	data.Set("username", username)
	data.Set("password", password)
	data.Set("client_id", provider.ClientId)
	data.Set("client_secret", provider.ClientSecret)

	resp, err := http.PostForm(provider.TokenEndpoint, data)

	return readResponse(err, resp)
}

func requestUserInfo(a *config.AuthConfig, s string) (map[string]interface{}, error) {
	// request user info from auth provider
	resp, err := http.Get(a.UserIdEndpoint)
	return readResponse(err, resp)
}

func readResponse(err error, resp *http.Response) (map[string]interface{}, error) {
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("Failed to close response body", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error token endpoint auth provider: %s", resp.Status)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
