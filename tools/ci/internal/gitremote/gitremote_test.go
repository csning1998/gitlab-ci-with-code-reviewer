package gitremote

import (
	"fmt"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
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

// Two writers update the repository configuration through separate handles under config.lock.
// Each update MUST survive in the final configuration.
func TestConfigureOriginRemoteURL_ConcurrentConfigWritersDoNotLoseUpdates(t *testing.T) {
	const rounds = 30

	for round := 0; round < rounds; round++ {
		dir := t.TempDir()
		if _, err := git.PlainInit(dir, false); err != nil {
			t.Fatalf("init repository: %v", err)
		}

		remoteURL := fmt.Sprintf("https://example.com/round-%d.git", round)
		errs := raceConfigWriters(dir, remoteURL)
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d: writer %d returned an unexpected error: %v", round, i, err)
			}
		}
		assertConfigSurvivedConcurrentWrites(t, dir, remoteURL, round)
	}
}

// raceConfigWriters starts two goroutines at the same instant, one setting the origin remote URL
// through ConfigureOriginRemoteURL and the other setting user.name through updateConfig, and
// returns the error of each goroutine in writer order.
func raceConfigWriters(dir, remoteURL string) []error {
	start := make(chan struct{})
	errs := make([]error, 2)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		repo, err := git.PlainOpen(dir)
		if err != nil {
			errs[0] = err
			return
		}
		_, errs[0] = ConfigureOriginRemoteURL(repo, remoteURL)
	}()
	go func() {
		defer wg.Done()
		<-start
		repo, err := git.PlainOpen(dir)
		if err != nil {
			errs[1] = err
			return
		}
		errs[1] = updateConfig(repo, func(cfg *config.Config) {
			cfg.Raw.Section("user").SetOption("name", "boundary")
		})
	}()
	close(start)
	wg.Wait()
	return errs
}

// assertConfigSurvivedConcurrentWrites fails the test unless both the origin remote URL and
// user.name set by raceConfigWriters are present in the final configuration of the repository.
func assertConfigSurvivedConcurrentWrites(t *testing.T, dir, remoteURL string, round int) {
	t.Helper()

	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("round %d: reopen repository: %v", round, err)
	}
	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("round %d: read configuration: %v", round, err)
	}
	if remote, ok := cfg.Remotes[Name]; !ok || len(remote.URLs) != 1 || remote.URLs[0] != remoteURL {
		t.Fatalf("round %d: origin remote lost or altered: %+v", round, cfg.Remotes[Name])
	}
	if got := cfg.Raw.Section("user").Option("name"); got != "boundary" {
		t.Fatalf("round %d: user.name = %q, want the concurrent update to survive", round, got)
	}
}

// updateConfig falls back to a mutex-serialized read-mutate-write when the storer is not
// *filesystem.Storage. A memory-backed repository exercises that fallback.
func TestUpdateConfig_NonFilesystemStorerDoesNotLoseConcurrentUpdates(t *testing.T) {
	const writers = 16

	repo, err := git.Init(memory.NewStorage(), nil)
	if err != nil {
		t.Fatalf("init in-memory repository: %v", err)
	}

	start := make(chan struct{})
	errs := make([]error, writers)

	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = updateConfig(repo, func(cfg *config.Config) {
				cfg.Raw.Section("writer").SetOption(fmt.Sprintf("w%d", i), "set")
			})
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d returned an unexpected error: %v", i, err)
		}
	}

	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("read configuration: %v", err)
	}
	section := cfg.Raw.Section("writer")
	missing := 0
	for i := 0; i < writers; i++ {
		if section.Option(fmt.Sprintf("w%d", i)) == "" {
			missing++
		}
	}
	if missing != 0 {
		t.Fatalf("%d of %d concurrent writes to a non-filesystem storer went missing; want every write to survive", missing, writers)
	}
}

func TestConfigureOriginRemoteURL_URLSpellingBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "https", url: "https://gitlab.com/group/project.git"},
		{name: "ssh scp form", url: "git@gitlab.com:group/project.git"},
		{name: "file path", url: "/srv/git/project.git"},
		{name: "url with newline", url: "https://gitlab.com/a\nb.git", wantErr: true},
		{name: "url with nul byte", url: "https://gitlab.com/a\x00b.git", wantErr: true},
		{name: "url with leading space", url: " https://gitlab.com/a.git", wantErr: true},
		{name: "url with trailing space", url: "https://gitlab.com/a.git ", wantErr: true},
		{name: "url with carriage return", url: "https://gitlab.com/a.git\r", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := git.PlainInit(t.TempDir(), false)
			if err != nil {
				t.Fatalf("init repository: %v", err)
			}
			_, err = ConfigureOriginRemoteURL(repo, tc.url)
			if tc.wantErr && err == nil {
				t.Errorf("ConfigureOriginRemoteURL(%q) succeeded unexpectedly", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ConfigureOriginRemoteURL(%q) returned an unexpected error: %v", tc.url, err)
			}
		})
	}
}
