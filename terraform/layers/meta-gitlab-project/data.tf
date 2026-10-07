
data "terraform_remote_state" "group_foundation" {
  backend = "http"
  config = {
    address = "https://gitlab.com/api/v4/projects/86417732/terraform/state/group-foundation"
  }
}

data "terraform_remote_state" "group_federation_anthropic" {
  backend = "http"
  config = {
    address = "https://gitlab.com/api/v4/projects/86417732/terraform/state/group-federation-anthropic"
  }
}

data "terraform_remote_state" "group_federation_gcp" {
  backend = "http"
  config = {
    address = "https://gitlab.com/api/v4/projects/86417732/terraform/state/group-federation-gcp"
  }
}

data "terraform_remote_state" "group_federation_azure" {
  backend = "http"
  config = {
    address = "https://gitlab.com/api/v4/projects/86417732/terraform/state/group-federation-azure"
  }
}

ephemeral "vault_kv_secret_v2" "state_backend" {
  provider = vault.bastion
  mount    = "secret"
  name     = "gitlab-ci-with-code-reviewer/terraform/state-backend"
}

# Anthropic admin API key MUST be stored in Bastion Vault at parent-group-governance/ai-provider-console/anthropic.
# The ephemeral block retrieves the credential in memory for provider authentication.
ephemeral "vault_kv_secret_v2" "anthropic_admin_key" {
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
