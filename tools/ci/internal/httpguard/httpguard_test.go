package httpguard

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func mustRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}
	return &http.Request{URL: parsed}
}

func TestRefuseCrossHostRedirect_HostBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		target  string
		wantErr bool
	}{
		{name: "same host and port", origin: "https://gitlab.com/a", target: "https://gitlab.com/b"},
		{name: "same host other port", origin: "http://127.0.0.1:8080/a", target: "http://127.0.0.1:9090/b"},
		{name: "host case differs", origin: "https://GitLab.com/a", target: "https://gitlab.com/b"},
		{name: "different host", origin: "https://gitlab.com/a", target: "https://evil.example.com/b", wantErr: true},
		{name: "loopback name versus address", origin: "http://127.0.0.1:8080/a", target: "http://localhost:8080/b", wantErr: true},
		{name: "subdomain", origin: "https://gitlab.com/a", target: "https://api.gitlab.com/b", wantErr: true},
		{name: "suffix lookalike", origin: "https://gitlab.com/a", target: "https://gitlab.com.evil.example/b", wantErr: true},
		{name: "userinfo confusion", origin: "https://gitlab.com/a", target: "https://gitlab.com@evil.example/b", wantErr: true},
		{name: "scheme downgrade on same host", origin: "https://gitlab.com/a", target: "http://gitlab.com/b", wantErr: true},
		{name: "scheme upgrade on same host", origin: "http://gitlab.com/a", target: "https://gitlab.com/b"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := RefuseCrossHostRedirect(mustRequest(t, tc.target), []*http.Request{mustRequest(t, tc.origin)})
			if tc.wantErr && err == nil {
				t.Errorf("redirect from %q to %q was allowed", tc.origin, tc.target)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("redirect from %q to %q returned an unexpected error: %v", tc.origin, tc.target, err)
			}
		})
	}
}

func TestRefuseCrossHostRedirect_ChainAnchorsOnTheFirstRequest(t *testing.T) {
	via := []*http.Request{
		mustRequest(t, "https://gitlab.com/a"),
		mustRequest(t, "https://gitlab.com/b"),
	}
	err := RefuseCrossHostRedirect(mustRequest(t, "https://evil.example.com/c"), via)
	if err == nil {
		t.Error("a redirect away from the original host was allowed on the third hop")
	}
}

func TestRefuseCrossHostRedirect_HopLimit(t *testing.T) {
	origin := mustRequest(t, "https://gitlab.com/a")
	for hops := 1; hops <= 12; hops++ {
		via := make([]*http.Request, hops)
		for i := range via {
			via[i] = origin
		}
		err := RefuseCrossHostRedirect(mustRequest(t, "https://gitlab.com/next"), via)
		if wantErr := hops >= 10; (err != nil) != wantErr {
			t.Errorf("after %d prior requests: error = %v, want error %t", hops, err, wantErr)
		}
	}
}

// A chain which leaves the original host is refused at the first hop.
// The foreign host never receives the request.
func TestRefuseCrossHostRedirect_BounceViaForeignHostNeverReachesIt(t *testing.T) {
	var hitsOrigin, hitsForeign atomic.Int64
	var originURL, foreignURL string

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hitsOrigin.Add(1) == 1 {
			http.Redirect(w, r, foreignURL, http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(origin.Close)
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsForeign.Add(1)
		http.Redirect(w, r, originURL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(foreign.Close)
	originURL = origin.URL
	foreignURL = strings.Replace(foreign.URL, "127.0.0.1", "localhost", 1)

	client := &http.Client{CheckRedirect: RefuseCrossHostRedirect}
	req, err := http.NewRequest(http.MethodGet, origin.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("PRIVATE-TOKEN", "secret")

	if resp, err := client.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Fatal("the redirect chain via a foreign host succeeded")
	}
	if got := hitsForeign.Load(); got != 0 {
		t.Errorf("foreign host received %d requests, want 0", got)
	}
}

// Ports of one host share a hostname. A chain across ports of that host stays allowed.
func TestRefuseCrossHostRedirect_BounceAcrossPortsOfOneHostIsAllowed(t *testing.T) {
	var hitsOrigin, hitsSibling atomic.Int64
	var originURL, siblingURL string

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hitsOrigin.Add(1) == 1 {
			http.Redirect(w, r, siblingURL, http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(origin.Close)
	sibling := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsSibling.Add(1)
		http.Redirect(w, r, originURL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(sibling.Close)
	originURL, siblingURL = origin.URL, sibling.URL

	client := &http.Client{CheckRedirect: RefuseCrossHostRedirect}
	req, err := http.NewRequest(http.MethodGet, origin.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("the chain across ports of one host failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || hitsSibling.Load() != 1 {
		t.Errorf("status = %d and sibling hits = %d, want 200 and 1", resp.StatusCode, hitsSibling.Load())
	}
}

func TestValidateCredential_Boundaries(t *testing.T) {
	tests := []struct {
		token   string
		wantErr bool
	}{
		{token: "", wantErr: true},
		{token: " ", wantErr: true},
		{token: "\t\n\r", wantErr: true},
		{token: " ", wantErr: true},
		{token: "k"},
		{token: " padded "},
		{token: "sk-ant-oat01-abc"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.token), func(t *testing.T) {
			err := ValidateCredential(tc.token)
			if tc.wantErr && !errors.Is(err, ErrEmptyCredential) {
				t.Errorf("ValidateCredential(%q) = %v, want ErrEmptyCredential", tc.token, err)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateCredential(%q) returned an unexpected error: %v", tc.token, err)
			}
		})
	}
}
