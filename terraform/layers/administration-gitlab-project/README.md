# Administration GitLab Project Layer

## Description

This layer provisions GitLab project metadata, enables CI/CD catalog publishing, and configures Workload Identity Federation (WIF) integrations across AI and cloud providers.

### Anthropic WIF Credentials

Managing Anthropic WIF resources requires two credentials read from Bastion Vault in memory via an `ephemeral` secret at `secret/parent-group-governance/ai-provider-console/anthropic`:

| Vault field                    | Content                                      | Lifetime       | Provider argument | Target resources                      |
| ------------------------------ | -------------------------------------------- | -------------- | ----------------- | ------------------------------------- |
| `anthropic_admin_api_key`      | Persistent Admin API key (`sk-ant-admin...`) | Until revoked  | `admin_api_key`   | `anthropic_workspace`                 |
| `anthropic_oauth_access_token` | Temporary `org:admin` OAuth token            | Within minutes | `auth_token`      | Service accounts and federation rules |

The Anthropic Terraform provider initializes two independent HTTP clients without fallback. The provider sends the credential of the workspace endpoints in the `x-api-key` header, which does not accept an OAuth token. Conversely, the federation and service account endpoints require the OAuth token via the `Authorization: Bearer` header and reject Admin API keys. Each field therefore MUST supply its designated credential type. The upstream credential architecture is documented in `parent-group-governance/terraform/layers/group-federation-anthropic/README.md` Section 1 Item B.

### Provisioning Bastion Vault Credentials

The operator writes the Admin API key once, and writes a fresh OAuth token immediately before each `terraform apply`. Both credentials reach Vault through stdin, hence neither enters the shell history or the argument list of `vault`, which other local users can read through `/proc/<pid>/cmdline`. The commands strip the trailing newline, since `key=-` stores stdin verbatim and a newline invalidates the HTTP header of the credential.

1. **Write the persistent Admin API key once from stdin**

    ```bash
    (
        eval "$(vault-proxy-env parent-group-governance)"
        printf 'Admin API key: ' && read -rs anthropic_key && echo
        printf '%s' "$anthropic_key" | vault kv put -mount=secret parent-group-governance/ai-provider-console/anthropic \
            anthropic_admin_api_key=-
    )
    ```

2. **Refresh and patch the temporary OAuth token immediately before apply**

    ```bash
    ant auth login --profile issuer-gitlab-saas --scope "org:admin"
    (
        eval "$(vault-proxy-env parent-group-governance)"
        ant auth print-credentials --profile issuer-gitlab-saas --access-token | tr -d '\n' | \
            vault kv patch -mount=secret -method=rw parent-group-governance/ai-provider-console/anthropic \
                anthropic_oauth_access_token=-
    )
    ```
