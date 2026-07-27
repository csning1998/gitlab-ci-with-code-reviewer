
variable "repository_name" {
  description = "The name of the repository"
  type        = string
  default     = "gitlab-ci-with-code-reviewer"
}

variable "repository_description" {
  description = "Description of the repository"
  type        = string
  default     = "GitLab CI pipeline with AI-powered code review using Claude and Gemini."
}

variable "visibility" {
  description = "Visibility of the project. Can be 'public', 'private', or 'internal'."
  type        = string
  default     = "public"

  validation {
    condition     = contains(["public", "private", "internal"], var.visibility)
    error_message = "The visibility must be one of: public, private, or internal."
  }
}

variable "gitlab_token" {
  description = "GitLab Personal Access Token with api scope"
  type        = string
  sensitive   = true
}

variable "runner_description" {
  description = "Display name of the project runner registered to this repository"
  type        = string
  default     = "local-podman-runner"
}

variable "runner_tag_list" {
  description = "Tag list for job matching; jobs must declare matching tags to run on this runner"
  type        = list(string)
  default     = ["podman", "local"]
}

variable "vault_address" {
  description = "URL of the self-hosted Vault instance, reachable from the machine running terraform apply."
  type        = string
  default     = "https://127.0.0.1:8222"
}

variable "vault_token" {
  description = "Authenticates the vault provider against the self-hosted Vault instance."
  type        = string
  sensitive   = true
}

variable "vault_ca_cert_path" {
  description = "Absolute path to the self-hosted Vault instance's CA certificate, from csning1998-lab-meta-provision's vault/tls directory."
  type        = string
}
