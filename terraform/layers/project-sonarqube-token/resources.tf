
ephemeral "vault_kv_secret_v2" "sonarqube_admin" {
  provider = vault.bastion
  mount    = "secret"
  name     = "parent-group-governance/sonarqube/admin-account"
}

module "local_credential_contexts" {
  source  = "gitlab.com/csning1998-lab/contexts-local-credential/gitlab"
  version = "~> 0.1.2"
}

resource "sonarqube_user_token" "ci_analysis" {
  name        = "gitlab-ci-with-code-reviewer-analysis"
  type        = "PROJECT_ANALYSIS_TOKEN"
  project_key = "csning1998-lab-gitlab-ci-with-code-reviewer"
}

resource "vault_kv_secret_v2" "sonarqube" {
  provider  = vault.bastion
  mount     = "secret"
  name      = "gitlab-ci-with-code-reviewer/sonarqube"
  data_json = jsonencode({ token = sonarqube_user_token.ci_analysis.token })
}
