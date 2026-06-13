
terraform {
  required_version = ">= 1.8.0"
  required_providers {
    gitlab = {
      source  = "gitlabhq/gitlab"
      version = "19.0.0"
    }
    local = {
      source  = "hashicorp/local"
      version = "2.9.0"
    }
  }

  backend "http" {
    address        = "https://gitlab.com/api/v4/projects/83083739/terraform/state/default"
    lock_address   = "https://gitlab.com/api/v4/projects/83083739/terraform/state/default/lock"
    unlock_address = "https://gitlab.com/api/v4/projects/83083739/terraform/state/default/lock"
    lock_method    = "POST"
    unlock_method  = "DELETE"
    retry_wait_min = 5
  }
}

provider "gitlab" {
  token = var.gitlab_token
}
