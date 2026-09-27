
terraform {
  required_version = ">= 1.14.0"
  required_providers {
    anthropic = {
      source  = "ippontech/anthropic"
      version = "1.43.5"
    }
    azuread = {
      source  = "hashicorp/azuread"
      version = "3.10.0"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "5.7.0"
    }
    google = {
      source  = "hashicorp/google"
      version = "8.4.0"
    }
  }
}

# The Anthropic provider rejects an empty credential set during provider configuration.
provider "anthropic" {
  admin_api_key = local.enable_anthropic ? null : "unused"
}

provider "google" {
  project = var.google.project_id
}

provider "azuread" {
  tenant_id = var.azure.tenant_id
}

provider "azurerm" {
  subscription_id = var.azure.subscription_id
  tenant_id       = var.azure.tenant_id
  features {}
}
