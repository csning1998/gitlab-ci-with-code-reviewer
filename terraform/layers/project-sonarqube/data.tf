
data "terraform_remote_state" "meta_gitlab_project" {
  backend = "http"
  config = {
    address = "https://gitlab.com/api/v4/projects/83083739/terraform/state/meta-gitlab-project"
  }
}

ephemeral "vault_kv_secret_v2" "state_backend" {
  provider = vault.bastion
  mount    = "secret"
  name     = "parent-group-governance/terraform/state-backend"
}

data "vault_kv_secret_v2" "sonarqube" {
  provider = vault.bastion
  mount    = "secret"
  name     = "parent-group-governance/sonarqube/ci-analysis-bot"
}
