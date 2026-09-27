
data "anthropic_organization" "current" {
  count = local.enable_anthropic ? 1 : 0
}

data "google_project" "current" {
  count      = local.enable_google ? 1 : 0
  project_id = var.google.project_id
}

data "azuread_client_config" "current" {
  count = local.enable_azure ? 1 : 0
}

data "azurerm_client_config" "current" {
  count = local.enable_azure ? 1 : 0
}
