
terraform {
  required_version = ">= 1.14.0"
  required_providers {
    gitlab = {
      source  = "gitlabhq/gitlab"
      version = "19.2.0"
    }
    vault = {
      source  = "hashicorp/vault"
      version = "5.5.0"
    }
  }
}

data "vault_kv_secret_v2" "code_reviewer_bot" {
  count = var.vault_secrets.code_reviewer != null ? 1 : 0

  mount = var.vault_secrets.code_reviewer.mount
  name  = var.vault_secrets.code_reviewer.name
}

data "vault_kv_secret_v2" "tag_bot" {
  count = var.vault_secrets.auto_version_tag != null ? 1 : 0

  mount = var.vault_secrets.auto_version_tag.mount
  name  = var.vault_secrets.auto_version_tag.name
}

locals {
  reviewer_token = length(data.vault_kv_secret_v2.code_reviewer_bot) > 0 ? data.vault_kv_secret_v2.code_reviewer_bot[0].data["token"] : null
  tag_token      = length(data.vault_kv_secret_v2.tag_bot) > 0 ? data.vault_kv_secret_v2.tag_bot[0].data["token"] : null

  core_keys = concat(
    var.vault_secrets.code_reviewer != null ? (
      var.legacy_alias_enabled ? [
        "REVIEW_MR_REVIEWER",
        "CLAUDE_MR_REVIEWER",
        "GEMINI_MR_REVIEWER",
        ] : [
        "REVIEW_MR_REVIEWER",
      ]
    ) : [],
    var.vault_secrets.auto_version_tag != null ? ["TAG_PUSH_TOKEN"] : [],
  )

  extra_keys = keys(var.extra_variables)

  all_keys_set = toset(concat(local.core_keys, local.extra_keys))

  values_map = merge(
    local.reviewer_token != null ? {
      REVIEW_MR_REVIEWER = local.reviewer_token
    } : {},
    local.reviewer_token != null && var.legacy_alias_enabled ? {
      CLAUDE_MR_REVIEWER = local.reviewer_token
      GEMINI_MR_REVIEWER = local.reviewer_token
    } : {},
    local.tag_token != null ? {
      TAG_PUSH_TOKEN = local.tag_token
    } : {},
    var.extra_variables,
  )
}

resource "gitlab_project_variable" "reviewer_variable" {
  for_each = local.all_keys_set

  project   = tostring(var.gitlab_project_id)
  key       = each.key
  value     = local.values_map[each.key]
  masked    = true
  hidden    = true
  raw       = true
  protected = false
}
