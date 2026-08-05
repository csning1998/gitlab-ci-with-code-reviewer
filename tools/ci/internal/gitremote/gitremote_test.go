package gitremote

import (
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

func TestConfigureOriginRemoteURL_NoExistingRemote(t *testing.T) {
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}

	remote, err := ConfigureOriginRemoteURL(repo, "https://example.invalid/repo.git")
	if err != nil {
		t.Fatalf("ConfigureOriginRemoteURL(...) returned an unexpected error: %v", err)
	}
	if remote.Config().Name != Name {
		t.Errorf("remote name = %q, want %q", remote.Config().Name, Name)
	}
	if len(remote.Config().URLs) != 1 || remote.Config().URLs[0] != "https://example.invalid/repo.git" {
		t.Errorf("remote URLs = %v, want [\"https://example.invalid/repo.git\"]", remote.Config().URLs)
	}
}

func TestConfigureOriginRemoteURL_EmptyURLAccepted(t *testing.T) {
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}

	// RemoteConfig.Validate rejects nil or empty URL slices, not empty URL strings.
	// Set persists empty URL inputs without validation error.
	remote, err := ConfigureOriginRemoteURL(repo, "")
	if err != nil {
		t.Fatalf("ConfigureOriginRemoteURL(...) returned an unexpected error: %v", err)
	}
	if len(remote.Config().URLs) != 1 || remote.Config().URLs[0] != "" {
		t.Errorf("remote URLs = %v, want a single empty-string URL", remote.Config().URLs)
	}
}

func TestConfigureOriginRemoteURL_PopulatesDefaultFetchRefSpec(t *testing.T) {
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}

	remote, err := ConfigureOriginRemoteURL(repo, "https://example.invalid/repo.git")
	if err != nil {
		t.Fatalf("ConfigureOriginRemoteURL(...) returned an unexpected error: %v", err)
	}

	wantRefSpec := config.RefSpec("+refs/heads/*:refs/remotes/" + Name + "/*")
	if len(remote.Config().Fetch) != 1 || remote.Config().Fetch[0] != wantRefSpec {
		t.Errorf("Fetch refspecs = %v, want [%q]", remote.Config().Fetch, wantRefSpec)
	}
}

func TestConfigureOriginRemoteURL_ReplacesCustomFetchRefSpecOnExistingRemote(t *testing.T) {
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}
	customRefSpec := config.RefSpec("+refs/heads/main:refs/remotes/" + Name + "/main")
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name:  Name,
		URLs:  []string{"https://example.invalid/original.git"},
		Fetch: []config.RefSpec{customRefSpec},
	}); err != nil {
		t.Fatalf("failed to seed pre-existing origin remote configuration: %v", err)
	}

	remote, err := ConfigureOriginRemoteURL(repo, "https://example.invalid/replacement.git")
	if err != nil {
		t.Fatalf("ConfigureOriginRemoteURL(...) returned an unexpected error: %v", err)
	}

	// Set consistently constructs a new RemoteConfig instance lacking Fetch entries;
	// therefore, pre-existing custom refspec definitions are discarded instead of retained.
	wantDefault := config.RefSpec("+refs/heads/*:refs/remotes/" + Name + "/*")
	if len(remote.Config().Fetch) != 1 || remote.Config().Fetch[0] != wantDefault {
		t.Errorf("Fetch refspecs = %v, want the default [%q], not the prior custom refspec", remote.Config().Fetch, wantDefault)
	}
}

func TestConfigureOriginRemoteURL_LeavesOtherRemotesUntouched(t *testing.T) {
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "upstream",
		URLs: []string{"https://example.invalid/upstream.git"},
	}); err != nil {
		t.Fatalf("failed to seed pre-existing upstream remote configuration: %v", err)
	}

	if _, err := ConfigureOriginRemoteURL(repo, "https://example.invalid/repo.git"); err != nil {
		t.Fatalf("ConfigureOriginRemoteURL(...) returned an unexpected error: %v", err)
	}

	upstream, err := repo.Remote("upstream")
	if err != nil {
		t.Fatalf("Remote(\"upstream\") failed: %v", err)
	}
	if upstream.Config().URLs[0] != "https://example.invalid/upstream.git" {
		t.Errorf("upstream remote URL = %q, want it unchanged", upstream.Config().URLs[0])
	}
}

func TestConfigureOriginRemoteURL_CalledTwice_LastWriteWins(t *testing.T) {
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}

	if _, err := ConfigureOriginRemoteURL(repo, "https://example.invalid/first.git"); err != nil {
		t.Fatalf("first ConfigureOriginRemoteURL(...) returned an unexpected error: %v", err)
	}
	remote, err := ConfigureOriginRemoteURL(repo, "https://example.invalid/second.git")
	if err != nil {
		t.Fatalf("second ConfigureOriginRemoteURL(...) returned an unexpected error: %v", err)
	}

	if remote.Config().URLs[0] != "https://example.invalid/second.git" {
		t.Errorf("remote URL = %q, want the URL from the second call", remote.Config().URLs[0])
	}
	reopened, err := repo.Remote(Name)
	if err != nil {
		t.Fatalf("Remote(%q) failed: %v", Name, err)
	}
	if reopened.Config().URLs[0] != "https://example.invalid/second.git" {
		t.Errorf("persisted remote URL = %q, want the URL from the second call", reopened.Config().URLs[0])
	}
}

func TestConfigureOriginRemoteURL_URLWithEmbeddedCredentials(t *testing.T) {
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}

	credentialURL := "https://gitlab-ci-token:secret-token@example.invalid/repo.git"
	remote, err := ConfigureOriginRemoteURL(repo, credentialURL)
	if err != nil {
		t.Fatalf("ConfigureOriginRemoteURL(...) returned an unexpected error: %v", err)
	}

	// Set executes neither URL sanitization nor credential extraction;
	// callers maintain sole responsibility for parameters embedded within remoteURL.
	if remote.Config().URLs[0] != credentialURL {
		t.Errorf("remote URL = %q, want it persisted verbatim including embedded credentials", remote.Config().URLs[0])
	}
}

func TestConfigureOriginRemoteURL_BareRepository(t *testing.T) {
	repo, err := git.PlainInit(t.TempDir(), true)
	if err != nil {
		t.Fatalf("PlainInit(..., bare) failed: %v", err)
	}

	remote, err := ConfigureOriginRemoteURL(repo, "https://example.invalid/repo.git")
	if err != nil {
		t.Fatalf("ConfigureOriginRemoteURL(...) returned an unexpected error: %v", err)
	}
	if remote.Config().URLs[0] != "https://example.invalid/repo.git" {
		t.Errorf("remote URL = %q, want the configured URL", remote.Config().URLs[0])
	}
}

func TestConfigureOriginRemoteURL_OverwritesExistingRemote(t *testing.T) {
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: Name,
		URLs: []string{"https://example.invalid/original.git"},
	}); err != nil {
		t.Fatalf("failed to seed pre-existing origin remote configuration: %v", err)
	}

	remote, err := ConfigureOriginRemoteURL(repo, "https://example.invalid/replacement.git")
	if err != nil {
		t.Fatalf("ConfigureOriginRemoteURL(...) returned an unexpected error: %v", err)
	}
	if remote.Config().URLs[0] != "https://example.invalid/replacement.git" {
		t.Errorf("remote URL = %q, want the replacement URL", remote.Config().URLs[0])
	}

	reopened, err := repo.Remote(Name)
	if err != nil {
		t.Fatalf("Remote(%q) failed: %v", Name, err)
	}
	if reopened.Config().URLs[0] != "https://example.invalid/replacement.git" {
		t.Errorf("persisted remote URL = %q, want the replacement URL", reopened.Config().URLs[0])
	}
}
