package controller

import (
	"net/http"
)

// LoginAction is the handler for the login route
// login will be handled by the authenticator
func (c *Controller) LoginAction(w http.ResponseWriter, r *http.Request) {
	printCallInfo("LoginAction", r)
	w.WriteHeader(http.StatusOK)
}
