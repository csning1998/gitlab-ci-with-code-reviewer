
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
  mount = var.vault_secret.mount
  name  = var.vault_secret.name
}

locals {
  reviewer_token = data.vault_kv_secret_v2.code_reviewer_bot.data["token"]

  core_keys = var.legacy_alias_enabled ? [
    "REVIEW_MR_REVIEWER",
    "CLAUDE_MR_REVIEWER",
    "GEMINI_MR_REVIEWER",
    ] : [
    "REVIEW_MR_REVIEWER",
  ]

  extra_keys = keys(var.extra_variables)

  all_keys_set = toset(concat(local.core_keys, local.extra_keys))

  values_map = merge(
    {
      REVIEW_MR_REVIEWER = local.reviewer_token
    },
    var.legacy_alias_enabled ? {
      CLAUDE_MR_REVIEWER = local.reviewer_token
      GEMINI_MR_REVIEWER = local.reviewer_token
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
