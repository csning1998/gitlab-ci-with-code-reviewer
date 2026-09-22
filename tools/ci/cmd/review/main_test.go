package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// executeReviewSubprocess re-executes the test binary as the review command.
func executeReviewSubprocess(t *testing.T, extraEnv ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	return executeReviewSubprocessInDir(t, "", extraEnv...)
}

// executeReviewSubprocessInDir runs the review command with dir as its working directory.
func executeReviewSubprocessInDir(t *testing.T, dir string, extraEnv ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestReviewSubprocessHelper")
	cmd.Dir = dir
	baseEnv := []string{
		"BE_REVIEW_CMD=1",
		"CI_API_V4_URL=https://gitlab.example.com/api/v4",
		"CI_PROJECT_ID=1",
		"CI_MERGE_REQUEST_IID=2",
		"CLAUDE_MR_REVIEWER=",
		"GEMINI_MR_REVIEWER=",
		"CLAUDE_API_KEY=",
		"GEMINI_API_KEY=",
		"OPENAI_API_KEY=",
		"AZURE_OPENAI_API_KEY=",
		"GROK_API_KEY=",
		"LOCAL_API_KEY=",
		"REVIEW_BASE_URL=",
		"REVIEW_API_VERSION=",
		"VAULT_ADDR=",
		"VAULT_ID_TOKEN=",
		"VAULT_AUTH_MOUNT=",
		"VAULT_ROLE=",
		"VAULT_KV_MOUNT=",
		"VAULT_SECRET_PATH=",
		"VAULT_SECRET_FIELD=",
		"VAULT_CACERT=",
		"REVIEW_MR_REVIEWER=mock-gitlab-token",
		"REVIEW_API_KEY=mock-provider-key",
		"REVIEW_MODEL=claude-sonnet-5",
	}
	cmd.Env = append(os.Environ(), append(baseEnv, extraEnv...)...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		return 0, outBuf.String(), errBuf.String()
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("subprocess failed with unexpected error: %v", err)
	}
	return exitErr.ExitCode(), outBuf.String(), errBuf.String()
}

// TestReviewSubprocessHelper is re-executed in isolation to observe main() process exit behaviors.
func TestReviewSubprocessHelper(t *testing.T) {
	if os.Getenv("BE_REVIEW_CMD") != "1" {
		t.Skip("only runs as a re-executed subprocess")
	}
	main()
}

func TestReview_EnvironmentValidation_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		env      []string
		wantCode int
		wantWord string
	}{
		{
			name:     "missing gitlab token exits nonzero",
			env:      []string{"REVIEW_MR_REVIEWER="},
			wantCode: 1,
			wantWord: "REVIEW_MR_REVIEWER",
		},
		{
			name:     "missing provider key exits nonzero",
			env:      []string{"REVIEW_API_KEY="},
			wantCode: 1,
			wantWord: "CLAUDE_API_KEY",
		},
		{
			name:     "unconfigured model exits nonzero",
			env:      []string{"REVIEW_MODEL="},
			wantCode: 1,
			wantWord: "REVIEW_MODEL",
		},
		{
			name:     "unapproved model exits nonzero",
			env:      []string{"REVIEW_MODEL=deepseek-r1"},
			wantCode: 1,
			wantWord: "deepseek-r1",
		},
		{
			name: "half configured vault federation exits nonzero",
			env: []string{
				"REVIEW_API_KEY=mock-provider-key",
				"VAULT_ADDR=https://vault.example.com",
			},
			wantCode: 1,
			wantWord: "VAULT_ID_TOKEN",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := executeReviewSubprocess(t, tc.env...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d", code, tc.wantCode)
			}
			if !strings.Contains(stderr, tc.wantWord) {
				t.Errorf("stderr = %q, want mention of %q", stderr, tc.wantWord)
			}
		})
	}
}

func TestReview_ResolvesProviderKeyForSelectedModel(t *testing.T) {
	server := newEmptyChangesGitLabServer(t)
	code, _, stderr := executeReviewSubprocess(t,
		"CI_API_V4_URL="+server.URL,
		"REVIEW_API_KEY=",
		"REVIEW_MODEL=grok-4.6",
		"GROK_API_KEY=mock-grok-key",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
}

func TestReview_EmptyChanges_SucceedsWithoutLLMCall(t *testing.T) {
	server := newEmptyChangesGitLabServer(t)
	code, _, stderr := executeReviewSubprocess(t, "CI_API_V4_URL="+server.URL)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
}

func TestReview_GitLabError_ExitsNonzero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	code, _, stderr := executeReviewSubprocess(t, "CI_API_V4_URL="+server.URL)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if len(stderr) == 0 {
		t.Error("stderr is empty, want error output on review failure")
	}
}

func writeDeclarationDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gitlab"), 0o755); err != nil {
		t.Fatalf("create declaration directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitlab", "reviewer.yml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write declaration: %v", err)
	}
	return dir
}

func TestReview_DeclarationSlotResolvesDeclaredModel(t *testing.T) {
	dir := writeDeclarationDir(t, `
defaults:
  max_tokens: 4096
models:
  fast-reviewer:
    model: gemini-2.5-flash
    temperature: 0.2
slots:
  primary: fast-reviewer
`)
	server := newEmptyChangesGitLabServer(t)

	code, _, stderr := executeReviewSubprocessInDir(t, dir,
		"CI_API_V4_URL="+server.URL,
		"REVIEW_MODEL=primary",
		"REVIEW_API_KEY=",
		"GEMINI_API_KEY=mock-gemini-key",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
}

func TestReview_DeclarationNamedModelResolvesDirectly(t *testing.T) {
	dir := writeDeclarationDir(t, `
models:
  deep-reviewer:
    model: grok-4.6
    reasoning_level: xhigh
`)
	server := newEmptyChangesGitLabServer(t)

	code, _, stderr := executeReviewSubprocessInDir(t, dir,
		"CI_API_V4_URL="+server.URL,
		"REVIEW_MODEL=deep-reviewer",
		"REVIEW_API_KEY=",
		"GROK_API_KEY=mock-grok-key",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
}

func TestReview_SlotBoundToUndeclaredModelNamesTheDanglingBinding(t *testing.T) {
	// The failure MUST name the undeclared model. The fallback message names the slot as an
	// unapproved model id, which hides the broken binding.
	dir := writeDeclarationDir(t, `
models:
  declared-reviewer:
    model: grok-4.6
slots:
  primary: undeclared-reviewer
`)

	code, _, stderr := executeReviewSubprocessInDir(t, dir,
		"REVIEW_MODEL=primary",
		"REVIEW_API_KEY=mock-provider-key",
	)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "undeclared-reviewer") {
		t.Errorf("stderr = %q, want the undeclared model %q named", stderr, "undeclared-reviewer")
	}
}

func TestReview_FederationCredentialReplacesStaticKey(t *testing.T) {
	server := newEmptyChangesGitLabServer(t)

	code, _, stderr := executeReviewSubprocess(t,
		"CI_API_V4_URL="+server.URL,
		"REVIEW_API_KEY=",
		"VAULT_ADDR=https://vault.example.com",
		"VAULT_ID_TOKEN=header.payload.signature",
		"VAULT_ROLE=ci-code-reviewer",
		"VAULT_KV_MOUNT=secret",
		"VAULT_SECRET_PATH=ci/credentials",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
}

func newEmptyChangesGitLabServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`{"title":"feat: test","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// newReviewFixtureServers serves one changed file to the reviewer and records the body of the
// single chat completion request sent to the language model endpoint.
func newReviewFixtureServers(t *testing.T) (gitlabURL, llmURL string, llmBody func() string) {
	t.Helper()

	var mu sync.Mutex
	var recorded string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		recorded = string(raw)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[]"}}]}`))
	}))
	t.Cleanup(llm.Close)

	gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			_, _ = w.Write([]byte(`[{"new_path":"a.go","old_path":"a.go","diff":"@@ -0,0 +1 @@\n+x\n"}]`))
			return
		}
		_, _ = w.Write([]byte(`{"title":"feat: t","description":"","diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`))
	}))
	t.Cleanup(gitlab.Close)

	return gitlab.URL, llm.URL, func() string {
		mu.Lock()
		defer mu.Unlock()
		return recorded
	}
}

func TestReview_DeclaredPromptReachesLanguageModel(t *testing.T) {
	tests := []struct {
		name        string
		declaration string
		promptFile  string
		marker      string
	}{
		{
			name:        "inline prompt",
			declaration: "    prompt: CUSTOM-INLINE-MARKER\n",
			marker:      "CUSTOM-INLINE-MARKER",
		},
		{
			name:        "prompt file",
			declaration: "    prompt_file: prompt.md\n",
			promptFile:  "CUSTOM-FILE-MARKER",
			marker:      "CUSTOM-FILE-MARKER",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertDeclaredPromptReachesLanguageModel(t, tc.declaration, tc.promptFile, tc.marker)
		})
	}
}

// assertDeclaredPromptReachesLanguageModel fails the test unless a review subprocess run against
// declaration (and, when promptFile is non-empty, a prompt.md file holding promptFile) exits 0
// and the request recorded by the fixture language model contains marker.
func assertDeclaredPromptReachesLanguageModel(t *testing.T, declaration, promptFile, marker string) {
	t.Helper()

	gitlabURL, llmURL, llmBody := newReviewFixtureServers(t)
	dir := writeDeclarationDir(t, fmt.Sprintf(
		"models:\n  custom:\n    model: llama-3.3-70b\n    base_url: %s\n%s", llmURL, declaration))
	if promptFile != "" {
		if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte(promptFile), 0o600); err != nil {
			t.Fatalf("write prompt file: %v", err)
		}
	}

	code, _, stderr := executeReviewSubprocessInDir(t, dir,
		"CI_API_V4_URL="+gitlabURL,
		"REVIEW_MODEL=custom",
		"REVIEW_API_KEY=",
		"LOCAL_API_KEY=mock-local-key",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if llmBody() == "" {
		t.Fatal("no request reached the language model endpoint")
	}
	if !strings.Contains(llmBody(), marker) {
		t.Errorf("language model request lacks the declared prompt %q", marker)
	}
}

// The declaration comes from the repository under review. A prompt_file outside the working
// directory MUST be rejected before any file content reaches the language model.

// EvalSymlinks returns an absolute path when a symlink target is itself absolute, even for a
// relative input path.
func TestConfinePromptFile_RelativeSymlinkToAbsoluteOutsidePathIsRejected(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if _, err := confinePromptFile("link.md"); err == nil {
		t.Error("confinePromptFile(\"link.md\") succeeded unexpectedly for a relative symlink whose target resolves outside the working directory")
	}
}

func TestReview_PromptFileOutsideWorkspaceIsRejected(t *testing.T) {
	tests := []struct {
		name  string
		value func(dir, outside string) string
		setup func(t *testing.T, dir, outside string)
	}{
		{
			name:  "absolute path",
			value: func(_, outside string) string { return outside },
		},
		{
			name: "parent traversal",
			value: func(dir, outside string) string {
				rel, err := filepath.Rel(dir, outside)
				if err != nil {
					t.Fatalf("relative path: %v", err)
				}
				return filepath.ToSlash(rel)
			},
		},
		{
			name:  "symlink to an outside file",
			value: func(_, _ string) string { return "link.md" },
			setup: func(t *testing.T, dir, outside string) {
				if err := os.Symlink(outside, filepath.Join(dir, "link.md")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
		},
		{
			name:  "working directory itself",
			value: func(_, _ string) string { return "." },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertPromptFileOutsideWorkspaceRejected(t, tc.value, tc.setup)
		})
	}
}

// assertPromptFileOutsideWorkspaceRejected fails the test unless a review subprocess run with a
// declared prompt_file computed by value (after an optional setup step) exits 1, names
// prompt_file in stderr, and never forwards the content of the outside file to the language model.
func assertPromptFileOutsideWorkspaceRejected(
	t *testing.T,
	value func(dir, outside string) string,
	setup func(t *testing.T, dir, outside string),
) {
	t.Helper()

	gitlabURL, llmURL, llmBody := newReviewFixtureServers(t)
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("OUTSIDE-SECRET-MARKER"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	dir := writeDeclarationDir(t, "models:\n  custom:\n    model: llama-3.3-70b\n    base_url: "+llmURL+"\n")
	if setup != nil {
		setup(t, dir, outside)
	}
	declaration := fmt.Sprintf("models:\n  custom:\n    model: llama-3.3-70b\n    base_url: %s\n    prompt_file: %q\n",
		llmURL, value(dir, outside))
	if err := os.WriteFile(filepath.Join(dir, ".gitlab", "reviewer.yml"), []byte(declaration), 0o600); err != nil {
		t.Fatalf("rewrite declaration: %v", err)
	}

	code, _, stderr := executeReviewSubprocessInDir(t, dir,
		"CI_API_V4_URL="+gitlabURL,
		"REVIEW_MODEL=custom",
		"REVIEW_API_KEY=",
		"LOCAL_API_KEY=mock-local-key",
	)
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "prompt_file") {
		t.Errorf("stderr = %q, want prompt_file named", stderr)
	}
	if strings.Contains(llmBody(), "OUTSIDE-SECRET-MARKER") {
		t.Error("content of a file outside the working directory reached the language model")
	}
}
