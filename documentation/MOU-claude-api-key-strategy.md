# Memorandum of Understanding: Claude API Key Provisioning Strategy

## Section 1. Status

This document records an architectural decision that remains deferred. No implementation work described below has begun. This memorandum exists to preserve the analysis for future reference.

## Section 2. Current Architecture

Each repository consuming `gitlab-ci-with-code-reviewer` supplies its own `CLAUDE_API_KEY` and `GEMINI_API_KEY` CI/CD variable, read by `internal/config.Load()` and passed to the respective provider client. The Gemini keys are provisioned through Terraform in `csning1998-lab-meta-provision/layers/40-provider-api-keys`, using the `hashicorp/google` provider's `google_apikeys_key` resource, one key per repository, each restricted to `generativelanguage.googleapis.com` under the shared Google Cloud project `gen-lang-client-0531142873`. Claude keys have no equivalent automation; each must be created manually in the Claude Console.

## Section 3. Considered Alternative

An alternative design would replace static `CLAUDE_API_KEY` values entirely with Workload Identity Federation (WIF), whereby the `claude-review` CI job exchanges a short-lived GitLab CI OIDC identity token for a short-lived Anthropic access token at runtime, eliminating any static Anthropic credential from CI/CD variables, Terraform state, or Vault. This alternative was not implemented. It was raised as a question during a design discussion and is recorded here for future evaluation.

## Section 4. Feasibility Findings

### Task A. Automated Key Creation Is Not Available

The Anthropic Admin API's `/v1/organizations/api_keys` endpoint supports only `list` and `update` (rename, activate, deactivate); it has no `create` operation. Anthropic's own documentation states this explicitly: "No, new API keys can only be created through the Claude Console for security reasons. The Admin API can only manage existing API keys." This holds regardless of client (`curl`, an SDK, or any Terraform provider); the `terraform-mars/anthropic` provider's `anthropic_api_key` resource claims a create capability in its own documentation, but that claim cannot be true against the real API and MUST NOT be relied upon.

### Task B. Workload Identity Federation Is a Viable Replacement, Not a Workaround

WIF removes the need to create per-repository static keys at all, rather than automating their creation. It requires three Anthropic-side resources: a federation issuer (the OIDC provider), a service account (the non-human principal), and a federation rule (the match condition binding a JWT to a service account). Unlike API keys, all three ARE programmatically manageable through the Admin API ("create issuers, service accounts, and rules from infrastructure as code"), though no Terraform provider found during this investigation exposes them as resources; a direct Admin API caller (`curl`, a Go script, or Terraform's generic `http` provider) would be required.

### Task C. GitLab CI Is a Compatible OIDC Provider

GitLab.com's OIDC issuer is `https://gitlab.com`, with JWKS served at `https://gitlab.com/-/jwks` via standard discovery, satisfying Anthropic's federation issuer requirements (HTTPS, port 443, public DNS hostname). GitLab is not one of Anthropic's five preset provider tiles (GitHub Actions, AWS, Google Cloud, Microsoft Entra ID, Kubernetes) and would be configured through the "Custom OIDC" path. GitLab CI's ID token exposes `project_path` as a dedicated claim (e.g. `csning1998-lab/personal/second-brain`), which maps one federation rule to one repository more directly than GitHub Actions' `sub`-string parsing.

The `.gitlab-ci.yml` syntax for requesting the token:

```yaml
job_with_id_tokens:
  id_tokens:
    ANTHROPIC_ID_TOKEN:
      aud: https://api.anthropic.com
  script:
    - claude-review
```

### Task D. No SDK Version Bump Is Required

`tools/ci/go.mod` currently pins `github.com/anthropics/anthropic-sdk-go v1.48.0`. WIF support (`option.WithFederationTokenProvider`) shipped in v1.39.0 (2026-05-04), so the currently pinned version already supports it.

### Task E. Implementation Cost

Adopting WIF requires: registering one GitLab CI federation issuer (one-time, not per-repository); creating one service account and one federation rule per repository requiring isolated cost/usage attribution; adding `id_tokens` to the `claude-code-review` job in `templates/core.yml`; and replacing the static `CLAUDE_API_KEY` read in `internal/config.Load()` and `internal/claude/client.go` with the SDK's federation credential construction. This is a change to the reviewer's Go code and CI template, not confined to the Terraform layer that provisions secrets.

## Section 5. Recommendation

Static `CLAUDE_API_KEY` provisioning SHOULD be retained for the present scale (seven repositories), using manually created Console keys stored in the shared Vault instance described in `csning1998-lab-meta-provision`. Workload Identity Federation is technically feasible and architecturally preferable at larger scale, since it removes static Anthropic credentials from CI/CD variables, Terraform state, and Vault entirely, but its implementation cost spans both the Terraform layer and the reviewer's Go codebase and is out of scope for the current provisioning work.

## Section 6. Prerequisites Before Implementation

Should this alternative be pursued in the future, the following MUST be completed first.

1. Register a GitLab CI federation issuer in the Claude Console (or via the Admin API) using the Custom OIDC path.
2. Decide the service account and federation rule granularity: one service account per repository (preserving today's per-repository cost isolation) versus one shared service account with per-repository Workspace routing.
3. Determine how federation issuers, service accounts, and federation rules will be created as infrastructure as code, since no Terraform provider was found to expose them as first-class resources.
4. Update `templates/core.yml` to request an `id_tokens` claim with `aud: https://api.anthropic.com` on the `claude-code-review` job.
5. Update `internal/config.Load()` and `internal/claude/client.go` to construct the Anthropic client from federation credentials instead of a static `CLAUDE_API_KEY`.
