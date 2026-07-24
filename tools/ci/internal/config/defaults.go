package config

// Default LLM model identifiers establishing global fallback targets across reviewer binaries
// when model override environment variables are not explicitly defined.
const (
	defaultGeminiModel     = "gemini-3.5-flash"
	defaultClaudeModel     = "claude-sonnet-5"
	defaultClaudeMaxTokens = 16384
)
