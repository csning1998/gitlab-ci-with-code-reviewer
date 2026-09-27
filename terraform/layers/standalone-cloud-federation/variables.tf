
variable "anthropic" {
  description = "Anthropic Claude federation pre-configuration object."
  type = object({
    enabled     = optional(bool, false)
    issuer_name = optional(string, "issuer-gitlab-saas")
    issuer_url  = optional(string, "https://gitlab.com")
  })
  default = {}
}

variable "google" {
  description = "Google Cloud Workload Identity Federation pre-configuration object."
  type = object({
    enabled                = optional(bool, false)
    project_id             = optional(string)
    pool_id                = optional(string, "gitlab-saas-pool")
    provider_id            = optional(string, "gitlab-saas-provider")
    allowed_namespace_path = optional(string)
  })
  default = {}

  validation {
    condition = var.google == null || !var.google.enabled || (
      var.google.project_id != null && length(trimspace(var.google.project_id)) > 0
    )
    error_message = "google.project_id is required when google.enabled is true."
  }
}

variable "azure" {
  description = "Microsoft Azure OpenAI pre-configuration object."
  type = object({
    enabled                = optional(bool, false)
    tenant_id              = optional(string)
    subscription_id        = optional(string)
    resource_group_name    = optional(string, "rg-federation-openai")
    location               = optional(string, "eastus")
    cognitive_account_name = optional(string)
    sku_name               = optional(string, "S0")
    is_premium_tier        = optional(bool, false)
    model_name             = optional(string, "gpt-4o")
    model_version          = optional(string, "2024-05-13")
    cmk_expiration_date    = optional(string, "2027-01-01T00:00:00Z")
    key_vault_ip_rules     = optional(list(string), [])
    virtual_network = optional(object({
      address_space                  = optional(list(string), ["10.0.0.0/16"])
      private_endpoint_subnet_prefix = optional(string, "10.0.1.0/24")
    }))
  })
  default = {}

  validation {
    condition = var.azure == null || !var.azure.enabled || (
      var.azure.tenant_id != null && length(trimspace(var.azure.tenant_id)) > 0 &&
      var.azure.subscription_id != null && length(trimspace(var.azure.subscription_id)) > 0 &&
      var.azure.cognitive_account_name != null && length(trimspace(var.azure.cognitive_account_name)) > 0
    )
    error_message = "azure.tenant_id, azure.subscription_id, and azure.cognitive_account_name are required when azure.enabled is true."
  }
}
