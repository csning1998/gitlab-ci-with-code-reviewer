
module "provisioner_gitlab_project" {
  source  = "gitlab.com/csning1998-lab/provisioner-gitlab-project/gitlab"
  version = "0.2.0"

  name         = var.gitlab_project_name
  description  = "GitLab CI pipeline with AI-powered code review using Claude and Gemini."
  visibility   = "public"
  namespace_id = tonumber(local.gitlab_group_top_level.group_id)

  only_allow_merge_if_pipeline_succeeds = false
}

resource "gitlab_project_cicd_catalog" "this" {
  project = module.provisioner_gitlab_project.project_id
  enabled = true
}

module "workload_identity_federation" {
  source  = "gitlab.com/csning1998-lab/provisioner-workload-identity-federation/gitlab"
  version = "0.5.0"

  gitlab_project = {
    id   = module.provisioner_gitlab_project.project_id
    path = module.provisioner_gitlab_project.full_path
    code = var.gitlab_project_name
  }

  anthropic_federation = {
    issuer_id       = local.ai_provider["anthropic"].issuer_id
    organization_id = local.ai_provider["anthropic"].organization_id
  }

  google_federation = {
    project_id     = local.ai_provider["google-cloud"].project_id
    project_number = local.ai_provider["google-cloud"].project_number
    pool_id        = local.ai_provider["google-cloud"].pool_id
    provider_id    = local.ai_provider["google-cloud"].provider_id
  }

  azure_federation = {
    tenant_id            = local.ai_provider["azure"].tenant_id
    subscription_id      = local.ai_provider["azure"].subscription_id
    cognitive_account_id = local.ai_provider["azure"].cognitive_account_id
    openai_endpoint      = local.ai_provider["azure"].openai_endpoint
    subjects = [
      "project_path:${module.provisioner_gitlab_project.full_path}:ref_type:branch:ref:main",
      "project_path:${module.provisioner_gitlab_project.full_path}:ref_type:branch:ref:refactor/integration-gcp-azure",
      "project_path:${module.provisioner_gitlab_project.full_path}:ref_type:branch:ref:test/wif-boundaries",
    ]
  }
}

module "code_reviewer" {
  source    = "../../modules/provisioner-code-reviewer"
  providers = { vault = vault.bastion }

  gitlab_project_id    = module.provisioner_gitlab_project.project_id
  legacy_alias_enabled = true
}

module "github_mirror" {
  source  = "gitlab.com/csning1998-lab/provisioner-github-mirror/gitlab"
  version = "0.3.1"

  gitlab_project_id = module.provisioner_gitlab_project.project_id

  github_repository = {
    name  = var.gitlab_project_name
    owner = var.github_owner
  }
}
