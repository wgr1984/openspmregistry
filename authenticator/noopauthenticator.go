package authenticator

import (
	"context"
	"log/slog"
	"net/http"
)

// NoOpAuthenticator is an authenticator that does nothing
type NoOpAuthenticator struct{}

func (n *NoOpAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (string, error) {
	if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
		slog.Debug("NoOp authentication")
	}
	return "noop", nil
}

func (n *NoOpAuthenticator) Callback(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "callback not supported", http.StatusUnauthorized)
}
