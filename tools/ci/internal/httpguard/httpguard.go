// Package httpguard holds the outbound HTTP checks shared by every client which sends a credential.
package httpguard

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
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

// TruncateUTF8 bounds data to at most limit bytes and trims back past an incomplete trailing
// UTF-8 sequence, which keeps an excerpt embedded in an error message valid UTF-8.
func TruncateUTF8(data []byte, limit int) []byte {
	if len(data) <= limit {
		return data
	}
	data = data[:limit]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return data
}
