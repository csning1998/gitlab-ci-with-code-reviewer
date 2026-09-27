# Multi-Cloud Workload Identity Federation and AI Model Integration Engineering Reference

## Section 1. Architectural Scope and Terminology

### Item A. Scope

This document specifies technical constraints, operational failure modes, and coding patterns across multi-cloud Workload Identity Federation (WIF) and large language model (LLM) client integrations within `gitlab-ci-with-code-reviewer` and its supporting infrastructure modules. Every client implementation, infrastructure definition, and pipeline configuration MUST conform to the clauses defined herein.

### Item B. Lexicon

Workload Identity Federation defines an authentication pattern wherein a workload exchanges an external OpenID Connect identity token for short-lived cloud credentials without persistent secret keys. A reasoning model denotes an artificial intelligence model whose inference pipeline enforces server-side chain-of-thought processing and restricts sampling parameter manipulation. Token rate limit defines the quota mechanism enforcing maximum token throughput per minute across cloud provider endpoints. A moved block defines a Terraform language construct recording resource identity migration within state files.

## Section 2. Multi-Cloud Identity Mapping and Organization Bijection

### Item A. Organizational Hierarchy Mapping

The current constraint requires establishing equivalent security and administrative boundaries across disparate cloud resource models (Anthropic Organizations, Google Cloud Platform Projects, and Microsoft Azure Entra ID Tenants). Inconsistent administrative hierarchy prevents uniform least-privilege scoping across federated GitLab CI pipelines.

The chosen path enforces a bijective structural mapping across all cloud providers. The infrastructure architecture MUST align provider boundaries according to the following matrix:

- Anthropic: Organization -> Workspace -> Service Account -> API Key / Workload Identity.
- Google Cloud Platform: Organization -> Project -> Workload Identity Pool -> Service Account.
- Microsoft Azure: Entra ID Tenant -> Subscription -> Resource Group -> Entra ID Application -> Cognitive Services Account.

The associated cost requires configuring explicit identity federation mappings across three distinct cloud control planes.

### Item B. Minimizing Static Credentials via Cloud Data Sources

The current constraint risks credential leakage and configuration drift when operators manually paste cloud identifiers (such as tenant IDs, subscription IDs, and project numbers) into variable files.

The chosen path extracts existing cloud hierarchy metadata dynamically using read-only Terraform data sources. Infrastructure layers MUST read active context through data sources including `data.azuread_client_config.current`, `data.azurerm_client_config.current`, and `data.google_project.current`. Manual static entry of existing cloud metadata into variable declarations is prohibited.

The associated cost introduces runtime dependencies on provider read-only API availability during plan operations.

## Section 3. Terraform Configuration, State Management, and Resource Naming

### Item A. Resource Renaming and Moved Block Requirements

The current constraint causes Terraform to interpret an in-place resource block rename as the destruction of the existing resource followed by the creation of a new resource. Renaming a parent resource group resource block without a migration declaration triggers recursive deletion of the resource group, which cascades to deleting nested Cognitive Services accounts and can block state operations in Azure Resource Manager.

The chosen path pairs every resource address modification with a declarative `moved` block. Infrastructure configurations MUST declare `moved { from = <old_address> to = <new_address> }` whenever an existing resource address is modified. Applying an uncoordinated resource rename that triggers destructive recreation of stateful infrastructure is prohibited.

The associated cost requires maintaining historical `moved` blocks within configuration repositories until all environments execute the state transition.

### Item B. Variable File Separation and Sanitization

The current constraint causes credential leakage or interface ambiguity when public template files, variable declarations, and private runtime values are conflated.

The chosen path enforces a strict three-tier variable separation model:

- `variables.tf`: Contains only variable schema declarations, type constraints, and descriptions. Private infrastructure values MUST NOT be declared as defaults within `variables.tf`.
- `terraform.tfvars.example`: Public sanitization template versioned in source control. Sensitive values MUST be sanitized with placeholder text.
- `terraform.tfvars`: Private runtime values utilized on local operator workstations. This file MUST be excluded from version control via `.gitignore`.
- Leading blank line: In accordance with global repository conventions, the first line of `variables.tf` MUST remain an empty line.

The associated cost requires operators to copy and populate `terraform.tfvars` manually during initial workspace bootstrapping.

### Item C. Semantic Resource Naming Disciplines

The current constraint blocks Anthropic, Google Cloud Platform, and Microsoft Azure when the three providers share one Terraform state and any one provider credential is absent.

The chosen path assigns each provider a separate organization trust layer. The layer name MUST be `group-federation-anthropic`, `group-federation-gcp`, or `group-federation-azure`.

The associated cost requires a separate state file and a separate apply for each provider.

## Section 4. Cloud Workload Identity Federation Implementation Constraints

### Item A. Google Cloud Platform WIF Handshake

The current constraint prevents GitLab CI OpenID Connect JSON Web Tokens (`CI_JOB_JWT_V2` / `id_tokens`) from authenticating directly against Google Cloud Vertex AI APIs.

The chosen path implements a two-step token exchange protocol in `tools/ci/internal/tokensource/gcp_wif.go`:

1. The client sends the GitLab OIDC JWT to the Google Security Token Service (STS) endpoint (`https://sts.googleapis.com/v1/token`) with grant type `urn:ietf:params:oauth:grant-type:token-exchange` and audience `//iam.googleapis.com/projects/{PROJECT_NUMBER}/locations/global/workloadIdentityPools/{POOL_ID}/providers/{PROVIDER_ID}`.
2. The client exchanges the resulting federated token with the Google IAM Credentials API (`https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/{EMAIL}:generateAccessToken`) requesting the `https://www.googleapis.com/auth/cloud-platform` OAuth2 scope.

The service account MUST grant `roles/iam.workloadIdentityUser` to the principal set `principalSet://iam.googleapis.com/projects/{PROJECT_NUMBER}/locations/global/workloadIdentityPools/{POOL_ID}/attribute.project_path/{PROJECT_PATH}`.

The associated cost requires two sequential network roundtrips to obtain an operational access token.

### Item B. Microsoft Entra ID WIF Subject Matching

The current constraint causes Microsoft Entra ID token issuance to fail with HTTP 401 `AADSTS700213: No matching federated identity record found for presented assertion` when a pipeline runs on a feature branch but the federated identity credential restricts the subject claim strictly to `ref:main`.

The chosen path defines federated identity credentials supporting branch wildcarding or multi-subject lists in the Workload Identity Federation Terraform module. The module MUST declare `subjects = optional(list(string))` to permit simultaneous federation across `main`, protected branches, and merge request ref paths (`project_path:{group}/{project}:ref_type:branch:ref:*`). Redundant variable declarations between `subject` and `subjects` are prohibited.

The client exchanges the token at `https://login.microsoftonline.com/{tenant_id}/oauth2/v2.0/token` using `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`, `grant_type=client_credentials`, and scope `https://cognitiveservices.azure.com/.default`.

The associated cost expands the trust boundary of the federated Entra ID application to designated non-main repository branches.

### Item C. Required Cloud IAM Roles and Resource Provider Registrations

The current constraint blocks API execution if requisite resource providers or IAM permissions remain unregistered.

The chosen path mandates explicit registration of required cloud services before provisioning client credentials:

- Google Cloud Platform: The project MUST enable `aiplatform.googleapis.com`, `generativelanguage.googleapis.com`, `iam.googleapis.com`, `iamcredentials.googleapis.com`, and `sts.googleapis.com`.
- Microsoft Azure: The subscription MUST register `Microsoft.CognitiveServices`. Infrastructure automation MUST verify registration state (`Registered`) before provisioning cognitive accounts.
- Azure IAM: The federated application service principal MUST hold `Cognitive Services OpenAI User` on the provisioned Cognitive Services account.

The associated cost introduces subscription-level administrative prerequisites prior to workspace provisioning.

## Section 5. Upstream Large Language Model Service and Gateway Pitfalls

### Item A. Google Vertex AI Publisher Endpoint Routing

The current constraint causes Google Vertex AI to return HTTP 404 for Gemini 3.x models (`gemini-3.7-flash`) when invoked through regional publisher endpoints such as `us-central1`.

The chosen path constructs the Vertex AI invocation URL using the `locations/global` path component for all Gemini 3.x requests in `tools/ci/internal/providers/gemini/client.go`. The client runtime MUST target `https://aiplatform.googleapis.com/v1/projects/{PROJECT}/locations/global/publishers/google/models/{MODEL}:generateContent`.

The associated cost routes inference requests through Google Cloud global traffic management rather than a fixed regional facility.

### Item B. Azure OpenAI Deployment Identifier Requirement

The current constraint causes Azure OpenAI endpoints to return HTTP 404 `DeploymentNotFound` when client requests specify naked model names rather than provisioned deployment names.

The chosen path routes inference requests to `{endpoint}/openai/deployments/{deployment_id}/chat/completions?api-version={api-version}`. The deployment identifier declared in client configuration MUST match an active `azurerm_cognitive_deployment` resource name.

The associated cost requires declarative deployment provisioning in Terraform prior to executing client requests.

### Item C. Azure Quota Discovery and Regional Capacity Allocation

The current constraint causes deployment provisioning to fail with HTTP 400 `InsufficientQuota` when configuring models without allocated subscription quota in the target region (for example, `gpt-5.6-terra` possessing default quota limit 0 in `eastus`).

The chosen path queries regional quota availability via `az cognitiveservices usage list --location <region>` before writing Terraform declarations. Infrastructure configurations MUST declare cognitive deployments exclusively for models possessing positive available quota.

The associated cost limits model selection to verified subscription quota allocations.

### Item D. Azure SKU Selection and Gateway Throttling

The current constraint exposes `GlobalStandard` deployments to dynamic protective throttling and gateway caching lag, resulting in rate limit rejection even after ARM capacity updates.

The chosen path configures production deployments with `DataZoneStandard` SKU (for example, `gpt-5.4-mini` version `2026-03-17` with SKU `DataZoneStandard` and capacity 100). The deployment definition in Terraform infrastructure layers MUST declare `sku_name = "DataZoneStandard"` when stable dedicated quota is required.

The associated cost restricts deployment availability to data zones supporting DataZoneStandard allocations.

### Item E. Token Rate Limit Reservation and Payload Sizing

The current constraint triggers HTTP 429 `rate_limit_exceeded` when deployment capacity is provisioned at 10 (10,000 TPM) for review jobs sending code review diffs (~77,000 characters / ~20,000 to ~25,000 prompt tokens). Azure OpenAI calculates upfront rate limit consumption by summing prompt tokens with `max_completion_tokens` (or system default reservation), requiring over 57,000 tokens of available capacity per request.

The chosen path dimensions cognitive deployment capacity to at least 100 (100,000 TPM) for automated code review pipelines. Infrastructure declarations for code reviewer deployments MUST allocate `capacity >= 100`.

The associated cost increases subscription quota consumption within the hosting region.

### Item F. Upstream Request Parameter Sanitization

The current constraint causes reasoning models (`gpt-5` series, `o1`, `o3`, `o4`, and Claude thinking models) to reject non-default sampling parameters with HTTP 400 `unsupported_value: 'temperature' does not support 0 with this model`.

The chosen path inspects model capabilities within the client payload builder and strips unsupported parameters before request dispatch. When targeting reasoning models, the client builder MUST drop `temperature`, `top_p`, `presence_penalty`, and `frequency_penalty`. The client MUST write an explicit warning notification to standard error when parameter stripping occurs.

The associated cost requires maintaining capability lookup tables inside client provider modules.

### Item G. Compiled Model Allow List

The current constraint causes configuration parsing to fail when a reviewer configuration names a model identifier absent from the compiled allow list in `tools/ci/internal/config/registry.go`.

The chosen path stores the allow list in the compiled registry. A reviewer configuration MUST name a canonical identifier present in the allow list. An identifier which the vendor has retired MUST leave the allow list before the vendor retirement date. Addition or removal of an allow list identifier MUST be recorded in `decisions.md` before the image release.

The associated cost requires one decision record and one image release for each allow list change.

## Section 6. Go Client Implementation and Testing Standards

### Item A. Test Function Cognitive Complexity Management

The current constraint triggers linter rejections (`gocognit`) when test functions combine extensive setup, execution, and deep assertion branching into a single monolithic block.

The chosen path refactors complex test functions (such as `TestNew_ConfigurationValidation`) into modular, table-driven subtests utilizing declarative test fixtures. Test functions MUST maintain cognitive complexity scores beneath configured linter thresholds.

The associated cost increases total lines of test scaffolding.

### Item B. Hermetic Test Isolation and Environment Decoupling

The current constraint causes test suites to succeed on developer workstations but fail in GitLab CI pipelines due to implicit reliance on local credential caches, missing CI ID tokens, unmasked variables, or missing build artifacts.

The chosen path requires complete hermetic test isolation. Unit and integration tests MUST utilize dependency injection, mock HTTP transport layers, or verified ephemeral test credentials. Tests MUST NOT depend on active developer workstation session state.

The associated cost requires implementing mock servers and simulated token responders for all cloud provider interactions.

## Section 7. GitLab CI Pipeline Configuration

### Item A. GitLab CI OIDC Token Declaration

The current constraint prevents CI jobs from executing cloud WIF handshakes when the job definition fails to emit an OpenID Connect token with the designated audience.

The chosen path configures `id_tokens` in `.gitlab-ci.yml` and `.gitlab/reviewer.yml`. Jobs interacting with cloud federation MUST declare:

```yaml
id_tokens:
    GITLAB_OIDC_TOKEN:
        aud: <configured_cloud_audience>
```

The audience string declared in CI configuration MUST match the audience configured in the target cloud provider identity pool or application.

The associated cost ties CI job definitions directly to cloud provider federation schemas.
