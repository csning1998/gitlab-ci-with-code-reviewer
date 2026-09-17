
# Perform `terraform apply -target=module.local_credential_contexts.local_file.bastion_ca_cert` if greenfield
module "local_credential_contexts" {
  source  = "gitlab.com/csning1998-lab/contexts-local-credential/gitlab"
  version = "~> 0.1.2"
}

module "baseline" {
  source  = "gitlab.com/csning1998-lab/provisioner-gitlab-project/gitlab"
  version = "~> 0.1.1"

  name         = "gitlab-ci-with-code-reviewer"
  description  = "GitLab CI pipeline with AI-powered code review using Claude and Gemini."
  visibility   = "public"
  namespace_id = data.terraform_remote_state.group_foundation.outputs.group_id

  only_allow_merge_if_pipeline_succeeds = false
}

resource "gitlab_project_cicd_catalog" "this" {
  project = module.baseline.project_id
  enabled = true
}
