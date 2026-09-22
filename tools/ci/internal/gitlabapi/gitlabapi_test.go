package gitlabapi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCreateTag_Success(t *testing.T) {
	var gotMethod, gotPath, gotToken string
	var gotQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	if err := CreateTag(server.URL, "123", "1.4.4", "abc123", "secret-token"); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("request method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotPath != "/projects/123/repository/tags" {
		t.Errorf("request path = %q, want %q", gotPath, "/projects/123/repository/tags")
	}
	if got := gotQuery.Get("tag_name"); got != "1.4.4" {
		t.Errorf("tag_name = %q, want %q", got, "1.4.4")
	}
	if got := gotQuery.Get("ref"); got != "abc123" {
		t.Errorf("ref = %q, want %q", got, "abc123")
	}
	if gotToken != "secret-token" {
		t.Errorf("PRIVATE-TOKEN header = %q, want %q", gotToken, "secret-token")
	}
}

func TestCreateTag_ProjectIDEscaped(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	if err := CreateTag(server.URL, "group/project", "1.4.4", "abc123", "unused"); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error: %v", err)
	}

	if gotPath != "/projects/group%2Fproject/repository/tags" {
		t.Errorf("request path = %q, want the project ID path-escaped", gotPath)
	}
}

func TestCreateTag_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"Tag already exists"}`)
	}))
	defer server.Close()

	err := CreateTag(server.URL, "123", "1.4.4", "abc123", "unused")
	if err == nil {
		t.Fatal("CreateTag(...) succeeded unexpectedly; expected an error")
	}
	if !strings.Contains(err.Error(), "status 400") || !strings.Contains(err.Error(), "Tag already exists") {
		t.Errorf("CreateTag(...) error = %q, want it to mention the status code and response body", err.Error())
	}
}

func TestCreateTag_UnreachableServer(t *testing.T) {
	err := CreateTag("http://127.0.0.1:0", "123", "1.4.4", "abc123", "unused")
	if err == nil {
		t.Fatal("CreateTag(...) against an unreachable server succeeded unexpectedly; expected an error")
	}
	if !strings.Contains(err.Error(), "failed to reach GitLab API") {
		t.Errorf("CreateTag(...) error = %q, want it to mention the unreachable API", err.Error())
	}
}

func TestCreateTag_InvalidAPIBaseURL(t *testing.T) {
	err := CreateTag(fmt.Sprintf("http://%s", string([]byte{0x7f})), "123", "1.4.4", "abc123", "unused")
	if err == nil {
		t.Fatal("CreateTag(...) with a malformed API base URL succeeded unexpectedly; expected an error")
	}
}

func TestCreateTag_NestedNamespaceProjectIDEscaped(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	if err := CreateTag(server.URL, "group/subgroup/project", "1.4.4", "abc123", "unused"); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error: %v", err)
	}
	if gotPath != "/projects/group%2Fsubgroup%2Fproject/repository/tags" {
		t.Errorf("request path = %q, want every \"/\" in a multi-level namespace path percent-encoded", gotPath)
	}
}

func TestCreateTag_TagNameWithSlashRoundTrips(t *testing.T) {
	// Slashes represent valid Git ref path segments. Query encoding MUST preserve
	// slash characters to prevent ref path corruption on remote endpoints.
	var gotTagName string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTagName = r.URL.Query().Get("tag_name")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	if err := CreateTag(server.URL, "123", "release/1.0.0", "abc123", "unused"); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error: %v", err)
	}
	if gotTagName != "release/1.0.0" {
		t.Errorf("tag_name = %q, want %q round-tripped exactly", gotTagName, "release/1.0.0")
	}
}

func TestCreateTag_RefWithPlusCharacterRoundTrips(t *testing.T) {
	// Unencoded '+' characters decode as spaces in HTTP query parameters. Force explicit '%2B'
	// percent-encoding to ensure literal plus characters in ref names survive transit.
	var gotRef string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRef = r.URL.Query().Get("ref")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	if err := CreateTag(server.URL, "123", "1.4.4", "feature+branch", "unused"); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error: %v", err)
	}
	if gotRef != "feature+branch" {
		t.Errorf("ref = %q, want %q round-tripped exactly (not decoded as a space)", gotRef, "feature+branch")
	}
}

func TestCreateTag_TagNameWithUnicodeRoundTrips(t *testing.T) {
	var gotTagName string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTagName = r.URL.Query().Get("tag_name")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	if err := CreateTag(server.URL, "123", "版本-1.0.0", "abc123", "unused"); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error: %v", err)
	}
	if gotTagName != "版本-1.0.0" {
		t.Errorf("tag_name = %q, want %q round-tripped exactly", gotTagName, "版本-1.0.0")
	}
}

func TestCreateTag_TokenWithCRLF_HeaderInjectionRejected(t *testing.T) {
	// CRLF sequences in header values enable HTTP response splitting and header injection.
	// Ensure client-side transport validation drops malformed tokens before socket writes.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler invoked; want the request rejected client-side before it reaches the server")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	err := CreateTag(server.URL, "123", "1.4.4", "abc123", "token-with-\r\ninjected-header: evil")
	if err == nil {
		t.Fatal("CreateTag(...) with a CRLF-bearing token succeeded unexpectedly; want it rejected")
	}
}

func TestCreateTag_RequiresExactly201_Rejects200(t *testing.T) {
	// Strict adherence to HTTP 201 Created prevents intermediate reverse proxies or
	// API gateways from masking unconfirmed resource creations via HTTP 200 OK responses.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := CreateTag(server.URL, "123", "1.4.4", "abc123", "unused")
	if err == nil {
		t.Fatal("CreateTag(...) succeeded unexpectedly on a 200 OK response; want only 201 Created accepted")
	}
	if !strings.Contains(err.Error(), "status 200") {
		t.Errorf("CreateTag(...) error = %q, want it to report status 200", err.Error())
	}
}

func TestCreateTag_ErrorStatusWithEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := CreateTag(server.URL, "123", "1.4.4", "abc123", "unused")
	if err == nil {
		t.Fatal("CreateTag(...) succeeded unexpectedly against a 500 response; want an error")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("CreateTag(...) error = %q, want it to report status 500 even with an empty response body", err.Error())
	}
}

func TestCreateTag_FollowsRedirect(t *testing.T) {
	// Intermediate reverse proxies may issue redirects. Redirect processing MUST preserve
	// the original POST method and payload without client side downgrade to GET.
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer final.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+r.URL.RequestURI(), http.StatusFound)
	}))
	defer redirector.Close()

	if err := CreateTag(redirector.URL, "123", "1.4.4", "abc123", "unused"); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error following a redirect: %v", err)
	}
}

func TestVerifyScope_Success(t *testing.T) {
	var gotPath, gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		_, _ = io.WriteString(w, `{"scopes":["api","read_api"],"active":true,"revoked":false}`)
	}))
	defer server.Close()

	if err := VerifyScope(server.URL, "secret-token", "api"); err != nil {
		t.Fatalf("VerifyScope(...) returned an unexpected error: %v", err)
	}
	if gotPath != "/personal_access_tokens/self" {
		t.Errorf("request path = %q, want %q", gotPath, "/personal_access_tokens/self")
	}
	if gotToken != "secret-token" {
		t.Errorf("PRIVATE-TOKEN header = %q, want %q", gotToken, "secret-token")
	}
}

func TestVerifyScope_MissingScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"scopes":["write_repository"],"active":true,"revoked":false}`)
	}))
	defer server.Close()

	err := VerifyScope(server.URL, "unused", "api")
	if err == nil {
		t.Fatal("VerifyScope(...) succeeded unexpectedly for a token missing the required scope")
	}
	if !strings.Contains(err.Error(), "do not include the required") {
		t.Errorf("VerifyScope(...) error = %q, want it to mention the missing scope", err.Error())
	}
}

func TestVerifyScope_Revoked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"scopes":["api"],"active":true,"revoked":true}`)
	}))
	defer server.Close()

	err := VerifyScope(server.URL, "unused", "api")
	if err == nil {
		t.Fatal("VerifyScope(...) succeeded unexpectedly for a revoked token")
	}
	if !strings.Contains(err.Error(), "revoked or inactive") {
		t.Errorf("VerifyScope(...) error = %q, want it to mention the revoked token", err.Error())
	}
}

func TestVerifyScope_Inactive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"scopes":["api"],"active":false,"revoked":false}`)
	}))
	defer server.Close()

	err := VerifyScope(server.URL, "unused", "api")
	if err == nil {
		t.Fatal("VerifyScope(...) succeeded unexpectedly for an inactive token")
	}
	if !strings.Contains(err.Error(), "revoked or inactive") {
		t.Errorf("VerifyScope(...) error = %q, want it to mention the inactive token", err.Error())
	}
}

func TestVerifyScope_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"401 Unauthorized"}`)
	}))
	defer server.Close()

	err := VerifyScope(server.URL, "unused", "api")
	if err == nil {
		t.Fatal("VerifyScope(...) succeeded unexpectedly on a 401 response")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Errorf("VerifyScope(...) error = %q, want it to mention the status code", err.Error())
	}
}

func TestVerifyScope_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `not json`)
	}))
	defer server.Close()

	err := VerifyScope(server.URL, "unused", "api")
	if err == nil {
		t.Fatal("VerifyScope(...) succeeded unexpectedly on a malformed response body")
	}
	if !strings.Contains(err.Error(), "failed to parse token introspection response") {
		t.Errorf("VerifyScope(...) error = %q, want it to mention the parse failure", err.Error())
	}
}

func TestVerifyScope_UnreachableServer(t *testing.T) {
	err := VerifyScope("http://127.0.0.1:0", "unused", "api")
	if err == nil {
		t.Fatal("VerifyScope(...) against an unreachable server succeeded unexpectedly")
	}
	if !strings.Contains(err.Error(), "failed to reach GitLab API") {
		t.Errorf("VerifyScope(...) error = %q, want it to mention the unreachable API", err.Error())
	}
}

func TestCreateTag_EmptyToken(t *testing.T) {
	var gotToken string
	var gotHeaderPresent bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		_, gotHeaderPresent = r.Header["Private-Token"]
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	// Client-side token validation is omitted to delegate credential authorization
	// exclusively to the remote API gateway via standard 401 Unauthorized responses.
	if err := CreateTag(server.URL, "123", "1.4.4", "abc123", ""); err != nil {
		t.Fatalf("CreateTag(...) returned an unexpected error for an empty token: %v", err)
	}
	if !gotHeaderPresent {
		t.Error("PRIVATE-TOKEN header absent from the request; want it present (empty) even for an empty token")
	}
	if gotToken != "" {
		t.Errorf("PRIVATE-TOKEN header = %q, want empty", gotToken)
	}
}

func newCrossHostRedirector(t *testing.T, leaked *atomic.Value) *httptest.Server {
	t.Helper()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("PRIVATE-TOKEN"))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"active":true,"scopes":["api"]}`))
	}))
	t.Cleanup(target.Close)
	targetURL := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetURL+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)
	return redirector
}

// A redirect to a different host MUST NOT carry the PRIVATE-TOKEN header.
func TestCreateTag_CrossHostRedirectDoesNotForwardToken(t *testing.T) {
	var leaked atomic.Value
	redirector := newCrossHostRedirector(t, &leaked)

	_ = CreateTag(redirector.URL, "1", "1.0.0", "abc", "secret-token")

	if got, _ := leaked.Load().(string); got != "" {
		t.Errorf("redirect target received PRIVATE-TOKEN %q", got)
	}
}

func TestVerifyScope_CrossHostRedirectDoesNotForwardToken(t *testing.T) {
	var leaked atomic.Value
	redirector := newCrossHostRedirector(t, &leaked)

	_ = VerifyScope(redirector.URL, "secret-token", "api")

	if got, _ := leaked.Load().(string); got != "" {
		t.Errorf("redirect target received PRIVATE-TOKEN %q", got)
	}
}

func TestCreateTag_TrailingSlashApiURLDoesNotProduceEmptyPathSegment(t *testing.T) {
	var gotPath atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath.Store(r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)

	_ = CreateTag(server.URL+"/api/v4/", "1", "1.0.0", "abc", "token")

	if path, _ := gotPath.Load().(string); strings.Contains(path, "//") {
		t.Errorf("request path %q contains an empty segment", path)
	}
}

// Two release jobs create one tag name. The API reports the loser with a client error, and
// CreateTag MUST surface that error to the caller.
func TestCreateTag_ConflictingWriterReceivesError(t *testing.T) {
	const writers = 16

	var created atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if created.CompareAndSwap(false, true) {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Error(w, `{"message":"Tag 1.0.0 already exists"}`, http.StatusConflict)
	}))
	t.Cleanup(server.Close)

	results := make([]error, writers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = CreateTag(server.URL, "1", "1.0.0", "abc", "token")
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Errorf("%d writers reported success, want exactly 1", successes)
	}
}

func TestCreateTag_ParameterEscapingBoundaries(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		ref  string
	}{
		{name: "ampersand in tag", tag: "a&b=c", ref: "abc"},
		{name: "hash in tag", tag: "a#b", ref: "abc"},
		{name: "percent in tag", tag: "a%2Fb", ref: "abc"},
		{name: "space in tag", tag: "a b", ref: "abc"},
		{name: "newline in tag", tag: "a\nb", ref: "abc"},
		{name: "unicode ref", tag: "1.0.0", ref: "主線"},
		{name: "empty tag", tag: "", ref: "abc"},
		{name: "empty ref", tag: "1.0.0", ref: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotTag, gotRef atomic.Value
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotTag.Store(r.URL.Query().Get("tag_name"))
				gotRef.Store(r.URL.Query().Get("ref"))
				w.WriteHeader(http.StatusCreated)
			}))
			t.Cleanup(server.Close)

			if err := CreateTag(server.URL, "1", tc.tag, tc.ref, "token"); err != nil {
				t.Fatalf("CreateTag returned an unexpected error: %v", err)
			}
			if got, _ := gotTag.Load().(string); got != tc.tag {
				t.Errorf("tag_name = %q, want %q", got, tc.tag)
			}
			if got, _ := gotRef.Load().(string); got != tc.ref {
				t.Errorf("ref = %q, want %q", got, tc.ref)
			}
		})
	}
}

func TestCreateTag_OversizedErrorBodyIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("x", 5<<20)))
	}))
	t.Cleanup(server.Close)

	err := CreateTag(server.URL, "1", "1.0.0", "abc", "token")
	if err == nil {
		t.Fatal("CreateTag succeeded unexpectedly")
	}
	if len(err.Error()) > 8192 {
		t.Errorf("error message holds %d bytes, want a bounded excerpt", len(err.Error()))
	}
}

func TestVerifyScope_ScopeListBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "scope present", body: `{"scopes":["api"],"active":true,"revoked":false}`},
		{name: "scope among many", body: `{"scopes":["read_api","api","read_user"],"active":true}`},
		{name: "scope case differs", body: `{"scopes":["API"],"active":true}`, wantErr: true},
		{name: "scope with padding", body: `{"scopes":[" api"],"active":true}`, wantErr: true},
		{name: "empty scopes", body: `{"scopes":[],"active":true}`, wantErr: true},
		{name: "null scopes", body: `{"scopes":null,"active":true}`, wantErr: true},
		{name: "missing scopes", body: `{"active":true}`, wantErr: true},
		{name: "inactive", body: `{"scopes":["api"],"active":false}`, wantErr: true},
		{name: "missing active", body: `{"scopes":["api"]}`, wantErr: true},
		{name: "revoked and active", body: `{"scopes":["api"],"active":true,"revoked":true}`, wantErr: true},
		{name: "scopes as string", body: `{"scopes":"api","active":true}`, wantErr: true},
		{name: "trailing garbage after object", body: `{"scopes":["api"],"active":true} garbage`},
		{name: "empty body", body: ``, wantErr: true},
		{name: "array body", body: `[]`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(server.Close)

			err := VerifyScope(server.URL, "token", "api")
			if tc.wantErr && err == nil {
				t.Error("VerifyScope succeeded unexpectedly")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("VerifyScope returned an unexpected error: %v", err)
			}
		})
	}
}
