// Package httpguard holds the outbound HTTP checks shared by every client which sends a credential.
package httpguard

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// maxRedirects matches the net/http default hop limit.
const maxRedirects = 10

// ErrEmptyCredential reports a credential which is empty or holds only whitespace.
var ErrEmptyCredential = errors.New("credential is empty")

// RefuseCrossHostRedirect is an http.Client CheckRedirect function. net/http forwards custom
// credential headers to any redirect target, which leaves the host check to the caller.
func RefuseCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	origin := via[0].URL
	if !strings.EqualFold(req.URL.Hostname(), origin.Hostname()) {
		return fmt.Errorf("refusing redirect from host %q to host %q", origin.Hostname(), req.URL.Hostname())
	}
	if origin.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect from https to %s on host %q", req.URL.Scheme, req.URL.Hostname())
	}
	return nil
}

// ValidateCredential rejects a credential which cannot authenticate any request.
func ValidateCredential(token string) error {
	if strings.TrimSpace(token) == "" {
		return ErrEmptyCredential
	}
	return nil
}
