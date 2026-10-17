
# Meta GitLab Project Layer

## Description

This layer provisions the GitLab project metadata, CI/CD catalog enablement, and registers Workload Identity Federation (WIF) credentials with the upstream governance layer.

## Cross-Project Credentials

Managing Anthropic WIF resources requires the Anthropic Console Admin API Key. This layer reads the key from the central governance Vault path via an ephemeral secret:

- **Vault Path**: `secret/<path/to/ai-provider-console>/anthropic`
- **Field**: `anthropic_admin_api_key`

### Provisioning the Secret in Bastion Vault

```bash
vault kv put \
    -address='https://<vault-endpoint>' \
    -ca-cert="/path/to/ca.pem" \
    secret/<path/to/ai-provider-console>/anthropic \
    anthropic_admin_api_key='<YOUR_ANTHROPIC_ADMIN_API_KEY>'
```
