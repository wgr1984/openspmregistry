package utils

import (
	"OpenSPMRegistry/config"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

func CopyStruct[T any](src *T) T {
	temp := *src
	temp2 := temp
	return temp2
}

func StripExtension(s string, ext string) string {
	if strings.HasSuffix(s, ext) {
		return strings.TrimSuffix(s, ext)
	} else {
		return s
	}
}

func RandomString(i int) (string, error) {
	if i < 0 {
		return "", fmt.Errorf("invalid length: %d", i)
	}
	b := make([]byte, i)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// BaseUrl returns the base URL for the server
// based on the configuration
// e.g. https://hostname:port
func BaseUrl(config config.ServerConfig) string {
	return fmt.Sprintf("https://%s:%d", config.Hostname, config.Port)
}

func WriteAuthorizationHeaderError(w http.ResponseWriter, err error) {
	slog.Error("Error parsing authorization header:", "error", err)
	http.Error(w, fmt.Sprintf("Authentication failed: %s", err), http.StatusUnauthorized)
}
