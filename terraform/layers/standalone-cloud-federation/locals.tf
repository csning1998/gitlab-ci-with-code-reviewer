
locals {
  enable_anthropic = var.anthropic != null && var.anthropic.enabled
  enable_google    = var.google != null && var.google.enabled
  enable_azure     = var.azure != null && var.azure.enabled

  google_required_services = toset([
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "sts.googleapis.com",
    "aiplatform.googleapis.com",
    "generativelanguage.googleapis.com",
  ])
}
