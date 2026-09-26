
# Perform `terraform apply -target=module.local_credential_contexts.local_file.bastion_ca_cert` if greenfield
module "local_credential_contexts" {
  source = "../../../../parent-group-governance/terraform/modules/contexts-local-credential"
}

module "baseline" {
  source  = "gitlab.com/csning1998-lab/provisioner-gitlab-project/gitlab"
  version = "~> 0.1.1"

  name         = var.gitlab_project_name
  description  = "GitLab CI pipeline with AI-powered code review using Claude and Gemini."
  visibility   = "public"
  namespace_id = data.terraform_remote_state.group_foundation.outputs.group_id

  only_allow_merge_if_pipeline_succeeds = false
}

resource "gitlab_project_cicd_catalog" "this" {
  project = module.baseline.project_id
  enabled = true
}

module "workload_identity_federation" {
  source = "../../../../parent-group-governance/terraform/modules/provisioner-workload-identity-federation"

  providers = {
    vault = vault.bastion
  }

  gitlab_project = {
    id   = module.baseline.project_id
    path = module.baseline.full_path
    code = var.gitlab_project_name
  }

  anthropic_federation = {
    issuer_id       = data.terraform_remote_state.group_federation_anthropic.outputs.issuers.gitlab_saas.id
    organization_id = data.terraform_remote_state.group_federation_anthropic.outputs.organization.id
  }
}

module "code_reviewer" {
  source    = "../../modules/provisioner-code-reviewer"
  providers = { vault = vault.bastion }

  gitlab_project_id    = module.baseline.project_id
  legacy_alias_enabled = true
}
