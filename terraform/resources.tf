
resource "gitlab_project" "this" {
  name             = var.repository_name
  description      = var.repository_description
  visibility_level = var.visibility

  merge_method  = "ff"
  squash_option = "always"

  # MR Workflow
  remove_source_branch_after_merge         = true
  ci_push_repository_for_job_token_allowed = true # Enables job token git push access required by formatting auto-commit tasks

  # Features
  issues_access_level    = "enabled"
  wiki_access_level      = "disabled"
  initialize_with_readme = false
  shared_runners_enabled = false
}

resource "gitlab_branch_protection" "main" {
  project = gitlab_project.this.id
  branch  = "main"

  allowed_to_push  = [{ access_level = "no one" }]
  allowed_to_merge = [{ access_level = "maintainer" }]

  allow_force_push = false
}

# Registers the repository in the GitLab CI/CD Catalog to enable publication of template components upon tagged releases.
resource "gitlab_project_cicd_catalog" "this" {
  project = gitlab_project.this.id
  enabled = true
}

resource "gitlab_user_runner" "this" {
  runner_type = "project_type"
  project_id  = gitlab_project.this.id
  description = var.runner_description
  tag_list    = var.runner_tag_list
  locked      = true
}

resource "local_file" "runner_config" {
  filename = "${path.module}/../runner-config/config.toml"
  content = templatefile("${path.module}/templates/config.toml.tftpl", {
    runner_name  = var.runner_description
    runner_token = gitlab_user_runner.this.token
    runner_tags  = var.runner_tag_list
  })
  file_permission = "0600"
}

# Assigns developer access level to grant write permissions for updating merge request labels
# and creating inline discussions, as reporter access limits operations to read-only queries and notes.
resource "gitlab_project_access_token" "claude_mr_reviewer" {
  project      = gitlab_project.this.id
  name         = "Claude MR Reviewer"
  access_level = "developer"
  scopes       = ["api", "read_api"]
  expires_at   = "2027-07-24"
}

resource "gitlab_project_variable" "claude_mr_reviewer" {
  project   = gitlab_project.this.id
  key       = "CLAUDE_MR_REVIEWER"
  value     = gitlab_project_access_token.claude_mr_reviewer.token
  masked    = true
  protected = false
}

resource "gitlab_project_label" "this" {
  for_each = local.project_labels

  project     = gitlab_project.this.id
  name        = each.key
  color       = each.value.color
  description = each.value.description
}
