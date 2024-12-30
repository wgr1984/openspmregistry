package authenticator

import "log/slog"

type NoOpAuthenticator struct{}

func (a *NoOpAuthenticator) Authenticate(username string, password string) error {
	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Debug("Authentication disabled")
	}
	return nil
}

// SkipAuth returns true for the NoOpAuthenticator
func (a *NoOpAuthenticator) SkipAuth() bool {
	return true
}
