
data "terraform_remote_state" "group_foundation" {
  backend = "http"
  config = merge(module.local_credential_contexts.state_auth_gitlab_saas, {
    address = "https://gitlab.com/api/v4/projects/86417732/terraform/state/group-foundation"
  })
}

ephemeral "vault_kv_secret_v2" "state_backend" {
  provider = vault.bastion
  mount    = "secret"
  name     = "gitlab-ci-with-code-reviewer/state-backend"
}
