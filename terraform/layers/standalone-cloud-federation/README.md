# standalone-cloud-federation

## Section 1. Purpose

Standalone Cloud Federation is an infrastructure provisioning layer which deploys the prerequisite organization-level identity trust structures across Anthropic, Google Cloud Platform, and Microsoft Azure OpenAI. This layer functions as a standalone configuration path for establishing AI provider environments.

An operator deploying `gitlab-ci-with-code-reviewer` independently MUST execute this layer or provide equivalent cloud infrastructure before provisioning project-level workload identity federation bindings.

## Section 2. Interface

### Item A. Usage

```bash
cd terraform/layers/standalone-cloud-federation
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with target cloud identifiers
terraform init
terraform apply
```

To enable or disable specific cloud providers, set the `enabled` attribute on the corresponding object variable in `terraform.tfvars`:

```hcl
anthropic = {
    enabled = true
}

google = {
    enabled    = true
    project_id = "target-project-id"
}

azure = {
    enabled = false
}
```

### Item B. Inputs

The layer defines three typed object variables:

#### Item B.1. Variable `anthropic`

| Field         | Type               | Default                | Description                                 |
| ------------- | ------------------ | ---------------------- | ------------------------------------------- |
| `enabled`     | `optional(bool)`   | `false`                | Toggles deployment of Anthropic resources.  |
| `issuer_name` | `optional(string)` | `"issuer-gitlab-saas"` | Name of the Anthropic OIDC issuer resource. |
| `issuer_url`  | `optional(string)` | `"https://gitlab.com"` | Issuer URL representing GitLab SaaS.        |

#### Item B.2. Variable `google`

| Field                    | Type               | Default                  | Description                                      |
| ------------------------ | ------------------ | ------------------------ | ------------------------------------------------ |
| `enabled`                | `optional(bool)`   | `false`                  | Toggles deployment of Google Cloud resources.    |
| `project_id`             | `optional(string)` | `null`                   | Target Google Cloud project identifier.          |
| `pool_id`                | `optional(string)` | `"gitlab-saas-pool"`     | Target Workload Identity Pool ID.                |
| `provider_id`            | `optional(string)` | `"gitlab-saas-provider"` | Target Workload Identity Pool Provider ID.       |
| `allowed_namespace_path` | `optional(string)` | `"csning1998-lab"`       | Top-level GitLab group path allowed to federate. |

#### Item B.3. Variable `azure`

| Field                    | Type               | Default                  | Description                            |
| ------------------------ | ------------------ | ------------------------ | -------------------------------------- |
| `enabled`                | `optional(bool)`   | `false`                  | Toggles deployment of Azure resources. |
| `tenant_id`              | `optional(string)` | `null`                   | Microsoft Entra ID tenant UUID.        |
| `subscription_id`        | `optional(string)` | `null`                   | Target Azure subscription UUID.        |
| `resource_group_name`    | `optional(string)` | `"rg-federation-openai"` | Target Azure Resource Group name.      |
| `location`               | `optional(string)` | `"eastus"`               | Datacenter region for Azure OpenAI.    |
| `cognitive_account_name` | `optional(string)` | `"oai-csning1998-lab"`   | Name of the Cognitive Account.         |
| `model_name`             | `optional(string)` | `"gpt-4o"`               | Deployed OpenAI model identifier.      |
| `model_version`          | `optional(string)` | `"2024-05-13"`           | Deployed OpenAI model version string.  |

### Item C. Outputs

The layer publishes the following outputs:

- `anthropic`: Object containing `organization_id` and `issuer_id` when Anthropic is enabled.
- `google`: Object containing `project_id`, `project_number`, `pool_id`, and `provider_id` when Google Cloud is enabled.
- `azure`: Object containing `tenant_id`, `subscription_id`, `cognitive_account_id`, and `openai_endpoint` when Azure is enabled.

## Section 3. Provider Authentication and Pre-configuration Contracts

### Item A. Anthropic Claude Authentication

When `anthropic.enabled` is set to `true`, the operator executing this layer MUST configure the following credentials:

- `ANTHROPIC_ADMIN_API_KEY`: Organization administrator API key.
- `ANTHROPIC_AUTH_TOKEN`: OAuth access token acquired through `ant auth login` possessing `org:admin` scope.

The layer registers an organization-level OIDC issuer pointing to `https://gitlab.com`.

### Item B. Google Cloud Platform Authentication

When `google.enabled` is set to `true`, the operator executing this layer MUST configure Google Cloud Application Default Credentials:

```bash
gcloud auth application-default login
gcloud config set project <TARGET_GCP_PROJECT_ID>
```

The layer enables required APIs, provisions the Workload Identity Pool, and registers the Workload Identity Provider with attribute mapping for `assertion.project_path`.

### Item C. Microsoft Azure OpenAI Authentication

When `azure.enabled` is set to `true`, the operator executing this layer MUST authenticate using the Azure CLI:

```bash
az login
az account set --subscription <TARGET_SUBSCRIPTION_ID>
```

The operator account MUST possess `Application Administrator` permissions in Microsoft Entra ID and `User Access Administrator` (or `Owner`) permissions on the target subscription. The layer provisions the Resource Group, the Cognitive Services account, and the GPT-4o model deployment.

### Item D. Selective Provider Activation via Toggles

Setting `enabled = false` inside an object variable disables resource creation for that provider. Disabling a provider removes the requirement for the operator to supply credentials for that cloud platform during execution.
