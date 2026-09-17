# Claude API Key Provisioning Strategy

## Section 1. Architectural Scope and Terminology

### Item A. Scope

This document specifies the credential provisioning architecture for Anthropic Claude integration in the `gitlab-ci-with-code-reviewer` component. A Claude review job is a GitLab CI job which executes automated code review through the Anthropic Claude API. Every repository which includes `gitlab-ci-with-code-reviewer` MUST provide an authentication credential to the Claude review job.

### Item B. Lexicon

A static API key is a persistent alphanumeric credential which authorizes API requests without credential expiration. Workload Identity Federation is an authentication mechanism in which an external workload exchanges a signed identity token for a short lived access token. A federation issuer is an OpenID Connect identity provider which signs identity tokens for client verification. A service account is a nonhuman identity resource in Anthropic Console to which permissions are assigned. A federation rule is an authorization policy which binds incoming identity token claims to a specific service account.

## Section 2. Primary Provisioning Architecture

### Item A. Static Credential Flow

The primary provisioning architecture uses static API keys for Anthropic Claude authentication. Each consuming repository retrieves a repository specific `CLAUDE_API_KEY` value from HashiCorp Vault. The `internal/config.Load` function reads `CLAUDE_API_KEY` from the CI execution environment. The reviewer runtime initializes the Anthropic client using the retrieved credential.

### Item B. Key Management and Storage

The chosen path stores every manual `CLAUDE_API_KEY` in the central HashiCorp Vault instance. An operator creates each `CLAUDE_API_KEY` manually within the Claude Console. The Google Cloud provider supports automated key provisioning through the `google_apikeys_key` Terraform resource for `GEMINI_API_KEY`. The Anthropic platform does not provide equivalent automated key creation resources. Consequently, static Claude credentials require manual creation by an operator.

The cost of the chosen path comprises manual key generation overhead and credential persistence in Vault. Every key rotation requires manual intervention by an operator in the Claude Console.

## Section 3. Constraints on Automated Key Provisioning

### Item A. Admin API Limitations

The Anthropic Admin API restricts key operations on the `/v1/organizations/api_keys` endpoint. The endpoint supports list and update operations. The endpoint does not provide a create operation. Anthropic requires manual key generation through the Claude Console to protect administrative boundaries.

### Item B. Provider Automation Constraints

Third party Terraform providers do not possess a functional API key creation mechanism. The `anthropic_api_key` resource documentation in the `terraform-mars/anthropic` provider declares a creation capability. The upstream Admin API does not support programmatic key generation. Consequently, the declared creation capability in the provider cannot function against the live API. Callers MUST NOT rely on third party Terraform providers for automated `CLAUDE_API_KEY` creation.

## Section 4. Workload Identity Federation Architecture

### Item A. Token Exchange Mechanism

Workload Identity Federation eliminates persistent credentials from CI pipeline configuration. The `claude-review` job obtains a signed JSON Web Token from GitLab CI at runtime. The job exchanges the signed JSON Web Token directly for a short lived Anthropic access token.

The `.gitlab-ci.yml` configuration requests the identity token through the `id_tokens` keyword.

```yaml
job_with_id_tokens:
    id_tokens:
        ANTHROPIC_ID_TOKEN:
            aud: https://api.anthropic.com
    script:
        - claude-review
```

The `aud` claim in the requested token MUST equal `https://api.anthropic.com`. GitLab serves the public keys for signature verification at `https://gitlab.com/-/jwks`. Anthropic verifies the token signature against the public keys of the issuer. The GitLab token provides the dedicated `project_path` claim for repository identification.

### Item B. Anthropic Resource Structure

Workload Identity Federation requires three resources within the Anthropic platform. The federation issuer defines the OpenID Connect endpoint for `https://gitlab.com`. A service account defines the nonhuman identity which executes review operations. A federation rule binds the `project_path` claim of the incoming token to the service account.

The Anthropic Admin API supports programmatic management for federation issuers, service accounts, and federation rules. Administrators MAY provision these three federation resources through custom API scripts.

### Item C. SDK Compatibility

The reviewer application declares dependencies within `tools/ci/go.mod`. The `tools/ci/go.mod` file pins `github.com/anthropics/anthropic-sdk-go` at version `v1.48.0`. The Anthropic Go SDK introduced the `option.WithFederationTokenProvider` option in version `v1.39.0`. The currently pinned SDK version supports token exchange without a dependency upgrade.

## Section 5. Implementation Requirements and Costs

### Item A. Tradeoff Analysis

The current architecture SHOULD retain static `CLAUDE_API_KEY` provisioning for the initial deployment scale of seven repositories. Static provisioning limits maintenance overhead to existing Vault infrastructure.

Workload Identity Federation becomes preferable when deployment scale increases. Workload Identity Federation removes static secrets from CI variables, Terraform state, and Vault storage. The cost of adopting Workload Identity Federation comprises Go codebase refactoring and custom API resource automation.

### Item B. Integration Steps

1. **Register Federation Issuer**: The administrator MUST register a GitLab CI federation issuer for `https://gitlab.com` through the Custom OIDC workflow.
2. **Configure Service Accounts and Rules**: The administrator MUST provision a service account and a federation rule for each consuming repository requiring isolated cost attribution.
3. **Update CI Pipeline Configuration**: The maintainer MUST add an `id_tokens` definition with audience `https://api.anthropic.com` to the review job in `templates/core.yml`.
4. **Update Reviewer Client Implementation**: The maintainer MUST modify `internal/config.Load` and `internal/claude/client.go` to initialize the client from federation credentials.
