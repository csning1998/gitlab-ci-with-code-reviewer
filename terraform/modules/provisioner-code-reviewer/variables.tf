
variable "gitlab_project_id" {
  description = "GitLab project identifier (numeric ID or full path) where reviewer variables are provisioned."
  type        = string
}

variable "vault_secret" {
  description = "Vault KV v2 secret configuration containing the reviewer bot token."
  type = object({
    mount = optional(string, "secret")
    name  = optional(string, "gitlab-ci-with-code-reviewer/integration/code-reviewer-bot")
  })
  default = {}
}

variable "legacy_alias_enabled" {
  description = "Controls whether legacy variable names (CLAUDE_MR_REVIEWER and GEMINI_MR_REVIEWER) are registered alongside REVIEW_MR_REVIEWER."
  type        = bool
  default     = true
}

variable "extra_variables" {
  description = "Optional additional CI/CD project variables for reviewer configuration (e.g. GEMINI_API_KEY, MAX_TOTAL_DIFF)."
  type        = map(string)
  default     = {}
}
