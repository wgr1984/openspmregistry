package authenticator

import (
	"log/slog"
	"net/http"
)

// NoOpAuthenticator is an authenticator that does nothing
type NoOpAuthenticator struct{}

func (n *NoOpAuthenticator) Authenticate(_ http.ResponseWriter, _ *http.Request) (error, string) {
	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Debug("NoOp authentication")
	}
	return nil, ""
}

func (n *NoOpAuthenticator) Callback(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "callback not supported", http.StatusUnauthorized)
}
