
output "anthropic" {
  description = "Anthropic Claude federation pre-configuration identifiers."
  value = local.enable_anthropic ? {
    organization_id = data.anthropic_organization.current[0].id
    issuer_id       = anthropic_federation_issuer.gitlab_saas[0].id
  } : null
}

output "google" {
  description = "Google Cloud Workload Identity Federation pre-configuration identifiers."
  value = local.enable_google ? {
    project_id     = data.google_project.current[0].project_id
    project_number = data.google_project.current[0].number
    pool_id        = google_iam_workload_identity_pool.gitlab_saas[0].workload_identity_pool_id
    provider_id    = google_iam_workload_identity_pool_provider.gitlab_saas[0].workload_identity_pool_provider_id
  } : null
}

output "azure" {
  description = "Microsoft Azure OpenAI pre-configuration identifiers."
  value = local.enable_azure ? {
    tenant_id            = data.azuread_client_config.current[0].tenant_id
    subscription_id      = data.azurerm_client_config.current[0].subscription_id
    cognitive_account_id = azurerm_cognitive_account.openai[0].id
    openai_endpoint      = azurerm_cognitive_account.openai[0].endpoint
    key_vault_id         = azurerm_key_vault.openai[0].id
    key_id               = azurerm_key_vault_key.openai[0].versionless_id
    virtual_network_id   = one(azurerm_virtual_network.openai[*].id)
    subnet_id            = one(azurerm_subnet.private_endpoints[*].id)
    private_endpoint_id  = one(azurerm_private_endpoint.openai[*].id)
  } : null
}
