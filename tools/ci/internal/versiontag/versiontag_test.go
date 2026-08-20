package versiontag

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "versioning.yml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write versioning config fixture: %v", err)
	}
	return path
}

func newRepoWithCommit(t *testing.T) (*git.Repository, string) {
	t.Helper()
	return newRepoWithFiles(t, map[string]string{"README.md": "test\n"})
}

func newRepoWithFiles(t *testing.T, files map[string]string) (*git.Repository, string) {
	t.Helper()
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit(...) failed: %v", err)
	}
	sha := commitFiles(t, repo, files)
	return repo, sha
}

func addCommitWithFiles(t *testing.T, repo *git.Repository, files map[string]string) string {
	t.Helper()
	return commitFiles(t, repo, files)
}

func commitFiles(t *testing.T, repo *git.Repository, files map[string]string) string {
	t.Helper()

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}

	root := worktree.Filesystem.Root()
	for relPath, content := range files {
		fullPath := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", fullPath, err)
		}
		if _, err := worktree.Add(relPath); err != nil {
			t.Fatalf("Add(%q) failed: %v", relPath, err)
		}
	}

	sha, err := worktree.Commit("test commit", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}
	return sha.String()
}

func mustTag(t *testing.T, repo *git.Repository, tagName, sha string) {
	t.Helper()
	if _, err := repo.CreateTag(tagName, plumbing.NewHash(sha), nil); err != nil {
		t.Fatalf("CreateTag(%q) failed: %v", tagName, err)
	}
}

func createLatestTagFixtures(t *testing.T, repo *git.Repository, sha string, tags []string, annotated bool) {
	t.Helper()
	for _, name := range tags {
		if !annotated {
			mustTag(t, repo, name, sha)
			continue
		}
		if _, err := repo.CreateTag(name, plumbing.NewHash(sha), &git.CreateTagOptions{
			Tagger:  &object.Signature{Name: "test", Email: "test@example.com"},
			Message: "annotated",
		}); err != nil {
			t.Fatalf("CreateTag(%q) failed: %v", name, err)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.Modules) != 1 {
		t.Fatalf("DefaultConfig() returned %d modules, want 1", len(cfg.Modules))
	}
	mod := cfg.Modules[0]
	if mod.Name != "" {
		t.Errorf("DefaultConfig() Name = %q, want empty", mod.Name)
	}
	if mod.Dir != "." {
		t.Errorf("DefaultConfig() Dir = %q, want %q", mod.Dir, ".")
	}
	if mod.TagPrefix != nil {
		t.Errorf("DefaultConfig() TagPrefix = %v, want nil so Prefix derives from the empty Name", mod.TagPrefix)
	}
	if got := mod.Prefix(); got != "" {
		t.Errorf("DefaultConfig().Prefix() = %q, want empty", got)
	}
}

func TestDefaultConfig_CallsAreIndependent(t *testing.T) {
	first := DefaultConfig()
	second := DefaultConfig()
	first.Modules[0].Dir = "mutated"
	first.Modules[0].Name = "mutated"
	if second.Modules[0].Dir != "." || second.Modules[0].Name != "" {
		t.Errorf("DefaultConfig() shares backing state across calls: second = %+v", second.Modules[0])
	}
}

func TestLoadConfig_Success(t *testing.T) {
	path := writeConfig(t, `
modules:
  - name: ""
    dir: "."
  - name: reviewer
    dir: tools/ci
    tag_prefix: "reviewer-"
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig(%q) returned an unexpected error: %v", path, err)
	}
	if len(cfg.Modules) != 2 {
		t.Fatalf("LoadConfig(%q) returned %d modules, want 2", path, len(cfg.Modules))
	}
	if cfg.Modules[0].Name != "" || cfg.Modules[0].Dir != "." {
		t.Errorf("Modules[0] = %+v, want empty Name and Dir %q", cfg.Modules[0], ".")
	}
	if cfg.Modules[0].TagPrefix != nil {
		t.Errorf("Modules[0].TagPrefix = %v, want nil when the key is omitted", cfg.Modules[0].TagPrefix)
	}
	if cfg.Modules[1].Name != "reviewer" || cfg.Modules[1].Dir != "tools/ci" {
		t.Errorf("Modules[1] = %+v, want name reviewer and dir tools/ci", cfg.Modules[1])
	}
	if cfg.Modules[1].TagPrefix == nil || *cfg.Modules[1].TagPrefix != "reviewer-" {
		t.Errorf("Modules[1].TagPrefix = %v, want pointer to %q", cfg.Modules[1].TagPrefix, "reviewer-")
	}
	if cfg.Modules[1].Prefix() != "reviewer-" {
		t.Errorf("Modules[1].Prefix() = %q, want %q", cfg.Modules[1].Prefix(), "reviewer-")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yml")
	cfg, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig(...) on a missing file succeeded unexpectedly; expected an error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("LoadConfig(...) error = %v, want errors.Is(..., fs.ErrNotExist) so cmd/auto-tag can apply DefaultConfig", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("LoadConfig(...) error = %q, want the missing path included", err.Error())
	}
	if len(cfg.Modules) != 0 {
		t.Errorf("LoadConfig(...) cfg.Modules = %+v, want a zero Config on error", cfg.Modules)
	}
}

func TestLoadConfig_MissingFile_DoesNotReturnDefaultConfig(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err == nil {
		t.Fatal("LoadConfig(...) on a missing file succeeded unexpectedly")
	}
	def := DefaultConfig()
	if len(cfg.Modules) == len(def.Modules) && len(cfg.Modules) > 0 && cfg.Modules[0].Dir == def.Modules[0].Dir {
		t.Error("LoadConfig(...) on a missing file must not populate DefaultConfig; the caller decides that fallback")
	}
}

func TestLoadConfig_PathIsDirectory_IsNotErrNotExist(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err == nil {
		t.Fatal("LoadConfig(...) on a directory succeeded unexpectedly; expected an error")
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("LoadConfig(...) on a directory error = %v, must not satisfy errors.Is(..., fs.ErrNotExist)", err)
	}
	if len(cfg.Modules) != 0 {
		t.Errorf("LoadConfig(...) cfg.Modules = %+v, want a zero Config on error", cfg.Modules)
	}
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	if _, err := LoadConfig(""); err == nil {
		t.Fatal("LoadConfig(\"\") succeeded unexpectedly; expected an error")
	}
}

func TestLoadConfig_NoModules(t *testing.T) {
	path := writeConfig(t, "modules: []\n")
	cfg, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig(...) with zero modules succeeded unexpectedly; expected an error")
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("LoadConfig(...) empty modules error = %v, must not look like a missing file", err)
	}
	if !strings.Contains(err.Error(), "declares no modules") {
		t.Errorf("LoadConfig(...) error = %q, want %q", err.Error(), "declares no modules")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("LoadConfig(...) error = %q, want the config path included", err.Error())
	}
	if len(cfg.Modules) != 0 {
		t.Errorf("LoadConfig(...) cfg.Modules = %+v, want a zero Config on error", cfg.Modules)
	}
}

func TestLoadConfig_ModuleMissingDir(t *testing.T) {
	path := writeConfig(t, "modules:\n  - name: reviewer\n")
	cfg, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig(...) with a module missing dir succeeded unexpectedly; expected an error")
	}
	if !strings.Contains(err.Error(), "module at index 0 has an empty dir") {
		t.Errorf("LoadConfig(...) error = %q, want index 0 empty dir", err.Error())
	}
	if len(cfg.Modules) != 0 {
		t.Errorf("LoadConfig(...) cfg.Modules = %+v, want a zero Config on error", cfg.Modules)
	}
}

func TestModule_Prefix(t *testing.T) {
	explicitEmpty := ""
	explicitCustom := "gopls/v"

	cases := []struct {
		name   string
		module Module
		want   string
	}{
		{"empty name defaults to no prefix", Module{Name: "", Dir: "."}, ""},
		{"non-empty name defaults to hyphenated prefix", Module{Name: "reviewer", Dir: "tools/ci"}, "reviewer-"},
		{"explicit override wins over name default", Module{Name: "gopls", Dir: "gopls", TagPrefix: &explicitCustom}, "gopls/v"},
		{"explicit empty override suppresses the hyphenated default", Module{Name: "reviewer", Dir: "tools/ci", TagPrefix: &explicitEmpty}, ""},
		{"explicit override of empty name still wins", Module{Name: "", Dir: ".", TagPrefix: &explicitCustom}, "gopls/v"},
		{"name without hyphen still receives a trailing hyphen", Module{Name: "v", Dir: "."}, "v-"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.module.Prefix(); got != c.want {
				t.Errorf("Prefix() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestLatestTag_TagSelection covers LatestTag's prefix matching, semver parsing, and version
// comparison rules against a single-commit repository carrying the listed tags. Cases needing a
// distinct commit topology (TestLatestTag_OrdersByVersionNotCommitAncestry) are kept separate.
func TestLatestTag_TagSelection(t *testing.T) {
	cases := []struct {
		name        string
		tags        []string
		annotated   bool
		prefix      string
		wantTag     string
		wantVersion string
	}{
		{name: "no tags returns a synthetic zero version", prefix: "reviewer-", wantTag: "reviewer-0.0.0", wantVersion: "0.0.0"},
		{name: "picks the max version among matching prefix", tags: []string{"reviewer-1.0.0", "reviewer-1.2.0", "reviewer-1.1.5", "reviewer-v1.3.0", "other-9.9.9", "reviewer-not-a-version"}, prefix: "reviewer-", wantTag: "reviewer-v1.3.0", wantVersion: "1.3.0"},
		{name: "empty prefix matches every semver tag", tags: []string{"v1.0.0", "v1.2.0", "v1.1.5", "not-semver"}, wantTag: "v1.2.0", wantVersion: "1.2.0"},
		{name: "compares numerically across two-digit segments", tags: []string{"reviewer-0.9.9", "reviewer-0.10.0", "reviewer-1.2.9", "reviewer-1.2.10", "reviewer-1.9.9", "reviewer-2.0.0"}, prefix: "reviewer-", wantTag: "reviewer-2.0.0", wantVersion: "2.0.0"},
		{name: "strips a single leading v or V", tags: []string{"reviewer-V2.1.0"}, prefix: "reviewer-", wantTag: "reviewer-V2.1.0", wantVersion: "2.1.0"},
		{name: "ignores tags that do not parse as MAJOR.MINOR.PATCH", tags: []string{"reviewer-1.2", "reviewer-1.2.3.4", "reviewer-invalid", "reviewer-1.0.0"}, prefix: "reviewer-", wantTag: "reviewer-1.0.0", wantVersion: "1.0.0"},
		{name: "a prefix does not match a longer sibling prefix", tags: []string{"app-1.0.0", "apple-2.0.0", "application-3.0.0"}, prefix: "app-", wantTag: "app-1.0.0", wantVersion: "1.0.0"},
		{name: "no matching prefix returns a synthetic zero version", tags: []string{"other-1.2.3"}, prefix: "reviewer-", wantTag: "reviewer-0.0.0", wantVersion: "0.0.0"},
		{name: "an existing 0.0.0 tag is returned as the real tag name", tags: []string{"reviewer-0.0.0"}, prefix: "reviewer-", wantTag: "reviewer-0.0.0", wantVersion: "0.0.0"},
		{name: "a tag equal to the prefix alone is not semver and is ignored", tags: []string{"reviewer-", "reviewer-0.1.0"}, prefix: "reviewer-", wantTag: "reviewer-0.1.0", wantVersion: "0.1.0"},
		{name: "prefix matching is case sensitive", tags: []string{"Reviewer-9.9.9", "reviewer-1.0.0"}, prefix: "reviewer-", wantTag: "reviewer-1.0.0", wantVersion: "1.0.0"},
		{name: "a prerelease suffix is not MAJOR.MINOR.PATCH and is ignored", tags: []string{"reviewer-2.0.0-rc.1", "reviewer-1.0.0"}, prefix: "reviewer-", wantTag: "reviewer-1.0.0", wantVersion: "1.0.0"},
		{name: "build metadata is not MAJOR.MINOR.PATCH and is ignored", tags: []string{"reviewer-2.0.0+build", "reviewer-1.0.0"}, prefix: "reviewer-", wantTag: "reviewer-1.0.0", wantVersion: "1.0.0"},
		{name: "only one leading v is stripped, a second v breaks parsing", tags: []string{"reviewer-vv1.0.0", "reviewer-0.2.0"}, prefix: "reviewer-", wantTag: "reviewer-0.2.0", wantVersion: "0.2.0"},
		{name: "patch segments compare numerically, not lexicographically", tags: []string{"reviewer-1.0.9", "reviewer-1.0.10"}, prefix: "reviewer-", wantTag: "reviewer-1.0.10", wantVersion: "1.0.10"},
		{name: "annotated tags are selected the same as lightweight tags", tags: []string{"reviewer-3.1.4"}, annotated: true, prefix: "reviewer-", wantTag: "reviewer-3.1.4", wantVersion: "3.1.4"},
		{name: "an empty prefix still requires HasPrefix, not equality", tags: []string{"0.0.1", "0.0.2"}, wantTag: "0.0.2", wantVersion: "0.0.2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo, sha := newRepoWithCommit(t)
			createLatestTagFixtures(t, repo, sha, c.tags, c.annotated)

			tag, version, err := LatestTag(repo, c.prefix)
			if err != nil {
				t.Fatalf("LatestTag(...) error = %v", err)
			}
			if tag != c.wantTag || version != c.wantVersion {
				t.Errorf("LatestTag(...) = (%q, %q), want (%q, %q)", tag, version, c.wantTag, c.wantVersion)
			}
		})
	}
}

func TestDetectDirectoryTreeChanges(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/file.txt": "a1",
		"dirB/file.txt": "b1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"dirA/file.txt": "a2",
		"dirB/file.txt": "b1",
	})

	changedA, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "dirA")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(..., \"dirA\") returned an unexpected error: %v", err)
	}
	if !changedA {
		t.Error("DetectDirectoryTreeChanges(..., \"dirA\") = false, want true")
	}

	changedB, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "dirB")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(..., \"dirB\") returned an unexpected error: %v", err)
	}
	if changedB {
		t.Error("DetectDirectoryTreeChanges(..., \"dirB\") = true, want false")
	}

	changedRoot, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, ".")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(..., \".\") returned an unexpected error: %v", err)
	}
	if !changedRoot {
		t.Error("DetectDirectoryTreeChanges(..., \".\") = false, want true")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	path := writeConfig(t, "modules: [unclosed_bracket")
	if _, err := LoadConfig(path); err == nil {
		t.Error("LoadConfig(...) with invalid YAML succeeded unexpectedly; expected an error")
	}
}

func TestDetectDirectoryTreeChanges_InvalidSHA(t *testing.T) {
	repo, validSHA := newRepoWithCommit(t)
	invalidSHA := "0000000000000000000000000000000000000000"

	if _, err := DetectDirectoryTreeChanges(repo, invalidSHA, validSHA, "."); err == nil {
		t.Error("DetectDirectoryTreeChanges(...) with invalid parentSHA succeeded unexpectedly; expected an error")
	}
	if _, err := DetectDirectoryTreeChanges(repo, validSHA, invalidSHA, "."); err == nil {
		t.Error("DetectDirectoryTreeChanges(...) with invalid target SHA succeeded unexpectedly; expected an error")
	}
}

func TestDetectDirectoryTreeChanges_SubdirectoryPath(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/sub/file.txt":   "1",
		"dirA-other/file.txt": "1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"dirA/sub/file.txt":   "2",
		"dirA-other/file.txt": "1",
	})

	changedSub, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "dirA/sub")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(..., \"dirA/sub\") returned an unexpected error: %v", err)
	}
	if !changedSub {
		t.Error("DetectDirectoryTreeChanges(..., \"dirA/sub\") = false, want true")
	}

	changedOther, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "dirA-other")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(..., \"dirA-other\") returned an unexpected error: %v", err)
	}
	if changedOther {
		t.Error("DetectDirectoryTreeChanges(..., \"dirA-other\") = true, want false")
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	path := writeConfig(t, "")
	if _, err := LoadConfig(path); err == nil {
		t.Error("LoadConfig(...) with empty file content succeeded unexpectedly; expected an error")
	}
}

func TestLoadConfig_NilModules(t *testing.T) {
	path := writeConfig(t, "other_key: value")
	if _, err := LoadConfig(path); err == nil {
		t.Error("LoadConfig(...) missing modules key succeeded unexpectedly; expected an error")
	}
}

func TestDetectDirectoryTreeChanges_SameCommit(t *testing.T) {
	repo, sha := newRepoWithCommit(t)
	changed, err := DetectDirectoryTreeChanges(repo, sha, sha, ".")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(...) with identical commits returned an unexpected error: %v", err)
	}
	if changed {
		t.Error("DetectDirectoryTreeChanges(...) with identical commits = true, want false")
	}
}

func TestDetectDirectoryTreeChanges_TrailingSlashAndEmptyDir(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/file.txt": "a1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"dirA/file.txt": "a2",
	})

	changedTrailing, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "dirA/")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(..., \"dirA/\") returned an unexpected error: %v", err)
	}
	if !changedTrailing {
		t.Error("DetectDirectoryTreeChanges(..., \"dirA/\") = false, want true")
	}

	changedEmpty, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(..., \"\") returned an unexpected error: %v", err)
	}
	if !changedEmpty {
		t.Error("DetectDirectoryTreeChanges(..., \"\") = false, want true")
	}
}

func TestDetectDirectoryTreeChanges_FileDeletion(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/file.txt": "content",
	})

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}
	if _, err := worktree.Remove("dirA/file.txt"); err != nil {
		t.Fatalf("Remove() failed: %v", err)
	}
	secondSHA, err := worktree.Commit("delete file", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	changed, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA.String(), "dirA")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(...) on file deletion returned an unexpected error: %v", err)
	}
	if !changed {
		t.Error("DetectDirectoryTreeChanges(...) on file deletion = false, want true")
	}
}

func TestLoadConfig_ExplicitEmptyTagPrefix(t *testing.T) {
	path := writeConfig(t, `
modules:
  - name: reviewer
    dir: tools/ci
    tag_prefix: ""
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig(...) error = %v", err)
	}
	if cfg.Modules[0].TagPrefix == nil {
		t.Fatal("TagPrefix = nil, want a pointer to empty string")
	}
	if *cfg.Modules[0].TagPrefix != "" {
		t.Errorf("TagPrefix = %q, want empty", *cfg.Modules[0].TagPrefix)
	}
	if got := cfg.Modules[0].Prefix(); got != "" {
		t.Errorf("Prefix() = %q, want empty", got)
	}
}

func TestLoadConfig_SecondModuleEmptyDirReportsIndex1(t *testing.T) {
	path := writeConfig(t, `
modules:
  - name: ok
    dir: tools
  - name: broken
    dir: ""
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig(...) succeeded; expected an empty dir error at index 1")
	}
	if !strings.Contains(err.Error(), "module at index 1 has an empty dir") {
		t.Errorf("LoadConfig(...) error = %q, want index 1", err.Error())
	}
}

func TestLoadConfig_WhitespaceDirAccepted(t *testing.T) {
	path := writeConfig(t, `
modules:
  - name: odd
    dir: " "
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig(...) error = %v; whitespace-only dir is non-empty and must pass validation", err)
	}
	if cfg.Modules[0].Dir != " " {
		t.Errorf("Dir = %q, want a single space", cfg.Modules[0].Dir)
	}
}

func TestLoadConfig_UnknownKeysIgnored(t *testing.T) {
	path := writeConfig(t, `
modules:
  - name: reviewer
    dir: tools/ci
    extra: ignored
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig(...) error = %v", err)
	}
	if cfg.Modules[0].Dir != "tools/ci" {
		t.Errorf("Dir = %q, want tools/ci", cfg.Modules[0].Dir)
	}
}

func TestLoadConfig_DuplicateNamesAllowed(t *testing.T) {
	path := writeConfig(t, `
modules:
  - name: dup
    dir: a
  - name: dup
    dir: b
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig(...) error = %v; duplicate names have no uniqueness check", err)
	}
	if len(cfg.Modules) != 2 {
		t.Fatalf("len(Modules) = %d, want 2", len(cfg.Modules))
	}
}

func TestLoadConfig_InvalidYAML_WrapsParseError(t *testing.T) {
	path := writeConfig(t, "modules: [unclosed_bracket")
	cfg, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig(...) with invalid YAML succeeded unexpectedly")
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("parse error = %v, must not satisfy errors.Is(..., fs.ErrNotExist)", err)
	}
	if !strings.Contains(err.Error(), "failed to parse versioning config") {
		t.Errorf("error = %q, want a parse wrapper", err.Error())
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the config path included", err.Error())
	}
	if len(cfg.Modules) != 0 {
		t.Errorf("cfg.Modules = %+v, want a zero Config on error", cfg.Modules)
	}
}

func TestLoadConfig_YAMLNullModules(t *testing.T) {
	path := writeConfig(t, "modules: null\n")
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig(...) with null modules succeeded; expected an error")
	}
}

func TestLoadConfig_DocumentPreamble(t *testing.T) {
	path := writeConfig(t, `
---
modules:
  - name: root
    dir: "."
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig(...) error = %v", err)
	}
	if len(cfg.Modules) != 1 || cfg.Modules[0].Dir != "." {
		t.Errorf("cfg = %+v, want one module with Dir %q", cfg, ".")
	}
}

func TestLoadConfig_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permission bits")
	}
	path := writeConfig(t, `
modules:
  - name: ""
    dir: "."
`)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("Chmod(0) failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig(...) succeeded on an unreadable file")
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("permission error = %v, must not satisfy errors.Is(..., fs.ErrNotExist)", err)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("error = %v, want errors.Is(..., fs.ErrPermission)", err)
	}
}

func TestLatestTag_OrdersByVersionNotCommitAncestry(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{"README.md": "v1\n"})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{"README.md": "v2\n"})
	mustTag(t, repo, "reviewer-2.0.0", firstSHA)
	mustTag(t, repo, "reviewer-1.0.0", secondSHA)

	tag, version, err := LatestTag(repo, "reviewer-")
	if err != nil {
		t.Fatalf("LatestTag(...) error = %v", err)
	}
	if tag != "reviewer-2.0.0" || version != "2.0.0" {
		t.Errorf("LatestTag(...) = (%q, %q), want 2.0.0 even though that tag points at the older commit", tag, version)
	}
}

func TestDetectDirectoryTreeChanges_FileAddition(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/keep.txt": "1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"dirA/new.txt": "2",
	})

	changed, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "dirA")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(...) error = %v", err)
	}
	if !changed {
		t.Error("DetectDirectoryTreeChanges(...) = false after adding a file under dirA, want true")
	}
}

func TestDetectDirectoryTreeChanges_RenameWithinDirectory(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/old.txt": "same",
	})
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}
	if _, err := wt.Move("dirA/old.txt", "dirA/new.txt"); err != nil {
		t.Fatalf("Move() failed: %v", err)
	}
	secondSHA, err := wt.Commit("rename", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	changed, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA.String(), "dirA")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(...) error = %v", err)
	}
	if !changed {
		t.Error("DetectDirectoryTreeChanges(...) = false after a rename under dirA, want true")
	}
}

func TestDetectDirectoryTreeChanges_RenameOutOfDirectory(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/file.txt": "x",
		"dirB/keep.txt": "y",
	})
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() failed: %v", err)
	}
	if _, err := wt.Move("dirA/file.txt", "dirB/file.txt"); err != nil {
		t.Fatalf("Move() failed: %v", err)
	}
	secondSHA, err := wt.Commit("move across modules", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	changedA, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA.String(), "dirA")
	if err != nil {
		t.Fatalf("dirA: %v", err)
	}
	if changedA {
		t.Error("dirA reported a change; DetectDirectoryTreeChanges prefers change.To.Name and therefore misses the source path of a rename")
	}
	changedB, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA.String(), "dirB")
	if err != nil {
		t.Fatalf("dirB: %v", err)
	}
	if !changedB {
		t.Error("dirB must report a change when a file is moved in")
	}
}

func TestDetectDirectoryTreeChanges_FileNamedExactlyDir(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"mod": "v1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"mod": "v2",
	})

	changed, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "mod")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(...) error = %v", err)
	}
	if !changed {
		t.Error("DetectDirectoryTreeChanges(..., \"mod\") = false for a file named mod, want true")
	}
}

func TestDetectDirectoryTreeChanges_PrefixDoesNotMatchLongerSiblingName(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"pkg/file.txt":     "1",
		"pkgutil/file.txt": "1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"pkgutil/file.txt": "2",
	})

	changed, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "pkg")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(...) error = %v", err)
	}
	if changed {
		t.Error("DetectDirectoryTreeChanges(..., \"pkg\") = true for a change in pkgutil, want false")
	}
}

func TestDetectDirectoryTreeChanges_DoubleTrailingSlashDoesNotMatch(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/file.txt": "1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"dirA/file.txt": "2",
	})

	changed, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "dirA//")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(...) error = %v", err)
	}
	if changed {
		t.Error("DetectDirectoryTreeChanges(..., \"dirA//\") = true; TrimSuffix removes only one slash so this prefix must not match")
	}
}

func TestDetectDirectoryTreeChanges_RootFileDoesNotMatchNestedDir(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"README.md":     "1",
		"dirA/file.txt": "1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"README.md": "2",
	})

	changed, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "dirA")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(...) error = %v", err)
	}
	if changed {
		t.Error("DetectDirectoryTreeChanges(..., \"dirA\") = true for a root README change, want false")
	}

	changedRoot, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, ".")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(..., \".\") error = %v", err)
	}
	if !changedRoot {
		t.Error("DetectDirectoryTreeChanges(..., \".\") = false for a root README change, want true")
	}
}

func TestDetectDirectoryTreeChanges_InvalidSHAIncludesQuotedHash(t *testing.T) {
	repo, validSHA := newRepoWithCommit(t)
	invalidSHA := "0000000000000000000000000000000000000000"

	_, err := DetectDirectoryTreeChanges(repo, invalidSHA, validSHA, ".")
	if err == nil {
		t.Fatal("expected an error for an invalid parent SHA")
	}
	if !strings.Contains(err.Error(), invalidSHA) {
		t.Errorf("error = %q, want the invalid parent SHA included", err.Error())
	}

	_, err = DetectDirectoryTreeChanges(repo, validSHA, invalidSHA, ".")
	if err == nil {
		t.Fatal("expected an error for an invalid target SHA")
	}
	if !strings.Contains(err.Error(), invalidSHA) {
		t.Errorf("error = %q, want the invalid target SHA included", err.Error())
	}
}

func TestDetectDirectoryTreeChanges_EmptySHA(t *testing.T) {
	repo, validSHA := newRepoWithCommit(t)
	if _, err := DetectDirectoryTreeChanges(repo, "", validSHA, "."); err == nil {
		t.Error("DetectDirectoryTreeChanges(...) with empty parentSHA succeeded; expected an error")
	}
}

func TestDetectDirectoryTreeChanges_NestedDirChangeDoesNotMatchChildOnlyQuery(t *testing.T) {
	repo, firstSHA := newRepoWithFiles(t, map[string]string{
		"dirA/sub/file.txt":   "1",
		"dirA/other/file.txt": "1",
	})
	secondSHA := addCommitWithFiles(t, repo, map[string]string{
		"dirA/other/file.txt": "2",
	})

	changedSub, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "dirA/sub")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(..., \"dirA/sub\") error = %v", err)
	}
	if changedSub {
		t.Error("DetectDirectoryTreeChanges(..., \"dirA/sub\") = true for a sibling subtree change, want false")
	}

	changedParent, err := DetectDirectoryTreeChanges(repo, firstSHA, secondSHA, "dirA")
	if err != nil {
		t.Fatalf("DetectDirectoryTreeChanges(..., \"dirA\") error = %v", err)
	}
	if !changedParent {
		t.Error("DetectDirectoryTreeChanges(..., \"dirA\") = false for a nested change, want true")
	}
}

func TestPathContainedInDirectory(t *testing.T) {
	cases := []struct {
		path string
		dir  string
		want bool
	}{
		{"README.md", ".", true},
		{"README.md", "", true},
		{"dirA/file.txt", ".", true},
		{"dirA/file.txt", "dirA", true},
		{"dirA", "dirA", true},
		{"dirA/file.txt", "dirA/", false},
		{"dirAB/file.txt", "dirA", false},
		{"dirA/file.txt", "dirB", false},
		{"dirA/sub/file.txt", "dirA/sub", true},
		{"dirA/sub2/file.txt", "dirA/sub", false},
		{"", ".", true},
	}
	for _, c := range cases {
		t.Run(c.path+"|"+c.dir, func(t *testing.T) {
			if got := pathContainedInDirectory(c.path, c.dir); got != c.want {
				t.Errorf("pathContainedInDirectory(%q, %q) = %v, want %v", c.path, c.dir, got, c.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		name                   string
		aMajor, aMinor, aPatch int
		bMajor, bMinor, bPatch int
		wantSign               int
	}{
		{"major greater", 2, 0, 0, 1, 99, 99, 1},
		{"major less", 1, 99, 99, 2, 0, 0, -1},
		{"minor greater", 1, 10, 0, 1, 9, 9, 1},
		{"patch greater", 1, 0, 10, 1, 0, 9, 1},
		{"equal", 1, 2, 3, 1, 2, 3, 0},
		{"zero vs sentinel", 0, 0, 0, -1, -1, -1, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := compareVersions(c.aMajor, c.aMinor, c.aPatch, c.bMajor, c.bMinor, c.bPatch)
			if sign(got) != c.wantSign {
				t.Errorf("compareVersions(...) = %d, want sign %d", got, c.wantSign)
			}
		})
	}
}

func sign(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}
	return 0
}
