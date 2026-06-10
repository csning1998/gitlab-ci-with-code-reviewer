
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

  backend "http" {}
}

provider "gitlab" {
  token = var.gitlab_token
}
