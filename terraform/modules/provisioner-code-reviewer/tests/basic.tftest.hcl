
mock_provider "gitlab" {}
mock_provider "vault" {
  mock_data "vault_kv_secret_v2" {
    defaults = {
      data = {
        token = "mock-reviewer-token-for-test"
      }
    }
  }
}

variables {
  gitlab_project_id = "12345678"
}

run "verify_default_vault_resolution_with_legacy_aliases" {
  command = plan

  assert {
    condition     = length(gitlab_project_variable.reviewer_variable) == 4
    error_message = "The module MUST provision exactly four variables (REVIEW_MR_REVIEWER, CLAUDE_MR_REVIEWER, GEMINI_MR_REVIEWER, TAG_PUSH_TOKEN) by default."
  }

  assert {
    condition     = gitlab_project_variable.reviewer_variable["TAG_PUSH_TOKEN"].masked == true
    error_message = "TAG_PUSH_TOKEN MUST be provisioned with masked enabled."
  }

  assert {
    condition     = gitlab_project_variable.reviewer_variable["REVIEW_MR_REVIEWER"].masked == true
    error_message = "The reviewer token MUST be provisioned with masked enabled."
  }

  assert {
    condition     = gitlab_project_variable.reviewer_variable["REVIEW_MR_REVIEWER"].protected == false
    error_message = "The reviewer token MUST NOT be protected to ensure MR pipelines on feature branches can access it."
  }

  assert {
    condition     = gitlab_project_variable.reviewer_variable["REVIEW_MR_REVIEWER"].raw == true
    error_message = "The reviewer token MUST be configured as raw."
  }
}

run "verify_provisioning_without_legacy_aliases" {
  command = plan

  variables {
    legacy_alias_enabled = false
  }

  assert {
    condition     = length(gitlab_project_variable.reviewer_variable) == 2
    error_message = "The module MUST provision REVIEW_MR_REVIEWER and TAG_PUSH_TOKEN when legacy_alias_enabled is false."
  }

  assert {
    condition     = contains(keys(gitlab_project_variable.reviewer_variable), "REVIEW_MR_REVIEWER")
    error_message = "The module MUST contain REVIEW_MR_REVIEWER."
  }

  assert {
    condition     = contains(keys(gitlab_project_variable.reviewer_variable), "TAG_PUSH_TOKEN")
    error_message = "The module MUST contain TAG_PUSH_TOKEN."
  }
}

run "verify_provisioning_with_extra_variables" {
  command = plan

  variables {
    legacy_alias_enabled = false
    extra_variables = {
      GEMINI_API_KEY = "mock-gemini-key"
      MAX_TOTAL_DIFF = "400000"
    }
  }

  assert {
    condition     = length(gitlab_project_variable.reviewer_variable) == 4
    error_message = "The module MUST provision REVIEW_MR_REVIEWER, TAG_PUSH_TOKEN along with the two extra variables."
  }

  assert {
    condition     = gitlab_project_variable.reviewer_variable["GEMINI_API_KEY"].key == "GEMINI_API_KEY"
    error_message = "The module MUST provision GEMINI_API_KEY."
  }

  assert {
    condition     = gitlab_project_variable.reviewer_variable["MAX_TOTAL_DIFF"].key == "MAX_TOTAL_DIFF"
    error_message = "The module MUST provision MAX_TOTAL_DIFF."
  }
}

run "verify_custom_vault_secret_configuration" {
  command = plan

  variables {
    vault_secrets = {
      code_reviewer = {
        mount = "custom-mount"
        name  = "custom/path/reviewer-bot"
      }
      auto_version_tag = {
        mount = "custom-mount"
        name  = "custom/path/tag-bot"
      }
    }
  }

  assert {
    condition     = data.vault_kv_secret_v2.code_reviewer_bot[0].mount == "custom-mount"
    error_message = "The Vault data source mount MUST match the custom mount."
  }

  assert {
    condition     = data.vault_kv_secret_v2.code_reviewer_bot[0].name == "custom/path/reviewer-bot"
    error_message = "The Vault data source name MUST match the custom name."
  }

  assert {
    condition     = data.vault_kv_secret_v2.tag_bot[0].name == "custom/path/tag-bot"
    error_message = "The tag bot data source name MUST match the custom name."
  }
}

run "verify_omit_tag_bot" {
  command = plan

  variables {
    legacy_alias_enabled = false
    vault_secrets = {
      code_reviewer    = {}
      auto_version_tag = null
    }
  }

  assert {
    condition     = length(gitlab_project_variable.reviewer_variable) == 1
    error_message = "The module MUST provision only REVIEW_MR_REVIEWER when auto_version_tag is null and legacy aliases are disabled."
  }
}
