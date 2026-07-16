# GitLab Merge Request Reviewer

## Section 1. Obtain API Key from AI Provider(s)

### Option 1. Google AI Studio Setup Process

The primary objective is to acquire model credentials and verify access permissions on the Google AI Studio platform.

1. **Create an API Key**:
    - Log in to Google AI Studio.
    - Navigate to and select **Get API key** in the sidebar or navigation menu.
    - Select **Create API key** and choose the appropriate Google Cloud project.
    - Copy the generated API key and store it securely.

2. **Verify Model Quota and Availability**:
    - Verify model access by checking that the designated model (e.g., `gemini-3.5-flash`) is available in the model selection menu.
    - Ensure that the model quota for the associated region and project is active (non-zero).

### Option 2. Anthropic Console Setup Process

The primary objective is to acquire API credentials for Claude models on the Anthropic Console.

1. **Create an API Key**:
    - Log in to the Anthropic Console.
    - Navigate to **API Keys** and select **Create Key**.
    - Copy the generated key string and store it securely. The key will not be displayed again.

2. **Verify Model Access**:
    - Verify that the account has access to the designated model (e.g., `claude-sonnet-4-6`) and that the usage tier allows API calls.

## Section 2. GitLab Setup Process

The setup workflow on GitLab consists of two parts:

1. Generating access tokens to authorize the pipeline to read Merge Request (MR) diffs and post review discussions.
2. Registering the credentials as project CI/CD variables.

### Step A. Generate a GitLab Access Token

This token authorizes the pipeline binary to retrieve MR diffs and write inline discussion comments.

- Navigate to the GitLab project and select **Settings > Access Tokens** (or **User Settings > Access Tokens** for a user-scoped personal access token).
- Select **Add new token**, configure the name, and specify an expiration date.
- Under **Select scopes**, enable **`api`** (Classic Personal Access Token (PAT) with full API access is required; fine-grained PATs do not expose the MR Create permission needed to post inline discussion threads).
- Copy the generated token string immediately upon creation, as it will not be displayed again.

### Step B. Add Reviewer Variables at Settings > Access Tokens

Navigate to **Settings > Access Tokens** and create project access tokens with the **Reporter** role and the **`api`** and **`read_api`** scopes.

| Variable             | Role            |
| -------------------- | --------------- |
| `GEMINI_MR_REVIEWER` | GitLab Reviewer |
| `CLAUDE_MR_REVIEWER` | GitLab Reviewer |

Copy each generated token string immediately. The values are non-retrievable after leaving the page.

Ensure that the active scopes for the tokens are strictly restricted to **`api`** and **`read_api`**. Tokens generated through other integrations or containing additional scopes (such as `mcp` or `ai_workflows`) may trigger a `403 Forbidden` API error during pipeline execution.

### Step C. Configure CI/CD Environment Variables at Settings > CI/CD

Ensure that the API keys from **Section 1** are obtained before configuring these variables.

Navigate to **Settings > CI/CD**, expand **Variables**, and select **Add variable** for each entry in the table below.

Configure all variables with the following settings:

- **Mask variable (Recommand)** and **Hidden variable**: Enabled to prevent credentials from being exposed in pipeline logs.
- **Protect variable**: Disabled to allow variables to be accessed by pipelines running on unprotected feature branches.

| Variable             | Value                                                |
| -------------------- | ---------------------------------------------------- |
| `GEMINI_MR_REVIEWER` | Token string generated at Step B                     |
| `CLAUDE_MR_REVIEWER` | Token string generated at Step B                     |
| `GEMINI_API_KEY`     | API key from Google AI Studio (Section 3, Option 1)  |
| `CLAUDE_API_KEY`     | API key from Anthropic Console (Section 3, Option 2) |

## Section 3. Runner Setup

The project runner is provisioned via Terraform, which registers the runner with GitLab and writes the configuration to the host.

### Step A. Generate Terraform Management Token

Navigate to the GitLab user avatar and select **User Settings > Access Tokens**.

- Select **Add new token**, and configure a name and expiration date.
- Under **Select scopes**, enable the **`api`** scope.
- Select **Create personal access token** and copy the generated string. The PAT will not be displayed again.

The token owner must possess the **Owner** role on the target GitLab project. This permission is default for projects within a personal namespace.

This token is referenced as `password` in `backend.hcl` and as `gitlab_token` in `terraform.tfvars`.

### Step B. Prerequisites

- Terraform >= 1.8.0
- Podman and `podman-compose` installed on the host
- Rootless Podman socket active at `/run/user/<HOST_UID>/podman/podman.sock`

### Step C. Configure Terraform Files

Create the following configurations manually in the `terraform/` directory. Refer to `terraform/terraform.tfvars.example` for the template.

- For **`terraform/backend.hcl`**: Stores the Terraform HTTP backend credentials for remote state.
    - **`address`**: `https://gitlab.com/api/v4/projects/<PROJECT_ID>/terraform/state/default`
    - **`lock_address`**: `https://gitlab.com/api/v4/projects/<PROJECT_ID>/terraform/state/default/lock`
    - **`unlock_address`**: `https://gitlab.com/api/v4/projects/<PROJECT_ID>/terraform/state/default/lock`
    - **`username`**: `oauth2`
    - **`password`**: The User PAT generated in Step A.
    - **`lock_method`**: `POST`
    - **`unlock_method`**: `DELETE`
    - **`retry_wait_min`**: `5`

    The `<PROJECT_ID>` value is located under **Settings > General** on the GitLab project page.

- For **`terraform/terraform.tfvars`**:
    - **`gitlab_token`**: The User PAT generated in Step A.
    - **`runner_description`**: Display name shown in **Settings > CI/CD > Runners** (default: `local-podman-runner`).
    - **`runner_tag_list`**: Tag list for explicit job targeting (default: `["podman", "local"]`).

### Step D. Provision with Terraform

1. **Initialize the Backend**: Execute initialization using the `-backend-config` flag to load the gitignored `backend.hcl` file, which contains the remote state address and credentials.

    ```bash
    terraform init -backend-config=backend.hcl
    ```

2. **Import the Existing Project**: Import the target GitLab project into the Terraform state. This operation is required during the initial execution because the project already exists; omitting this step will cause the subsequent apply execution to attempt a duplicate project creation.

    ```bash
    terraform import gitlab_project.this <PROJECT_ID>
    ```

3. **Apply the Configuration**: Apply the Terraform configuration to register the project runner on GitLab. This step automatically writes the generated runner token to `~/.config/gitlab-runner/config.toml` on the host machine.

    ```bash
    terraform apply -auto-approve
    ```

### Step E. Start the Runner Service

1. **Configure Environment Variables**: Copy `.env.example` to `.env` and configure the following variables:
    - **`UHOME`**: The absolute path to the user home directory (retrieve via `echo $HOME`).
    - **`HOST_UID`**: The numeric UID of the host user (retrieve via `id -u`), which is required to resolve the rootless Podman socket at `/run/user/<HOST_UID>/podman/podman.sock`.

    ```bash
    cp .env.example .env
    ```

2. **Start the Runner Container**: Start the runner service in the background using Podman Compose:

    ```bash
    podman compose up -d
    ```

## Section 4. Triggering Workflow

After setup is complete, code reviews integrate into the standard development workflow.

1. **Create an Merge Request**:
    - Push the development branch and open an MR targeting the default branch.

2. **Trigger a Review Manually**:
    - After the MR is created, a pipeline will appear under the **Pipelines** tab.
    - The review jobs (`gemini-code-review` and `claude-code-review`) appear based on model configurations in the `core` component (refer to Section 5, Step C). Jobs are paused by default.
    - Selecting the manual play trigger executes the review binary against the MR diff and publishes comments to the discussion timeline.
    - If multiple model providers are configured, jobs can be executed independently.

## Section 5. Consuming the CI Template

This project is published as a GitLab CI/CD Catalog component. Consuming projects integrate these jobs via `include:component`, passing configuration options through `inputs` to avoid template customization or forking.

### Step A. Prerequisites in the Consuming Project

- Ensure the runner is registered and project-level variables (e.g. `GEMINI_MR_REVIEWER`, `CLAUDE_MR_REVIEWER`, `GEMINI_API_KEY`, `CLAUDE_API_KEY`) are configured. These are read dynamically at runtime.
- Authorize the `CI_JOB_TOKEN` to write to the repository under **Settings > CI/CD > Token Access** if jobs are required to auto-commit and push changes back to the source branch.

### Step B. Reference Components in `.gitlab-ci.yml`

Components are referenced using the path `gitlab.com/csning1998/gitlab-ci-with-code-reviewer/<component>@<version>`. The `<version>` must be pinned to a specific release tag. The `core` component is mandatory and requires the `reviewer_image` input to be explicitly defined.

```yaml
include:
    - component: gitlab.com/csning1998/gitlab-ci-with-code-reviewer/core@1.0.0
      inputs:
          reviewer_image: registry.gitlab.com/csning1998/gitlab-ci-with-code-reviewer/reviewer:1.0.0
    - component: gitlab.com/csning1998/gitlab-ci-with-code-reviewer/lang-go@1.0.0
```

### Step C. Inject Inputs for Project-Specific Differences

Configure inputs to override default settings. Users may verify and specify the latest released version number for the component and container image. The variables `claude_model` and `gemini_model` default to empty strings; configuring a model activates its corresponding review pipeline. Representative examples:

```yaml
include:
    - component: gitlab.com/csning1998/gitlab-ci-with-code-reviewer/core@1.0.0
      inputs:
          reviewer_image: registry.gitlab.com/csning1998/gitlab-ci-with-code-reviewer/reviewer:1.0.0
          claude_model: claude-sonnet-4-6
          gemini_model: gemini-3.5-flash
          model_k: model_v

    - component: gitlab.com/csning1998/gitlab-ci-with-code-reviewer/lang-typescript@1.0.0
      inputs:
          ts_globs: ['frontend/**/*', 'backend/**/*']
          frontend_dir: frontend
          backend_dir: backend

    - component: gitlab.com/csning1998/gitlab-ci-with-code-reviewer/iac-terraform@1.0.0
      inputs:
          checkov_skip: 'CKV_GIT_1,CKV_GLB_1,CKV_GLB_3,CKV_GLB_4,CKV_K8S_21'

    - component: gitlab.com/csning1998/gitlab-ci-with-code-reviewer/iac-ansible@1.0.0
```

Available components include `core`, `lang-go`, `lang-typescript`, `iac-terraform`, `iac-packer`, and `iac-ansible`. The full input schemas are defined in the respective files under `templates/`.

## Section 6. Replicating on a Self-Hosted GitLab Instance

The CI/CD Catalog is instance-scoped. Because a self-hosted instance cannot resolve or include components directly from the gitlab.com catalog, the template components must be replicated within the local instance.

1. **Mirror Repository**: Import the `gitlab-ci-with-code-reviewer` repository into the self-hosted instance and enable it as a CI/CD Catalog project under **Settings > General > Visibility, project features, permissions > CI/CD Catalog project**.
2. **Mirror Container Image**: Pull the container image `registry.gitlab.com/csning1998/gitlab-ci-with-code-reviewer/reviewer:<tag>` and push it into the local instance registry or Harbor. Pass this internal image path to the `reviewer_image` input of the `core` component.
3. **Update Component References**: Configure consuming projects on the self-hosted instance to reference the local component path `<instance-namespace>/gitlab-ci-with-code-reviewer/<component>@<version>` instead of the gitlab.com path.
4. **Publish Catalog Release**: Tag the mirrored project to publish its components to the instance catalog, replicating the release workflow documented in Section 7.

## Section 7. Versioning and Release

A unified Semantic Versioning (SemVer) tag aligns the container image releases with the catalog components, ensuring that a consumer configuring `reviewer_image` to `reviewer:X.Y.Z` matches `core@X.Y.Z`. The `core` component does not specify a default for `reviewer_image` to ensure consumers explicitly pin version numbers, thereby preventing configuration drift.

1. **Push Version Tag**: Push the SemVer tag. The `release` stage in `.gitlab-ci.yml` builds and pushes `reviewer:<tag>` (releasing only the pinned version tag with no `:latest` tag), then creates a GitLab release that publishes the components located under `templates/` to the catalog.
2. **Verify Deployment**: Confirm the generated image is publicly accessible under the project Container Registry and the components are displayed on the project's CI/CD Catalog page.

### Planned Language Path Space (Not Yet Implemented)

Two language components are reserved for future addition, following the same `core` component input conventions:

- `lang-python`: Code formatting and linting (via `ruff` or a combination of `black` and `flake8`), and type checking (via `mypy`).
- `lang-java`: Build execution (via `gradle` or `maven`), and formatting and linting checks (via `spotless` and `checkstyle`). The choice of build tool dictates the job structure and caching strategy.
