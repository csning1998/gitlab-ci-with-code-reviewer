package config

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// clearRequiredEnv unsets every environment variable LoadEnvFile reads, then t.Setenv restores
// the caller's overrides, isolating each test from whatever CI or shell environment runs it in.
func clearRequiredEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"CI_API_V4_URL", "CI_PROJECT_ID", "CI_MERGE_REQUEST_IID",
		"CLAUDE_MR_REVIEWER", "GEMINI_MR_REVIEWER",
		"GEMINI_MODEL", "GEMINI_API_KEY", "CLAUDE_MODEL", "CLAUDE_API_KEY",
		"CLAUDE_MAX_TOKENS", "CLAUDE_TIMEOUT_MINUTES",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("CI_API_V4_URL", "https://gitlab.example.com/api/v4")
	t.Setenv("CI_PROJECT_ID", "1")
	t.Setenv("CI_MERGE_REQUEST_IID", "2")
}

func TestLoadEnvFile_AllFieldsPopulated(t *testing.T) {
	clearRequiredEnv(t)
	t.Setenv("CLAUDE_MR_REVIEWER", "claude-token")
	t.Setenv("GEMINI_MR_REVIEWER", "gemini-token")
	t.Setenv("GEMINI_MODEL", "gemini-custom")
	t.Setenv("GEMINI_API_KEY", "gkey")
	t.Setenv("CLAUDE_MODEL", "claude-custom")
	t.Setenv("CLAUDE_API_KEY", "ckey")
	t.Setenv("CLAUDE_MAX_TOKENS", "8192")
	t.Setenv("CLAUDE_TIMEOUT_MINUTES", "15")

	cfg := LoadEnvFile()
	if cfg.APIURL != "https://gitlab.example.com/api/v4" || cfg.ProjectID != "1" || cfg.MRIID != "2" {
		t.Errorf("LoadEnvFile() = %+v, want the required CI fields populated verbatim", cfg)
	}
	if cfg.ClaudeToken != "claude-token" || cfg.GeminiToken != "gemini-token" {
		t.Errorf("LoadEnvFile() = %+v, want both provider tokens populated", cfg)
	}
	if cfg.GeminiModel != "gemini-custom" || cfg.ClaudeModel != "claude-custom" {
		t.Errorf("LoadEnvFile() = %+v, want the model overrides honored", cfg)
	}
	if cfg.ClaudeMaxTokens != 8192 {
		t.Errorf("LoadEnvFile().ClaudeMaxTokens = %d, want 8192", cfg.ClaudeMaxTokens)
	}
	if cfg.ClaudeTimeoutMinutes != 15 {
		t.Errorf("LoadEnvFile().ClaudeTimeoutMinutes = %d, want 15", cfg.ClaudeTimeoutMinutes)
	}
}

func TestLoadEnvFile_DefaultsForOptionalFields(t *testing.T) {
	clearRequiredEnv(t)
	cfg := LoadEnvFile()
	if cfg.GeminiModel != defaultGeminiModel {
		t.Errorf("LoadEnvFile().GeminiModel = %q, want the default %q", cfg.GeminiModel, defaultGeminiModel)
	}
	if cfg.ClaudeModel != defaultClaudeModel {
		t.Errorf("LoadEnvFile().ClaudeModel = %q, want the default %q", cfg.ClaudeModel, defaultClaudeModel)
	}
	if cfg.ClaudeMaxTokens != defaultClaudeMaxTokens {
		t.Errorf("LoadEnvFile().ClaudeMaxTokens = %d, want the default %d", cfg.ClaudeMaxTokens, defaultClaudeMaxTokens)
	}
	if cfg.ClaudeTimeoutMinutes != defaultClaudeTimeoutMinutes {
		t.Errorf("LoadEnvFile().ClaudeTimeoutMinutes = %d, want the default %d", cfg.ClaudeTimeoutMinutes, defaultClaudeTimeoutMinutes)
	}
	if cfg.GeminiKey != "" || cfg.ClaudeKey != "" {
		t.Errorf("LoadEnvFile() = %+v, want empty provider API keys when unset", cfg)
	}
}

func TestLoadEnvFile_GitLabTokenPrefersClaudeOverGemini(t *testing.T) {
	clearRequiredEnv(t)
	t.Setenv("CLAUDE_MR_REVIEWER", "claude-token")
	t.Setenv("GEMINI_MR_REVIEWER", "gemini-token")

	cfg := LoadEnvFile()
	if cfg.GitLabToken != "claude-token" {
		t.Errorf("LoadEnvFile().GitLabToken = %q, want it to prefer the Claude token when both are set", cfg.GitLabToken)
	}
}

func TestLoadEnvFile_GitLabTokenFallsBackToGemini(t *testing.T) {
	clearRequiredEnv(t)
	t.Setenv("GEMINI_MR_REVIEWER", "gemini-token")

	cfg := LoadEnvFile()
	if cfg.GitLabToken != "gemini-token" {
		t.Errorf("LoadEnvFile().GitLabToken = %q, want it to fall back to the Gemini token", cfg.GitLabToken)
	}
}

func TestLoadEnvFile_GitLabTokenEmptyWhenNeitherProviderSet(t *testing.T) {
	clearRequiredEnv(t)
	cfg := LoadEnvFile()
	if cfg.GitLabToken != "" {
		t.Errorf("LoadEnvFile().GitLabToken = %q, want empty when neither provider token is set", cfg.GitLabToken)
	}
}

func TestLoadEnvFile_WhitespaceOnlyRequiredVar_TreatedAsMissing(t *testing.T) {
	// requireEnvVar trims before checking emptiness, so a value of pure whitespace must be
	// treated identically to an unset variable rather than accepted verbatim.
	clearRequiredEnv(t)
	t.Setenv("CLAUDE_MR_REVIEWER", "   ")
	cfg := LoadEnvFile()
	if cfg.ClaudeToken != "" {
		t.Errorf("LoadEnvFile().ClaudeToken = %q, want empty for a whitespace-only value", cfg.ClaudeToken)
	}
}

func TestLoadEnvFile_WhitespacePaddedValue_Trimmed(t *testing.T) {
	clearRequiredEnv(t)
	t.Setenv("CLAUDE_MR_REVIEWER", "  padded-token  ")
	cfg := LoadEnvFile()
	if cfg.ClaudeToken != "padded-token" {
		t.Errorf("LoadEnvFile().ClaudeToken = %q, want the value trimmed of surrounding whitespace", cfg.ClaudeToken)
	}
}

// executeConfigSubprocess re-executes the current test binary as a child process with the given
// extra environment variables, since requireEnvVar and lookupEnvInt call os.Exit on failure and
// would otherwise terminate the parent test runner.
func executeConfigSubprocess(t *testing.T, extraEnv ...string) (exitCode int, stdout string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestLoadEnvFileSubprocessHelper")
	cmd.Env = append(os.Environ(), append([]string{"BE_CONFIG_LOAD=1"}, extraEnv...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err == nil {
		return 0, out.String()
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("subprocess failed for a reason other than a nonzero exit: %v", err)
	}
	return exitErr.ExitCode(), out.String()
}

// TestLoadEnvFileSubprocessHelper is not a real test; it is re-executed in isolation by
// executeConfigSubprocess to observe LoadEnvFile()'s os.Exit(1) behavior on missing/invalid configuration.
func TestLoadEnvFileSubprocessHelper(t *testing.T) {
	if os.Getenv("BE_CONFIG_LOAD") != "1" {
		t.Skip("only runs as a re-executed subprocess")
	}
	LoadEnvFile()
}

func TestLoadEnvFile_MissingAPIURL_ExitsNonzero(t *testing.T) {
	code, stdout := executeConfigSubprocess(t, "CI_API_V4_URL=", "CI_PROJECT_ID=1", "CI_MERGE_REQUEST_IID=2")
	if code != 1 {
		t.Errorf("subprocess exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "CI_API_V4_URL") {
		t.Errorf("subprocess output = %q, want it to name the missing variable", stdout)
	}
}

func TestLoadEnvFile_MissingProjectID_ExitsNonzero(t *testing.T) {
	code, stdout := executeConfigSubprocess(t, "CI_API_V4_URL=https://gitlab.example.com/api/v4", "CI_PROJECT_ID=", "CI_MERGE_REQUEST_IID=2")
	if code != 1 {
		t.Errorf("subprocess exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "CI_PROJECT_ID") {
		t.Errorf("subprocess output = %q, want it to name the missing variable", stdout)
	}
}

func TestLoadEnvFile_MissingMRIID_ExitsNonzero(t *testing.T) {
	code, stdout := executeConfigSubprocess(t, "CI_API_V4_URL=https://gitlab.example.com/api/v4", "CI_PROJECT_ID=1", "CI_MERGE_REQUEST_IID=")
	if code != 1 {
		t.Errorf("subprocess exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "CI_MERGE_REQUEST_IID") {
		t.Errorf("subprocess output = %q, want it to name the missing variable", stdout)
	}
}

func TestLoadEnvFile_InvalidClaudeMaxTokens_ExitsNonzero(t *testing.T) {
	code, stdout := executeConfigSubprocess(t,
		"CI_API_V4_URL=https://gitlab.example.com/api/v4", "CI_PROJECT_ID=1", "CI_MERGE_REQUEST_IID=2",
		"CLAUDE_MAX_TOKENS=not-a-number",
	)
	if code != 1 {
		t.Errorf("subprocess exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "CLAUDE_MAX_TOKENS") {
		t.Errorf("subprocess output = %q, want it to name the invalid variable", stdout)
	}
}

func TestLoadEnvFile_InvalidClaudeTimeoutMinutes_ExitsNonzero(t *testing.T) {
	code, stdout := executeConfigSubprocess(t,
		"CI_API_V4_URL=https://gitlab.example.com/api/v4", "CI_PROJECT_ID=1", "CI_MERGE_REQUEST_IID=2",
		"CLAUDE_TIMEOUT_MINUTES=not-a-number",
	)
	if code != 1 {
		t.Errorf("subprocess exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "CLAUDE_TIMEOUT_MINUTES") {
		t.Errorf("subprocess output = %q, want it to name the invalid variable", stdout)
	}
}

func TestLoadEnvFile_AppliesDotEnvWhenProcessEnvEmpty(t *testing.T) {
	clearRequiredEnv(t)
	// Clear the seeded required vars so the file can supply them.
	t.Setenv("CI_API_V4_URL", "")
	t.Setenv("CI_PROJECT_ID", "")
	t.Setenv("CI_MERGE_REQUEST_IID", "")
	t.Setenv("CLAUDE_MR_REVIEWER", "")

	dir := t.TempDir()
	t.Chdir(dir)
	content := "" +
		"# comment\n" +
		"export CI_API_V4_URL=https://from-file.example/api/v4\n" +
		"CI_PROJECT_ID=\"42\"\n" +
		"CI_MERGE_REQUEST_IID='7'\n" +
		"CLAUDE_MR_REVIEWER=file-token\n"
	if err := os.WriteFile(".env", []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg := LoadEnvFile()
	if cfg.APIURL != "https://from-file.example/api/v4" {
		t.Errorf("APIURL = %q, want value from .env", cfg.APIURL)
	}
	if cfg.ProjectID != "42" || cfg.MRIID != "7" {
		t.Errorf("ProjectID/MRIID = %q/%q, want 42/7 from .env", cfg.ProjectID, cfg.MRIID)
	}
	if cfg.ClaudeToken != "file-token" {
		t.Errorf("ClaudeToken = %q, want file-token from .env", cfg.ClaudeToken)
	}
}

func TestLoadEnvFile_ProcessEnvOverridesDotEnv(t *testing.T) {
	clearRequiredEnv(t)
	t.Setenv("CLAUDE_MR_REVIEWER", "process-token")

	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(".env", []byte("CLAUDE_MR_REVIEWER=file-token\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg := LoadEnvFile()
	if cfg.ClaudeToken != "process-token" {
		t.Errorf("ClaudeToken = %q, want process environment to override .env", cfg.ClaudeToken)
	}
}

func TestApplyEnvFile_MissingFileIsNoop(t *testing.T) {
	applyEnvFile(t.TempDir() + "/does-not-exist.env")
}

func TestApplyEnvFile_SkipsMalformedAndCommentedLines(t *testing.T) {
	t.Setenv("ONLY_FROM_FILE", "")
	dir := t.TempDir()
	path := dir + "/custom.env"
	content := "" +
		"# ignored\n" +
		"not-a-pair\n" +
		"=novaluekey\n" +
		"ONLY_FROM_FILE=ok\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	applyEnvFile(path)
	if got := os.Getenv("ONLY_FROM_FILE"); got != "ok" {
		t.Errorf("ONLY_FROM_FILE = %q, want ok", got)
	}
}
