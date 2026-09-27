
resource "anthropic_federation_issuer" "gitlab_saas" {
  count = local.enable_anthropic ? 1 : 0

  name                     = var.anthropic.issuer_name
  issuer_url               = var.anthropic.issuer_url
  check_jti                = true
  max_jwt_lifetime_seconds = 3600
  jwks                     = { type = "discovery" }

  lifecycle {
    prevent_destroy = true
  }
}
