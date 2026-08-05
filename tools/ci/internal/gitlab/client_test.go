package gitlab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew_BuildsMRURL(t *testing.T) {
	c := New("https://gitlab.example.com/api/v4", "42", "7", "token")
	if c.mrURL != "https://gitlab.example.com/api/v4/projects/42/merge_requests/7" {
		t.Errorf("mrURL = %q, want the projects/.../merge_requests/... path composed from the inputs", c.mrURL)
	}
}

func TestFetchMRDescription_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("request method = %q, want GET", r.Method)
		}
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "secret" {
			t.Errorf("PRIVATE-TOKEN header = %q, want %q", got, "secret")
		}
		_, _ = w.Write([]byte(`{"title":"t","description":"a long description"}`))
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "secret")
	desc, err := c.FetchMRDescription()
	if err != nil {
		t.Fatalf("FetchMRDescription() returned an unexpected error: %v", err)
	}
	if desc != "a long description" {
		t.Errorf("FetchMRDescription() = %q, want %q", desc, "a long description")
	}
}

func TestFetchMRDescription_HTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "bad-token")
	if _, err := c.FetchMRDescription(); err == nil {
		t.Error("FetchMRDescription() succeeded unexpectedly against a 401 response; want an error")
	}
}

func TestFetchMRDescription_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	if _, err := c.FetchMRDescription(); err == nil {
		t.Error("FetchMRDescription() succeeded unexpectedly on malformed JSON; want an error")
	}
}

func TestFetchMRDescription_UnreachableServer(t *testing.T) {
	c := New("http://127.0.0.1:1", "1", "2", "token")
	if _, err := c.FetchMRDescription(); err == nil {
		t.Error("FetchMRDescription() succeeded unexpectedly against an unreachable host; want an error")
	}
}

func TestFetchMR_Success_UsesDiffsEndpoint(t *testing.T) {
	var diffsPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/diffs"):
			diffsPath = r.URL.RequestURI()
			_, _ = w.Write([]byte(`[{"new_path":"a.go","old_path":"a.go","diff":"@@ -1,1 +1,1 @@\n+x\n"}]`))
		default:
			_, _ = w.Write([]byte(`{"title":"t","description":"d","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`))
		}
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	mr, err := c.FetchMR()
	if err != nil {
		t.Fatalf("FetchMR() returned an unexpected error: %v", err)
	}
	if mr.Title != "t" || mr.Description != "d" {
		t.Errorf("FetchMR() = %+v, want title/description populated from the detail endpoint", mr)
	}
	if len(mr.Changes) != 1 || mr.Changes[0].NewPath != "a.go" {
		t.Errorf("FetchMR().Changes = %+v, want a single a.go entry from the diffs endpoint", mr.Changes)
	}
	if mr.DiffRefs.BaseSha != "b" {
		t.Errorf("FetchMR().DiffRefs = %+v, want base_sha \"b\"", mr.DiffRefs)
	}
	if !strings.Contains(diffsPath, "per_page=100") {
		t.Errorf("diffs request path = %q, want it to request per_page=100", diffsPath)
	}
}

func TestFetchMR_DetailFetchFails_DiffsNeverRequested(t *testing.T) {
	var diffsRequested bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			diffsRequested = true
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	if _, err := c.FetchMR(); err == nil {
		t.Error("FetchMR() succeeded unexpectedly despite a detail-fetch failure; want an error")
	}
	if diffsRequested {
		t.Error("FetchMR() requested the diffs endpoint despite the detail fetch failing; want it short-circuited")
	}
}

func TestFetchMR_DiffsFetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"title":"t","description":"d"}`))
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	if _, err := c.FetchMR(); err == nil {
		t.Error("FetchMR() succeeded unexpectedly despite a diffs-fetch failure; want an error")
	}
}

func TestFetchMR_NoChanges_EmptyArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`{"title":"t","description":"d"}`))
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	mr, err := c.FetchMR()
	if err != nil {
		t.Fatalf("FetchMR() returned an unexpected error: %v", err)
	}
	if len(mr.Changes) != 0 {
		t.Errorf("FetchMR().Changes = %v, want an empty slice", mr.Changes)
	}
}

func TestFetchMR_PaginationWarning_DoesNotFail(t *testing.T) {
	// A truncated file list (>100 changed files) must still return successfully; the caller only
	// receives a printed warning, not an error, since GitLab paginates rather than rejecting.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[{"new_path":"a.go"}]`))
			return
		}
		_, _ = w.Write([]byte(`{"title":"t","description":"d"}`))
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	mr, err := c.FetchMR()
	if err != nil {
		t.Fatalf("FetchMR() returned an unexpected error: %v", err)
	}
	if len(mr.Changes) != 1 {
		t.Errorf("FetchMR().Changes = %v, want the first page's single entry", mr.Changes)
	}
}

func TestFetchMR_DiffsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			_, _ = w.Write([]byte(`not json`))
			return
		}
		_, _ = w.Write([]byte(`{"title":"t","description":"d"}`))
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	if _, err := c.FetchMR(); err == nil {
		t.Error("FetchMR() succeeded unexpectedly on malformed diffs JSON; want an error")
	}
}

func TestPostDiscussion_SendsBodyAndPosition(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("request method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	status, err := c.PostDiscussion("body text", map[string]any{"new_line": float64(5)})
	if err != nil {
		t.Fatalf("PostDiscussion(...) returned an unexpected error: %v", err)
	}
	if status != http.StatusCreated {
		t.Errorf("PostDiscussion(...) status = %d, want %d", status, http.StatusCreated)
	}
	if captured["body"] != "body text" {
		t.Errorf("request body = %v, want \"body\" = \"body text\"", captured)
	}
}

func TestPostDiscussion_NilPosition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	if _, err := c.PostDiscussion("body", nil); err != nil {
		t.Fatalf("PostDiscussion(...) returned an unexpected error: %v", err)
	}
}

func TestPostDiscussion_ErrorStatusPropagated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"line outside diff context"}`))
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	status, err := c.PostDiscussion("body", map[string]any{})
	if err == nil {
		t.Fatal("PostDiscussion(...) succeeded unexpectedly against a 422 response; want an error")
	}
	if status != http.StatusUnprocessableEntity {
		t.Errorf("PostDiscussion(...) status = %d, want %d", status, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(err.Error(), "line outside diff context") {
		t.Errorf("PostDiscussion(...) error = %q, want it to include the response body", err.Error())
	}
}

func TestPostNote_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/notes") {
			t.Errorf("request path = %q, want it to target /notes", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	status, err := c.PostNote("note body")
	if err != nil {
		t.Fatalf("PostNote(...) returned an unexpected error: %v", err)
	}
	if status != http.StatusCreated {
		t.Errorf("PostNote(...) status = %d, want %d", status, http.StatusCreated)
	}
}

func TestAddLabels_JoinsWithComma(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("request method = %q, want PUT", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	if _, err := c.AddLabels([]string{"type::fix", "breaking-change"}); err != nil {
		t.Fatalf("AddLabels(...) returned an unexpected error: %v", err)
	}
	if captured["add_labels"] != "type::fix,breaking-change" {
		t.Errorf("request body add_labels = %v, want a comma-joined label list", captured["add_labels"])
	}
}

func TestAddLabels_EmptySlice(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	if _, err := c.AddLabels(nil); err != nil {
		t.Fatalf("AddLabels(nil) returned an unexpected error: %v", err)
	}
	if captured["add_labels"] != "" {
		t.Errorf("request body add_labels = %v, want an empty string for a nil label slice", captured["add_labels"])
	}
}

func TestExecuteHTTPRequest_RequestHeaderCarriesToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "ci-job-token" {
			t.Errorf("PRIVATE-TOKEN header = %q, want %q", got, "ci-job-token")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "ci-job-token")
	if _, err := c.PostNote("x"); err != nil {
		t.Fatalf("PostNote(...) returned an unexpected error: %v", err)
	}
}

func TestExecuteHTTPRequest_GETRequestOmitsContentTypeHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "" {
			t.Errorf("Content-Type header = %q on a GET request with no body, want empty", ct)
		}
		_, _ = w.Write([]byte(`{"title":"t"}`))
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	if _, err := c.FetchMRDescription(); err != nil {
		t.Fatalf("FetchMRDescription() returned an unexpected error: %v", err)
	}
}

func TestExecuteHTTPRequest_ErrorStatusIncludesResponseBodyVerbatim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"insufficient scope"}`))
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "1", "2", "token")
	_, err := c.FetchMRDescription()
	if err == nil {
		t.Fatal("FetchMRDescription() succeeded unexpectedly against a 403 response; want an error")
	}
	if !strings.Contains(err.Error(), "insufficient scope") {
		t.Errorf("error = %q, want the response body embedded verbatim", err.Error())
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want the status code included", err.Error())
	}
}
