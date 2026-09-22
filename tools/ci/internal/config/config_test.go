package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"ci-tools/internal/testutil"
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

func TestResolveModelOptions_UnconfiguredSlot_ReturnsErrSlotNotConfigured(t *testing.T) {
	t.Setenv("CLAUDE_MODEL", "")
	t.Setenv("CLAUDE_MODEL_SECONDARY", "")

	_, err := ResolveModelOptions("claude", "primary")
	if err != ErrSlotNotConfigured {
		t.Errorf("ResolveModelOptions(claude, primary) expected ErrSlotNotConfigured, got %v", err)
	}
}

func TestResolveModelOptions_ValidOptionalParams_ParsedCorrectly(t *testing.T) {
	t.Setenv("CLAUDE_MODEL", "claude-sonnet-4-6")
	t.Setenv("CLAUDE_TEMPERATURE", "0.8")
	t.Setenv("CLAUDE_TOP_P", "0.95")
	t.Setenv("CLAUDE_REASONING_EFFORT", "high")
	t.Setenv("CLAUDE_MAX_TOKENS", "4096")

	opts, err := ResolveModelOptions("claude", "primary")
	if err != nil {
		t.Fatalf("ResolveModelOptions(claude, primary) unexpected error: %v", err)
	}

	if opts.Provider != "claude" {
		t.Errorf("opts.Provider = %q, want %q", opts.Provider, "claude")
	}
	if opts.Model != "claude-sonnet-4-6" {
		t.Errorf("opts.Model = %q, want %q", opts.Model, "claude-sonnet-4-6")
	}
	if opts.Temperature == nil || *opts.Temperature != 0.8 {
		t.Errorf("opts.Temperature = %v, want 0.8", opts.Temperature)
	}
	if opts.TopP == nil || *opts.TopP != 0.95 {
		t.Errorf("opts.TopP = %v, want 0.95", opts.TopP)
	}
	if opts.ReasoningLevel != "high" {
		t.Errorf("opts.ReasoningLevel = %q, want %q", opts.ReasoningLevel, "high")
	}
	if opts.MaxTokens != 4096 {
		t.Errorf("opts.MaxTokens = %d, want 4096", opts.MaxTokens)
	}
}

func TestResolveModelOptions_OutOfRangeValues_Rejected(t *testing.T) {
	t.Setenv("GEMINI_MODEL", "gemini-3.1-pro-preview")
	t.Setenv("GEMINI_TEMPERATURE", "2.5") // exceeds allowed upper bound of 2.0

	_, err := ResolveModelOptions("gemini", "primary")
	if err == nil {
		t.Errorf("ResolveModelOptions(gemini, primary) expected error for temperature 2.5, got nil")
	}
}

func TestResolveModelOptions_SecondarySlot_ParsedIndependently(t *testing.T) {
	t.Setenv("OPENAI_MODEL", "gpt-4o")
	t.Setenv("OPENAI_TEMPERATURE", "0.2")
	t.Setenv("OPENAI_MODEL_SECONDARY", "o3-mini")
	t.Setenv("OPENAI_REASONING_EFFORT_SECONDARY", "high")

	primary, err := ResolveModelOptions("openai", "primary")
	if err != nil {
		t.Fatalf("primary slot resolve failed: %v", err)
	}
	secondary, err := ResolveModelOptions("openai", "secondary")
	if err != nil {
		t.Fatalf("secondary slot resolve failed: %v", err)
	}

	if primary.Model != "gpt-4o" {
		t.Errorf("primary.Model = %q, want gpt-4o", primary.Model)
	}
	if secondary.Model != "o3-mini" {
		t.Errorf("secondary.Model = %q, want o3-mini", secondary.Model)
	}
	if secondary.ReasoningLevel != "high" {
		t.Errorf("secondary.ReasoningLevel = %q, want high", secondary.ReasoningLevel)
	}
}

func TestParseConfigFile_ValidYAML_ParsedSuccessfully(t *testing.T) {
	yamlContent := `
defaults:
  timeout: 3m
  max_tokens: 4096

models:
  claude-code:
    provider: claude
    model: claude-sonnet-4-6
    temperature: 0.8
    reasoning_level: high

  gemini-sec:
    provider: gemini
    model: gemini-3.1-pro-preview
    temperature: 0.5
    thinking_budget: 8192

slots:
  primary: claude-code
  secondary: gemini-sec
`
	tmpFile, err := os.CreateTemp(t.TempDir(), "reviewer-*.yml")
	if err != nil {
		t.Fatalf("failed to create temp yaml: %v", err)
	}
	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		t.Fatalf("failed to write yaml: %v", err)
	}
	_ = tmpFile.Close()

	cfgDecl, err := ParseConfigFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseConfigFile failed: %v", err)
	}

	if cfgDecl.Defaults.MaxTokens != 4096 {
		t.Errorf("cfgDecl.Defaults.MaxTokens = %d, want 4096", cfgDecl.Defaults.MaxTokens)
	}
	claudeModel, ok := cfgDecl.Models["claude-code"]
	if !ok {
		t.Fatalf("model 'claude-code' not found in config")
	}
	if claudeModel.Provider != "claude" || claudeModel.Model != "claude-sonnet-4-6" {
		t.Errorf("claudeModel = %+v, want provider claude and model claude-sonnet-4-6", claudeModel)
	}
	if cfgDecl.Slots["primary"] != "claude-code" {
		t.Errorf("slots[primary] = %q, want claude-code", cfgDecl.Slots["primary"])
	}
}

func TestValidate_IncompatibleProviderFields_ReturnsError(t *testing.T) {
	temp := 0.7
	freqPenalty := 0.5
	opts := ModelOptions{
		Provider:         "claude",
		Model:            "claude-sonnet-4-6",
		Temperature:      &temp,
		FrequencyPenalty: &freqPenalty,
	}

	if err := Validate(opts); err == nil {
		t.Errorf("Validate(opts) expected error for FrequencyPenalty on claude provider, got nil")
	}
}

func TestValidate_PromptAndPromptFileMutualExclusion(t *testing.T) {
	opts := ModelOptions{
		Provider:   "claude",
		Model:      "claude-sonnet-4-6",
		Prompt:     "Inline prompt instructions",
		PromptFile: "/path/to/prompt.md",
	}

	if err := Validate(opts); err == nil {
		t.Errorf("Validate(opts) with both Prompt and PromptFile expected error, got nil")
	}
}

func TestValidate_InvalidEnumFieldsRejected(t *testing.T) {
	tests := []struct {
		name string
		opts ModelOptions
	}{
		{
			name: "invalid reasoning_level",
			opts: ModelOptions{
				Provider:       "openai",
				Model:          "o3-mini",
				ReasoningLevel: "ultra",
			},
		},
		{
			name: "invalid verbosity",
			opts: ModelOptions{
				Provider:  "openai",
				Model:     "gpt-4o",
				Verbosity: "extreme",
			},
		},
		{
			name: "invalid thinking_type",
			opts: ModelOptions{
				Provider:     "gemini",
				Model:        "gemini-3.1-pro-preview",
				ThinkingType: "forced",
			},
		},
		{
			name: "invalid media_resolution",
			opts: ModelOptions{
				Provider:        "gemini",
				Model:           "gemini-3.1-pro-preview",
				MediaResolution: "MEDIA_RESOLUTION_ULTRA",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(tt.opts); err == nil {
				t.Errorf("Validate(%s) expected error, got nil", tt.name)
			}
		})
	}
}

func TestValidate_ValidEnumFieldsAccepted(t *testing.T) {
	opts := ModelOptions{
		Provider:        "gemini",
		Model:           "gemini-3.1-pro-preview",
		ReasoningLevel:  "high",
		Verbosity:       "low",
		ThinkingType:    "adaptive",
		MediaResolution: "MEDIA_RESOLUTION_HIGH",
	}

	if err := Validate(opts); err != nil {
		t.Errorf("Validate(valid enums) unexpected error: %v", err)
	}
}

func TestGenerateJSONSchema_ValidOutput(t *testing.T) {
	schemaData, err := GenerateJSONSchema()
	if err != nil {
		t.Fatalf("GenerateJSONSchema failed: %v", err)
	}
	if len(schemaData) == 0 {
		t.Fatalf("GenerateJSONSchema returned empty data")
	}
}

func TestNormalizeAndValidateModel_UnapprovedModelsRejected(t *testing.T) {
	tests := []struct {
		provider string
		model    string
	}{
		{"claude", "deepseek-r1"},
		{"claude", "qwen-2.5-coder"},
		{"claude", "claude-3-5-sonnet-20241022"},
		{"claude", "claude-3-7-sonnet-20250219"},
		{"claude", "claude-3-opus-20240229"},
		{"claude", "claude-sonnet-4"},
		{"gemini", "deepseek-r1"},
		{"gemini", "qwen-2.5-coder"},
		{"gemini", "gemini-1.5-pro"},
		{"gemini", "gemini-1.5-flash"},
		{"gemini", "gemini-3.8-pro"},
		{"gemini", "gemma-4"},
		{"openai", "deepseek-v3"},
		{"openai", "qwen-max"},
		{"openai", "unknown-model"},
		{"grok", "grok-2"},
		{"grok", "grok-beta"},
		{"grok", "deepseek-r1"},
		{"local", "qwen2.5-coder-32b"},
		{"local", "deepseek-coder-v2"},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"_"+tt.model, func(t *testing.T) {
			_, err := NormalizeModel(tt.provider, tt.model)
			if err == nil {
				t.Errorf("NormalizeModel(%q, %q) expected error for unapproved/retired model, got nil", tt.provider, tt.model)
			}
			opts := ModelOptions{
				Provider: tt.provider,
				Model:    tt.model,
			}
			if err := Validate(opts); err == nil {
				t.Errorf("Validate(%q, %q) expected error for unapproved/retired model, got nil", tt.provider, tt.model)
			}
		})
	}
}

func TestNormalizeAndValidateModel_ValidAliasesNormalized(t *testing.T) {
	tests := []struct {
		provider string
		alias    string
		want     string
	}{
		{"claude", "claude-opus-4.7", "claude-opus-4-7"},
		{"claude", "claude-sonnet-4-5", "claude-sonnet-4-5-20250929"},
		{"claude", "claude-opus-4-5", "claude-opus-4-5-20251101"},
		{"claude", "claude-haiku-4-5", "claude-haiku-4-5-20251001"},
		{"gemini", "gemini-3-flash", "gemini-3-flash-preview"},
		{"gemini", "gemini-3-pro", "gemini-3.1-pro-preview"},
		{"openai", "gpt-5.6", "gpt-5.6-sol"},
		{"openai", "gpt-4.1-2025-04-14", "gpt-4.1"},
		{"openai", "gpt-5.4-mini-2026-03-17", "gpt-5.4-mini"},
		{"openai", "gpt-5.4-nano-2026-03-17", "gpt-5.4-nano"},
		{"openai", "o4-mini-2025-04-16", "o4-mini"},
		{"azure-openai", "gpt-5.6", "gpt-5.6-sol"},
		{"grok", "grok-4.6-latest", "grok-4.6"},
		{"grok", "grok-4.20-multi-agent-0309", "grok-4.20-multi-agent"},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"_"+tt.alias, func(t *testing.T) {
			got, err := NormalizeModel(tt.provider, tt.alias)
			if err != nil {
				t.Fatalf("NormalizeModel(%q, %q) unexpected error: %v", tt.provider, tt.alias, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeModel(%q, %q) = %q, want %q", tt.provider, tt.alias, got, tt.want)
			}
		})
	}
}

func TestNormalizeAndValidateModel_ApprovedModelsAccepted(t *testing.T) {
	approvedModels := map[string][]string{
		"claude": {
			"claude-fable-5.1",
			"claude-fable-5",
			"claude-opus-5",
			"claude-sonnet-5",
			"claude-opus-4-8",
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
			"claude-sonnet-4-5-20250929",
			"claude-opus-4-5-20251101",
			"claude-haiku-4-5-20251001",
		},
		"gemini": {
			"gemini-3.8-flash",
			"gemini-3.7-flash",
			"gemini-3.6-flash",
			"gemini-3.5-flash",
			"gemini-3.5-flash-lite",
			"gemini-3.1-flash-lite",
			"gemini-3.1-pro-preview",
			"gemini-3-flash-preview",
			"gemini-2.5-pro",
			"gemini-2.5-flash",
			"gemini-2.5-flash-lite",
			"gemma-4-31b-it",
			"gemma-4-26b-a4b-it",
		},
		"openai": {
			"gpt-4o",
			"gpt-4o-mini",
			"gpt-4-turbo",
			"gpt-4.1",
			"gpt-4.1-mini",
			"gpt-4.1-nano",
			"gpt-5",
			"gpt-5-mini",
			"gpt-5-nano",
			"gpt-5.4",
			"gpt-5.4-mini",
			"gpt-5.4-nano",
			"gpt-5.4-pro",
			"gpt-5.6-sol",
			"gpt-5.6-terra",
			"gpt-5.6-luna",
			"gpt-6-astra",
			"o1",
			"o1-mini",
			"o1-preview",
			"o3",
			"o3-mini",
			"o3-pro",
			"o4-mini",
		},
		"azure-openai": {
			"gpt-4o",
			"gpt-5",
			"o3",
		},
		"grok": {
			"grok-4.6",
			"grok-4.5",
			"grok-4.20-multi-agent",
		},
		"local": {
			"gemma-2-9b",
			"gemma-2-27b",
			"gemma-4-31b",
			"gemma-4-26b-a4b",
			"llama-3.3-70b",
			"llama-3.1-8b",
			"codellama-70b",
			"mistral-large",
		},
	}

	for provider, models := range approvedModels {
		for _, m := range models {
			t.Run(provider+"_"+m, func(t *testing.T) {
				verifyApprovedModel(t, provider, m)
			})
		}
	}
}

func verifyApprovedModel(t *testing.T, provider, model string) {
	t.Helper()
	if !IsModelSupported(provider, model) {
		t.Errorf("IsModelSupported(%q, %q) = false, want true", provider, model)
	}
	norm, err := NormalizeModel(provider, model)
	if err != nil {
		t.Fatalf("NormalizeModel(%q, %q) unexpected error: %v", provider, model, err)
	}
	if norm != model {
		t.Errorf("NormalizeModel(%q, %q) = %q, want %q", provider, model, norm, model)
	}
	opts := ModelOptions{
		Provider: provider,
		Model:    model,
	}
	if err := Validate(opts); err != nil {
		t.Errorf("Validate(%q, %q) unexpected error: %v", provider, model, err)
	}
}

// TestValidate_TemperatureBoundaries verifies exact floating point limits across providers.
func TestValidate_TemperatureBoundaries(t *testing.T) {
	t.Parallel()

	fPtr := func(v float64) *float64 { return &v }

	cases := []struct {
		name      string
		provider  string
		temp      *float64
		wantError bool
	}{
		{name: "exact minimum zero", provider: "openai", temp: fPtr(0.0), wantError: false},
		{name: "exact maximum two", provider: "openai", temp: fPtr(2.0), wantError: false},
		{name: "negative boundary epsilon", provider: "openai", temp: fPtr(-0.000001), wantError: true},
		{name: "positive boundary epsilon", provider: "openai", temp: fPtr(2.000001), wantError: true},
		{name: "claude exact limit one", provider: "claude", temp: fPtr(1.0), wantError: false},
		{name: "claude exceeding limit epsilon", provider: "claude", temp: fPtr(1.000001), wantError: true},
		{name: "claude zero temperature", provider: "claude", temp: fPtr(0.0), wantError: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			model := "gpt-4o"
			if tc.provider == "claude" {
				model = "claude-sonnet-4-6"
			}
			opts := ModelOptions{
				Provider:    tc.provider,
				Model:       model,
				Temperature: tc.temp,
			}
			err := Validate(opts)
			if (err != nil) != tc.wantError {
				t.Errorf("Validate() for %s error = %v, wantError = %v", tc.name, err, tc.wantError)
			}
		})
	}
}

// TestValidate_TopPBoundaries verifies exact probability boundaries between 0.0 and 1.0.
func TestValidate_TopPBoundaries(t *testing.T) {
	t.Parallel()

	fPtr := func(v float64) *float64 { return &v }

	cases := []struct {
		name      string
		topP      *float64
		wantError bool
	}{
		{name: "exact minimum zero", topP: fPtr(0.0), wantError: false},
		{name: "exact maximum one", topP: fPtr(1.0), wantError: false},
		{name: "below zero epsilon", topP: fPtr(-0.000001), wantError: true},
		{name: "above one epsilon", topP: fPtr(1.000001), wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := ModelOptions{
				Provider: "openai",
				Model:    "gpt-4o",
				TopP:     tc.topP,
			}
			err := Validate(opts)
			if (err != nil) != tc.wantError {
				t.Errorf("Validate() for %s error = %v, wantError = %v", tc.name, err, tc.wantError)
			}
		})
	}
}

// TestValidate_ThinkingBudgetBoundaries verifies non-negative constraints on thinking token quotas.
func TestValidate_ThinkingBudgetBoundaries(t *testing.T) {
	t.Parallel()

	iPtr := func(v int) *int { return &v }

	cases := []struct {
		name      string
		budget    *int
		wantError bool
	}{
		{name: "zero budget valid", budget: iPtr(0), wantError: false},
		{name: "positive budget valid", budget: iPtr(4096), wantError: false},
		{name: "negative budget rejected", budget: iPtr(-1), wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := ModelOptions{
				Provider:       "claude",
				Model:          "claude-sonnet-4-6",
				ThinkingBudget: tc.budget,
			}
			err := Validate(opts)
			if (err != nil) != tc.wantError {
				t.Errorf("Validate() for %s error = %v, wantError = %v", tc.name, err, tc.wantError)
			}
		})
	}
}

// TestValidate_PromptMutualExclusion_DeMorgan verifies the mutual exclusion truth table:
// not(hasPrompt and hasPromptFile) evaluates whether the configuration is valid.
func TestValidate_PromptMutualExclusion_DeMorgan(t *testing.T) {
	t.Parallel()

	truthTable := []struct {
		name       string
		prompt     string
		promptFile string
		hasPrompt  bool
		hasFile    bool
	}{
		{name: "neither set", prompt: "", promptFile: "", hasPrompt: false, hasFile: false},
		{name: "whitespace only treated as unset", prompt: "   \t", promptFile: "\n  ", hasPrompt: false, hasFile: false},
		{name: "prompt only set", prompt: "instructions", promptFile: "", hasPrompt: true, hasFile: false},
		{name: "prompt file only set", prompt: "", promptFile: "/tmp/p.md", hasPrompt: false, hasFile: true},
		{name: "both set concurrently", prompt: "instructions", promptFile: "/tmp/p.md", hasPrompt: true, hasFile: true},
	}

	for _, tt := range truthTable {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := ModelOptions{
				Provider:   "openai",
				Model:      "gpt-4o",
				Prompt:     tt.prompt,
				PromptFile: tt.promptFile,
			}
			err := Validate(opts)
			// De Morgan validation: mutually exclusive iff !hasPrompt || !hasFile.
			expectedValid := !tt.hasPrompt || !tt.hasFile
			gotValid := err == nil
			if gotValid != expectedValid {
				t.Errorf("De Morgan evaluation failed for %s: gotValid=%v, expectedValid=%v, err=%v",
					tt.name, gotValid, expectedValid, err)
			}
		})
	}
}

// TestValidate_EnumCaseSensitivity verifies that enum values require exact casing.
func TestValidate_EnumCaseSensitivity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts ModelOptions
	}{
		{name: "uppercase reasoning_level", opts: ModelOptions{Provider: "openai", Model: "o3-mini", ReasoningLevel: "HIGH"}},
		{name: "titlecase thinking_type", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.1-pro-preview", ThinkingType: "Adaptive"}},
		{name: "uppercase verbosity", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", Verbosity: "LOW"}},
		{name: "lowercase media_resolution", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.1-pro-preview", MediaResolution: "media_resolution_high"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := Validate(tc.opts); err == nil {
				t.Errorf("Validate(%s) expected error due to non-standard casing, got nil", tc.name)
			}
		})
	}
}

// TestGenerateJSONSchema_ConcurrentSafe verifies concurrent schema reflections without data races.
func TestGenerateJSONSchema_ConcurrentSafe(t *testing.T) {
	t.Parallel()

	const workers = 30
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for range workers {
		wg.Go(func() {
			data, err := GenerateJSONSchema()
			if err != nil {
				errCh <- fmt.Errorf("GenerateJSONSchema failed: %w", err)
				return
			}
			var schema map[string]interface{}
			if err := json.Unmarshal(data, &schema); err != nil {
				errCh <- fmt.Errorf("unmarshal generated schema: %w", err)
				return
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent schema failure: %v", err)
	}
}

// TestSupportedModels_ConcurrentSafe verifies concurrent reads against model registries.
func TestSupportedModels_ConcurrentSafe(t *testing.T) {
	t.Parallel()

	providers := []string{"claude", "gemini", "openai", "azure-openai", "grok", "local"}
	const iterations = 50
	var wg sync.WaitGroup
	errCh := make(chan error, len(providers)*iterations)

	for _, p := range providers {
		for i := range iterations {
			wg.Go(func() {
				models := SupportedModels(p)
				if len(models) == 0 {
					errCh <- fmt.Errorf("provider %q iteration %d returned empty slice", p, i)
				}
			})
		}
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent model registry failure: %v", err)
	}
}

// TestNormalizeModel_EdgeCases verifies whitespace trimming and casing around alias mapping.
func TestNormalizeModel_EdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		provider string
		input    string
		want     string
		wantErr  bool
	}{
		{provider: "claude", input: "  claude-sonnet-5  ", want: "claude-sonnet-5", wantErr: false},
		{provider: "gemini", input: "gemini-3-flash", want: "gemini-3-flash-preview", wantErr: false},
		{provider: "gemini", input: "gemini-3-pro", want: "gemini-3.1-pro-preview", wantErr: false},
		{provider: "openai", input: "gpt-5.6", want: "gpt-5.6-sol", wantErr: false},
		{provider: "openai", input: "GPT-4o", want: "", wantErr: true},
		{provider: "azure-openai", input: "gpt-4o", want: "gpt-4o", wantErr: false},
		{provider: "unknown-provider", input: "any-model", want: "", wantErr: true},
		{provider: "claude", input: "", want: "", wantErr: true},
		{provider: "claude", input: "   ", want: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s", tc.provider, tc.input), func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeModel(tc.provider, tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NormalizeModel(%q, %q) error = %v, wantErr = %v", tc.provider, tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("NormalizeModel(%q, %q) = %q, want %q", tc.provider, tc.input, got, tc.want)
			}
		})
	}
}

// TestValidate_MissingModelFieldRejected verifies mandatory validation rules.
func TestValidate_MissingModelFieldRejected(t *testing.T) {
	t.Parallel()

	cases := []string{"", "   ", "\t\n"}
	for _, m := range cases {
		opts := ModelOptions{
			Provider: "claude",
			Model:    m,
		}
		if err := Validate(opts); err == nil {
			t.Errorf("Validate() with empty model %q expected error, got nil", m)
		}
	}
}

// Validate MUST enforce every bound which the generated JSON schema advertises.
func TestValidate_EnforcesSchemaAdvertisedBounds(t *testing.T) {
	tests := []struct {
		name    string
		opts    ModelOptions
		wantErr bool
	}{
		{name: "max tokens one", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.5-flash", MaxTokens: 1}},
		{name: "max tokens negative", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.5-flash", MaxTokens: -1}, wantErr: true},
		{name: "top k one", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.5-flash", TopK: testutil.Ptr(1)}},
		{name: "top k zero", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.5-flash", TopK: testutil.Ptr(0)}, wantErr: true},
		{name: "top k negative", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.5-flash", TopK: testutil.Ptr(-1)}, wantErr: true},
		{name: "n one", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", N: testutil.Ptr(1)}},
		{name: "n zero", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", N: testutil.Ptr(0)}, wantErr: true},
		{name: "frequency penalty lower edge", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", FrequencyPenalty: testutil.Ptr(-2.0)}},
		{name: "frequency penalty upper edge", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", FrequencyPenalty: testutil.Ptr(2.0)}},
		{name: "frequency penalty below range", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", FrequencyPenalty: testutil.Ptr(-2.0001)}, wantErr: true},
		{name: "frequency penalty above range", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", FrequencyPenalty: testutil.Ptr(2.0001)}, wantErr: true},
		{name: "presence penalty below range", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", PresencePenalty: testutil.Ptr(-2.0001)}, wantErr: true},
		{name: "presence penalty above range", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", PresencePenalty: testutil.Ptr(2.0001)}, wantErr: true},
		{name: "repetition penalty zero", opts: ModelOptions{Provider: "local", Model: "llama-3.3-70b", RepetitionPenalty: testutil.Ptr(0.0)}},
		{name: "repetition penalty negative", opts: ModelOptions{Provider: "local", Model: "llama-3.3-70b", RepetitionPenalty: testutil.Ptr(-0.1)}, wantErr: true},
		{name: "repetition penalty huge value accepted", opts: ModelOptions{Provider: "local", Model: "llama-3.3-70b", RepetitionPenalty: testutil.Ptr(1e300)}},
		{name: "min p lower edge", opts: ModelOptions{Provider: "local", Model: "llama-3.3-70b", MinP: testutil.Ptr(0.0)}},
		{name: "min p upper edge", opts: ModelOptions{Provider: "local", Model: "llama-3.3-70b", MinP: testutil.Ptr(1.0)}},
		{name: "min p below range", opts: ModelOptions{Provider: "local", Model: "llama-3.3-70b", MinP: testutil.Ptr(-0.1)}, wantErr: true},
		{name: "min p above range", opts: ModelOptions{Provider: "local", Model: "llama-3.3-70b", MinP: testutil.Ptr(1.1)}, wantErr: true},
		{name: "top logprobs lower edge", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", TopLogprobs: testutil.Ptr(0)}},
		{name: "top logprobs upper edge", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", TopLogprobs: testutil.Ptr(20)}},
		{name: "top logprobs below range", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", TopLogprobs: testutil.Ptr(-1)}, wantErr: true},
		{name: "top logprobs above range", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", TopLogprobs: testutil.Ptr(21)}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.opts)
			if tc.wantErr && err == nil {
				t.Errorf("Validate(%+v) succeeded unexpectedly; want a rejection", tc.opts)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate(%+v) returned an unexpected error: %v", tc.opts, err)
			}
		})
	}
}

func TestValidate_RejectsNonFiniteNumbers(t *testing.T) {
	nan, posInf, negInf := math.NaN(), math.Inf(1), math.Inf(-1)

	tests := []struct {
		name string
		opts ModelOptions
	}{
		{name: "temperature NaN", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.5-flash", Temperature: &nan}},
		{name: "temperature positive infinity", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.5-flash", Temperature: &posInf}},
		{name: "temperature negative infinity", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.5-flash", Temperature: &negInf}},
		{name: "top p NaN", opts: ModelOptions{Provider: "gemini", Model: "gemini-3.5-flash", TopP: &nan}},
		{name: "frequency penalty NaN", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", FrequencyPenalty: &nan}},
		{name: "presence penalty NaN", opts: ModelOptions{Provider: "openai", Model: "gpt-4o", PresencePenalty: &nan}},
		{name: "min p NaN", opts: ModelOptions{Provider: "local", Model: "llama-3.3-70b", MinP: &nan}},
		{name: "repetition penalty positive infinity", opts: ModelOptions{Provider: "local", Model: "llama-3.3-70b", RepetitionPenalty: &posInf}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.opts); err == nil {
				t.Errorf("Validate(%s) succeeded unexpectedly; a non finite number cannot be serialized to JSON", tc.name)
			}
		})
	}
}

func TestParseConfigFile_RejectsUnknownKeys(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "misspelled top level key", body: "modles:\n  a:\n    model: grok-4.6\n"},
		{name: "misspelled model field", body: "models:\n  a:\n    model: grok-4.6\n    temprature: 0.2\n"},
		{name: "misspelled defaults field", body: "defaults:\n  max_token: 100\nmodels:\n  a:\n    model: grok-4.6\n"},
		{name: "camel case field", body: "models:\n  a:\n    model: grok-4.6\n    maxTokens: 100\n"},
		{name: "uppercase field", body: "models:\n  a:\n    model: grok-4.6\n    Temperature: 0.2\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseConfigFile(writeDeclaration(t, tc.body)); err == nil {
				t.Error("ParseConfigFile succeeded unexpectedly; a misspelled key silently drops the parameter")
			}
		})
	}
}

func TestParseConfigFile_TypeMismatchIsRejected(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "string for integer", body: "max_tokens: abc"},
		{name: "fraction for integer", body: "max_tokens: 1.5"},
		{name: "integer overflow", body: "max_tokens: 99999999999999999999"},
		{name: "list for scalar", body: "temperature: [0.1]"},
		{name: "map for scalar", body: "model: {a: b}"},
		{name: "string for float", body: "temperature: warm"},
		{name: "scalar for list", body: "stop: END"},
		{name: "bad boolean", body: "include_thoughts: maybe"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeDeclaration(t, "models:\n  a:\n    "+tc.body+"\n")
			if _, err := ParseConfigFile(path); err == nil {
				t.Errorf("ParseConfigFile accepted %q", tc.body)
			}
		})
	}
}
