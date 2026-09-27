
# Perform `terraform apply -target=module.local_credential_contexts.local_file.bastion_ca_cert` if greenfield
module "local_credential_contexts" {
  source  = "gitlab.com/csning1998-lab/contexts-local-credential/gitlab"
  version = "~> 0.3.0"
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
  source  = "gitlab.com/csning1998-lab/provisioner-workload-identity-federation/gitlab"
  version = "~> 0.3.0"

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

  google_federation = {
    project_id     = data.terraform_remote_state.group_federation_gcp.outputs.project.id
    project_number = data.terraform_remote_state.group_federation_gcp.outputs.project.number
    pool_id        = data.terraform_remote_state.group_federation_gcp.outputs.pool.id
    provider_id    = data.terraform_remote_state.group_federation_gcp.outputs.provider.id
  }

  azure_federation = {
    tenant_id            = data.terraform_remote_state.group_federation_azure.outputs.tenant.id
    subscription_id      = data.terraform_remote_state.group_federation_azure.outputs.subscription.id
    cognitive_account_id = data.terraform_remote_state.group_federation_azure.outputs.openai.id
    openai_endpoint      = data.terraform_remote_state.group_federation_azure.outputs.openai.endpoint
    subjects = [
      "project_path:${module.baseline.full_path}:ref_type:branch:ref:main",
      "project_path:${module.baseline.full_path}:ref_type:branch:ref:refactor/integration-gcp-azure",
      "project_path:${module.baseline.full_path}:ref_type:branch:ref:test/wif-boundaries",
    ]
  }
}

module "code_reviewer" {
  source    = "../../modules/provisioner-code-reviewer"
  providers = { vault = vault.bastion }

  gitlab_project_id    = module.baseline.project_id
  legacy_alias_enabled = true
}

module "github_mirror" {
  source  = "gitlab.com/csning1998-lab/provisioner-github-mirror/gitlab"
  version = "~> 0.3.0"

  gitlab_project_id = module.baseline.project_id

  github_repository = {
    name  = var.gitlab_project_name
    owner = var.github_owner
  }
}
