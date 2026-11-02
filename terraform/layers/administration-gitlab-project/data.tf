
locals {
  ai_provider = {
    for name, secret in data.vault_generic_secret.registry_platform_ai_provider : name => nonsensitive(secret.data)
  }
  gitlab_group_top_level = nonsensitive(data.vault_generic_secret.registry_platform_gitlab_group_top_level.data)
}

# parent-group-governance publishes the group and federation identifiers in the registry, in place of its Terraform state.
data "vault_generic_secret" "registry_platform_gitlab_group_top_level" {
  provider = vault.bastion
  path     = "registry/platform/gitlab-group/top-level"
}

data "vault_generic_secret" "registry_platform_ai_provider" {
  for_each = toset(["anthropic", "google-cloud", "azure"])
  provider = vault.bastion
  path     = "registry/platform/ai-provider/${each.key}"
}

ephemeral "vault_kv_secret_v2" "state_backend" {
  provider = vault.bastion
  mount    = "secret"
  name     = "${var.gitlab_token_owner_code}/terraform/state-backend"
}

# parent-group-governance/ai-provider-console/anthropic MUST hold the Anthropic Admin API key and an org:admin OAuth token.
# The ephemeral block retrieves the credential in memory for provider authentication.
ephemeral "vault_kv_secret_v2" "anthropic_console" {
  provider = vault.bastion
  mount    = "secret"
  name     = "parent-group-governance/ai-provider-console/anthropic"
}

ephemeral "vault_kv_secret_v2" "github_publication" {
  provider = vault.bastion
  mount    = "secret"
  name     = "parent-group-governance/github/publication"
}

variable "gitlab_project_name" {
  description = "The title of this project"
  type        = string
  default     = "gitlab-ci-with-code-reviewer"
}

variable "gitlab_token_owner_code" {
  description = "Specifies the owner code whose state backend token authenticates the gitlab provider. The first creation of the project sets parent-group-governance under the credential-distributor identity, since a token limited to the project cannot exist before the project."
  type        = string
  default     = "gitlab-ci-with-code-reviewer"

  validation {
    condition     = contains(["gitlab-ci-with-code-reviewer", "parent-group-governance"], var.gitlab_token_owner_code)
    error_message = "gitlab_token_owner_code must be gitlab-ci-with-code-reviewer, or parent-group-governance for the first creation of the project."
  }
}

variable "github_owner" {
  description = "Specifies the GitHub account or organization login hosting the mirrored repository."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$", var.github_owner))
    error_message = "github_owner must be a GitHub login of 1 to 39 characters. A hyphen must not be the first or last character."
  }
}

output "project_id" {
  description = "Numeric identifier of the project 'gitlab-ci-with-code-reviewer'."
  value       = module.provisioner_gitlab_project.project_id
}

output "repository_ssh_url" {
  description = "SSH repository clone URI."
  value       = module.provisioner_gitlab_project.repository_ssh_url
}
