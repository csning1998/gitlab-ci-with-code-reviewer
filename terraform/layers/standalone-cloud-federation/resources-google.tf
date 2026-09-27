
resource "google_project_service" "required" {
  for_each = local.enable_google ? local.google_required_services : toset([])

  project                    = data.google_project.current[0].project_id
  service                    = each.key
  disable_dependent_services = false
  disable_on_destroy         = false
}

resource "google_iam_workload_identity_pool" "gitlab_saas" {
  count = local.enable_google ? 1 : 0

  depends_on = [google_project_service.required]

  project                   = data.google_project.current[0].project_id
  workload_identity_pool_id = var.google.pool_id
  display_name              = "GitLab SaaS Pool"
  description               = "Workload Identity Pool for GitLab SaaS pipelines."
  disabled                  = false

  lifecycle {
    prevent_destroy = true
  }
}

resource "google_iam_workload_identity_pool_provider" "gitlab_saas" {
  count = local.enable_google ? 1 : 0

  depends_on = [google_project_service.required]

  project                            = data.google_project.current[0].project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.gitlab_saas[0].workload_identity_pool_id
  workload_identity_pool_provider_id = var.google.provider_id
  display_name                       = "GitLab SaaS Provider"
  description                        = "OIDC identity provider for GitLab SaaS."
  disabled                           = false

  attribute_mapping = {
    "google.subject"           = "assertion.sub"
    "attribute.aud"            = "assertion.aud"
    "attribute.project_path"   = "assertion.project_path"
    "attribute.project_id"     = "assertion.project_id"
    "attribute.namespace_path" = "assertion.namespace_path"
    "attribute.namespace_id"   = "assertion.namespace_id"
    "attribute.ref"            = "assertion.ref"
    "attribute.ref_type"       = "assertion.ref_type"
  }

  attribute_condition = var.google.allowed_namespace_path != null ? "assertion.namespace_path == \"${var.google.allowed_namespace_path}\" || assertion.namespace_path.startsWith(\"${var.google.allowed_namespace_path}/\")" : null

  oidc {
    issuer_uri        = "https://gitlab.com"
    allowed_audiences = ["https://gitlab.com"]
  }

  lifecycle {
    prevent_destroy = true
  }
}
