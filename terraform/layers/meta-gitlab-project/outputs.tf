
output "project_id" {
  description = "Numeric identifier of the project 'gitlab-ci-with-code-reviewer'."
  value       = module.baseline.project_id
}

output "repository_ssh_url" {
  description = "SSH repository clone URI."
  value       = module.baseline.repository_ssh_url
}
