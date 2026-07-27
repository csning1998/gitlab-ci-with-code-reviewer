
data "vault_kv_secret_v2" "sonar_token" {
  mount = "secret"
  name  = "sonarqube"
}
