
module "local_credential_contexts" {
  source  = "gitlab.com/csning1998-lab/contexts-local-credential/gitlab"
  version = "~> 0.1.2"
}

resource "gitlab_project_variable" "sonar_host_url" {
  project   = data.terraform_remote_state.meta_gitlab_project.outputs.project_id
  key       = "SONAR_HOST_URL"
  value     = "http://sonarqube:9000"
  masked    = false
  protected = false
}

resource "gitlab_project_variable" "sonar_token" {
  project   = data.terraform_remote_state.meta_gitlab_project.outputs.project_id
  key       = "SONAR_TOKEN"
  value     = data.vault_kv_secret_v2.sonarqube.data["token"]
  masked    = true
  protected = false
}
