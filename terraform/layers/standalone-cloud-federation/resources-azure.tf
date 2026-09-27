
locals {
  is_premium_tier   = var.azure != null && var.azure.is_premium_tier
  enable_azure_vnet = local.enable_azure && local.is_premium_tier

  azure_cognitive_account_name = coalesce(var.azure.cognitive_account_name, "unused")

  key_vault_name = var.azure.cognitive_account_name != null ? substr("kv${replace(lower(local.azure_cognitive_account_name), "/[^a-z0-9]/", "")}", 0, 24) : "kvdefault"

  private_endpoint_openai_name = var.azure != null && var.azure.cognitive_account_name != null ? substr("pe-${var.azure.cognitive_account_name}", 0, 80) : "pe-openai"
  private_endpoint_vault_name  = substr("pe-${local.key_vault_name}", 0, 80)

  cognitive_account_ip_rules = [
    for rule in var.azure.key_vault_ip_rules : replace(rule, "/\\/32$/", "")
  ]

  azure_vnet_address_space = var.azure.virtual_network != null ? coalesce(
    var.azure.virtual_network.address_space,
    ["10.0.0.0/16"]
  ) : ["10.0.0.0/16"]

  azure_subnet_prefix = var.azure.virtual_network != null ? coalesce(
    var.azure.virtual_network.private_endpoint_subnet_prefix,
    "10.0.1.0/24"
  ) : "10.0.1.0/24"
}

resource "azurerm_resource_group" "openai" {
  count = local.enable_azure ? 1 : 0

  name     = var.azure.resource_group_name
  location = var.azure.location

  lifecycle {
    prevent_destroy = true
  }
}

resource "azurerm_key_vault" "openai" {
  count = local.enable_azure ? 1 : 0

  name                = local.key_vault_name
  location            = azurerm_resource_group.openai[0].location
  resource_group_name = azurerm_resource_group.openai[0].name
  tenant_id           = data.azurerm_client_config.current[0].tenant_id
  sku_name            = local.is_premium_tier ? "premium" : "standard"

  rbac_authorization_enabled = true
  purge_protection_enabled   = true
  soft_delete_retention_days = 90

  public_network_access_enabled = !local.is_premium_tier

  network_acls {
    default_action             = "Deny"
    bypass                     = "AzureServices"
    ip_rules                   = var.azure.key_vault_ip_rules
    virtual_network_subnet_ids = local.enable_azure_vnet ? azurerm_subnet.private_endpoints[*].id : []
  }

  lifecycle {
    prevent_destroy = true

    precondition {
      condition     = length(local.key_vault_name) >= 3 && length(local.key_vault_name) <= 24
      error_message = "The derived Key Vault name must contain 3 to 24 alphanumeric characters."
    }
  }
}

resource "azurerm_user_assigned_identity" "openai" {
  count = local.enable_azure ? 1 : 0

  name                = substr("id-${local.azure_cognitive_account_name}", 0, 128)
  location            = azurerm_resource_group.openai[0].location
  resource_group_name = azurerm_resource_group.openai[0].name

  lifecycle {
    prevent_destroy = true
  }
}

resource "azurerm_role_assignment" "current_key_vault_admin" {
  count = local.enable_azure ? 1 : 0

  scope                = azurerm_key_vault.openai[0].id
  role_definition_name = "Key Vault Administrator"
  principal_id         = data.azurerm_client_config.current[0].object_id
}

resource "azurerm_role_assignment" "cognitive_encryption" {
  count = local.enable_azure ? 1 : 0

  scope                            = azurerm_key_vault.openai[0].id
  role_definition_name             = "Key Vault Crypto Service Encryption User"
  principal_id                     = azurerm_user_assigned_identity.openai[0].principal_id
  skip_service_principal_aad_check = true
}

resource "azurerm_key_vault_key" "openai" {
  count = local.enable_azure ? 1 : 0

  name         = "cognitive-account"
  key_vault_id = azurerm_key_vault.openai[0].id
  key_type     = local.is_premium_tier ? "RSA-HSM" : "RSA"
  key_size     = 2048
  key_opts     = ["unwrapKey", "wrapKey"]

  expiration_date = var.azure.cmk_expiration_date

  depends_on = [azurerm_role_assignment.current_key_vault_admin]
}

resource "azurerm_cognitive_account" "openai" {
  #checkov:skip=CKV_AZURE_134: Free tier keeps public HTTPS and denies other clients with a network ACL.
  count = local.enable_azure ? 1 : 0

  name                = local.azure_cognitive_account_name
  location            = azurerm_resource_group.openai[0].location
  resource_group_name = azurerm_resource_group.openai[0].name
  kind                = "OpenAI"
  sku_name            = var.azure.sku_name

  custom_subdomain_name              = local.azure_cognitive_account_name
  public_network_access_enabled      = !local.is_premium_tier
  local_auth_enabled                 = false
  outbound_network_access_restricted = true
  fqdns = [
    "${local.azure_cognitive_account_name}.openai.azure.com",
    "${local.key_vault_name}.vault.azure.net",
  ]

  dynamic "network_acls" {
    for_each = local.is_premium_tier ? [] : [1]

    content {
      default_action = "Deny"
      ip_rules       = local.cognitive_account_ip_rules
    }
  }

  identity {
    type         = "UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.openai[0].id]
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "azurerm_cognitive_account_customer_managed_key" "openai" {
  count = local.enable_azure ? 1 : 0

  cognitive_account_id = azurerm_cognitive_account.openai[0].id
  key_vault_key_id     = azurerm_key_vault_key.openai[0].id
  identity_client_id   = azurerm_user_assigned_identity.openai[0].client_id

  depends_on = [azurerm_role_assignment.cognitive_encryption]

  lifecycle {
    prevent_destroy = true
  }
}

resource "azurerm_cognitive_deployment" "gpt_4o" {
  count = local.enable_azure ? 1 : 0

  name                 = var.azure.model_name
  cognitive_account_id = azurerm_cognitive_account.openai[0].id

  model {
    format  = "OpenAI"
    name    = var.azure.model_name
    version = var.azure.model_version
  }

  sku {
    name     = "Standard"
    capacity = 10
  }
}

resource "azurerm_virtual_network" "openai" {
  count = local.enable_azure_vnet ? 1 : 0

  name                = substr("vnet-${local.azure_cognitive_account_name}", 0, 64)
  location            = azurerm_resource_group.openai[0].location
  resource_group_name = azurerm_resource_group.openai[0].name
  address_space       = local.azure_vnet_address_space
}

resource "azurerm_network_security_group" "private_endpoints" {
  count = local.enable_azure_vnet ? 1 : 0

  name                = "nsg-private-endpoints"
  location            = azurerm_resource_group.openai[0].location
  resource_group_name = azurerm_resource_group.openai[0].name

  security_rule {
    name                       = "DenyInternetRdp"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Deny"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "3389"
    source_address_prefix      = "Internet"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "DenyInternetSsh"
    priority                   = 110
    direction                  = "Inbound"
    access                     = "Deny"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefix      = "Internet"
    destination_address_prefix = "*"
  }
}

resource "azurerm_subnet" "private_endpoints" {
  count = local.enable_azure_vnet ? 1 : 0

  name                 = "snet-private-endpoints"
  resource_group_name  = azurerm_resource_group.openai[0].name
  virtual_network_name = azurerm_virtual_network.openai[0].name
  address_prefixes     = [local.azure_subnet_prefix]

  private_endpoint_network_policies = "Disabled"

  service_endpoint {
    service = "Microsoft.KeyVault"
  }
}

resource "azurerm_subnet_network_security_group_association" "private_endpoints" {
  count = local.enable_azure_vnet ? 1 : 0

  subnet_id                 = azurerm_subnet.private_endpoints[0].id
  network_security_group_id = azurerm_network_security_group.private_endpoints[0].id
}

resource "azurerm_private_dns_zone" "openai" {
  count = local.enable_azure_vnet ? 1 : 0

  name                = "privatelink.openai.azure.com"
  resource_group_name = azurerm_resource_group.openai[0].name
}

resource "azurerm_private_dns_zone" "vault" {
  count = local.enable_azure_vnet ? 1 : 0

  name                = "privatelink.vaultcore.azure.net"
  resource_group_name = azurerm_resource_group.openai[0].name
}

resource "azurerm_private_dns_zone_virtual_network_link" "openai" {
  count = local.enable_azure_vnet ? 1 : 0

  name                 = "link-openai"
  private_dns_zone_id  = azurerm_private_dns_zone.openai[0].id
  virtual_network_id   = azurerm_virtual_network.openai[0].id
  registration_enabled = false
}

resource "azurerm_private_dns_zone_virtual_network_link" "vault" {
  count = local.enable_azure_vnet ? 1 : 0

  name                 = "link-vault"
  private_dns_zone_id  = azurerm_private_dns_zone.vault[0].id
  virtual_network_id   = azurerm_virtual_network.openai[0].id
  registration_enabled = false
}

resource "azurerm_private_endpoint" "openai" {
  count = local.enable_azure_vnet ? 1 : 0

  name                = local.private_endpoint_openai_name
  location            = azurerm_resource_group.openai[0].location
  resource_group_name = azurerm_resource_group.openai[0].name
  subnet_id           = azurerm_subnet.private_endpoints[0].id

  private_service_connection {
    name                           = local.private_endpoint_openai_name
    private_connection_resource_id = azurerm_cognitive_account.openai[0].id
    subresource_names              = ["account"]
    is_manual_connection           = false
  }

  private_dns_zone_group {
    name                 = "openai"
    private_dns_zone_ids = [azurerm_private_dns_zone.openai[0].id]
  }
}

resource "azurerm_private_endpoint" "vault" {
  count = local.enable_azure_vnet ? 1 : 0

  name                = local.private_endpoint_vault_name
  location            = azurerm_resource_group.openai[0].location
  resource_group_name = azurerm_resource_group.openai[0].name
  subnet_id           = azurerm_subnet.private_endpoints[0].id

  private_service_connection {
    name                           = local.private_endpoint_vault_name
    private_connection_resource_id = azurerm_key_vault.openai[0].id
    subresource_names              = ["vault"]
    is_manual_connection           = false
  }

  private_dns_zone_group {
    name                 = "vault"
    private_dns_zone_ids = [azurerm_private_dns_zone.vault[0].id]
  }
}
