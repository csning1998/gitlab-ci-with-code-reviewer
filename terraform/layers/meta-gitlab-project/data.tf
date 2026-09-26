
data "terraform_remote_state" "group_foundation" {
  backend = "http"
  config = merge(module.local_credential_contexts.state_auth_gitlab_saas, {
    address = "https://gitlab.com/api/v4/projects/86417732/terraform/state/group-foundation"
  })
}

data "terraform_remote_state" "group_federation_anthropic" {
  backend = "http"
  config = merge(module.local_credential_contexts.state_auth_gitlab_saas, {
    address = "https://gitlab.com/api/v4/projects/86417732/terraform/state/group-federation-anthropic"
  })
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


variable "gitlab_project_name" {
  description = "The title of this project"
  type        = string
  default     = "gitlab-ci-with-code-reviewer"
}

output "project_id" {
  description = "Numeric identifier of the project 'gitlab-ci-with-code-reviewer'."
  value       = module.baseline.project_id
}

output "repository_ssh_url" {
  description = "SSH repository clone URI."
  value       = module.baseline.repository_ssh_url
}
