# GitLab Merge Request Reviewer

## Section 1. Obtain API Key from AI Provider(s)

### Option 1. Google AI Studio Setup Process

The primary objective entails acquiring model credentials and validating access permissions on the Google AI Studio platform.

1. **Create an API Key**:
    - Authenticate to Google AI Studio.
    - Navigate to and select **Get API key** within the sidebar or navigation menu.
    - Select **Create API key** and designate the target Google Cloud project.
    - Copy the generated API key string and retain it within secure storage.

2. **Verify Model Quota and Availability**:
    - Validate model access by confirming that the designated model (e.g., `gemini-3.5-flash`) is present within the model selection menu.
    - Ensure that the model quota allocated to the corresponding region and project remains active (non-zero).

### Option 2. Anthropic Console Setup Process

The primary objective entails acquiring API credentials for Claude models on the Anthropic Console.

1. **Create an API Key**:
    - Authenticate to the Anthropic Console.
    - Navigate to **API Keys** and select **Create Key**.
    - Copy the generated key string and retain it within secure storage. The credential string becomes unretrievable following initial display.

2. **Verify Model Access**:
    - Validate account authorization for the designated model (e.g., `claude-sonnet-4-6`) and verify that the assigned usage tier permits API invocation.

## Section 2. GitLab Setup Process

The setup workflow within GitLab comprises two discrete phases:

1. Generation of access tokens to authorize pipeline retrieval of Merge Request (MR) diffs and publication of review discussions.
2. Registration of credentials as project CI/CD environment variables.

### Step A. Generate a GitLab Access Token

This token authorizes the pipeline binary to retrieve MR diffs and execute the writing of inline discussion comments.

- Navigate to the target GitLab project and select **Settings > Access Tokens** (or **User Settings > Access Tokens** for a user-scoped personal access token).
- Select **Add new token**, configure the token name, and specify an expiration date.
- Under **Select scopes**, enable **`api`** (A Classic Personal Access Token (PAT) possessing full API access is required; fine-grained PATs do not expose the MR Create permission necessary to post inline discussion threads).
- Copy the generated token string immediately upon creation, as the value becomes unretrievable following initial display.

### Step B. Add Reviewer Variables at Settings > Access Tokens

Proceed to **Settings > Access Tokens** to generate project access tokens configured with the **Developer** role and the **`api`** and **`read_api`** scopes. The Reporter role provides insufficient permissions, as updating MR labels necessitates Developer privileges.

| Variable             | Role            |
| -------------------- | --------------- |
| `GEMINI_MR_REVIEWER` | GitLab Reviewer |
| `CLAUDE_MR_REVIEWER` | GitLab Reviewer |

Copy each generated token string immediately upon creation. The values are non-retrievable after navigating away from the page.

Ensure that the active scopes assigned to the tokens remain strictly restricted to **`api`** and **`read_api`**. Tokens generated via alternative integrations or possessing extraneous scopes (such as `mcp` or `ai_workflows`) risk triggering a `403 Forbidden` API error during pipeline execution.

### Step C. Configure CI/CD Environment Variables at Settings > CI/CD

Ensure acquisition of the API keys specified in **Section 1** prior to configuring these variables.

Navigate to **Settings > CI/CD**, expand the **Variables** block, and select **Add variable** for each entry enumerated in the following table.

Configure all variables according to the following parameters:

- **Mask variable (Recommended)** and **Hidden variable**: Enabled to prevent credential exposure within pipeline execution logs.
- **Protect variable**: Disabled to permit variable access for pipelines executing upon unprotected feature branches.

| Variable             | Value                                                |
| -------------------- | ---------------------------------------------------- |
| `GEMINI_MR_REVIEWER` | Token string generated at Step B                     |
| `CLAUDE_MR_REVIEWER` | Token string generated at Step B                     |
| `GEMINI_API_KEY`     | API key from Google AI Studio (Section 3, Option 1)  |
| `CLAUDE_API_KEY`     | API key from Anthropic Console (Section 3, Option 2) |

## Section 3. Runner Setup

Provisioning of the project runner occurs through Terraform, which registers the runner with GitLab and writes the resulting configuration to the host environment.

### Step A. Generate Terraform Management Token

Navigate to the GitLab user avatar and select **User Settings > Access Tokens**.

- Select **Add new token**, and configure a name and expiration date.
- Under **Select scopes**, enable the **`api`** scope.
- Select **Create personal access token** and copy the generated string. The PAT becomes unretrievable following initial display.

The token owner must possess the **Owner** role within the target GitLab project. This privilege is assigned by default to projects residing within personal namespaces.

This token is referenced as `password` within `backend.hcl` and as `gitlab_token` within `terraform.tfvars`.

### Step B. Prerequisites

- Terraform >= 1.8.0
- Podman and `podman-compose` installed on the host machine
- Rootless Podman socket active at `/run/user/<HOST_UID>/podman/podman.sock`

### Step C. Configure Terraform Files

Manually construct the following configurations within the `terraform/` directory, utilizing `terraform/terraform.tfvars.example` as a template reference.

- For **`terraform/backend.hcl`**: Maintains Terraform HTTP backend credentials for remote state storage.
    - **`address`**: `https://gitlab.com/api/v4/projects/<PROJECT_ID>/terraform/state/default`
    - **`lock_address`**: `https://gitlab.com/api/v4/projects/<PROJECT_ID>/terraform/state/default/lock`
    - **`unlock_address`**: `https://gitlab.com/api/v4/projects/<PROJECT_ID>/terraform/state/default/lock`
    - **`username`**: `oauth2`
    - **`password`**: The User PAT generated in Step A.
    - **`lock_method`**: `POST`
    - **`unlock_method`**: `DELETE`
    - **`retry_wait_min`**: `5`

    The `<PROJECT_ID>` value resides within **Settings > General** on the GitLab project page.

- For **`terraform/terraform.tfvars`**:
    - **`gitlab_token`**: The User PAT generated in Step A.
    - **`runner_description`**: Display name shown within **Settings > CI/CD > Runners** (default: `local-podman-runner`).
    - **`runner_tag_list`**: Tag list for explicit job targeting (default: `["podman", "local"]`).

### Step D. Provision with Terraform

1. **Initialize the Backend**: Execute initialization utilizing the `-backend-config` flag to load the gitignored `backend.hcl` file containing the remote state address and credentials.

    ```bash
    terraform init -backend-config=backend.hcl
    ```

2. **Import the Existing Project**: Import the target GitLab project into the Terraform state. This operation is required during initial execution because the project pre-exists; omitting this step causes subsequent apply executions to attempt duplicate project creation.

    ```bash
    terraform import gitlab_project.this <PROJECT_ID>
    ```

3. **Apply the Configuration**: Apply the Terraform configuration to register the project runner on GitLab. This operation automatically writes the generated runner token to `~/.config/gitlab-runner/config.toml` on the host machine.

    ```bash
    terraform apply -auto-approve
    ```

### Step E. Start the Runner Service

1. **Configure Environment Variables**: Copy `.env.example` to `.env` and specify parameters for the following variables:
    - **`UHOME`**: The absolute path to the user home directory (obtain via `echo $HOME`).
    - **`HOST_UID`**: The numeric UID of the host user (obtain via `id -u`), required for resolving the rootless Podman socket at `/run/user/<HOST_UID>/podman/podman.sock`.

    ```bash
    cp .env.example .env
    ```

2. **Start the Runner Container**: Initiate the runner service in the background utilizing Podman Compose:

    ```bash
    podman compose up -d
    ```

## Section 4. Triggering Workflow

Upon completion of the setup process, automated code review processes integrate into the standard development workflow.

1. **Create a Merge Request**:
    - Push the target feature branch and initiate an MR directed toward the default branch.

2. **Trigger a Review Manually**:
    - Following MR creation, a pipeline executes under the **Pipelines** tab.
    - The review jobs (`gemini-code-review` and `claude-code-review`) render based upon model configurations within the `core` component (refer to Section 5, Step C). These jobs remain paused by default.
    - Execution of the manual play trigger invokes the review binary against the MR diff and publishes inline comments to the discussion timeline.
    - When multiple model providers are configured, jobs operate independently.

## Section 5. Consuming the CI Template

This project is published as a GitLab CI/CD Catalog component. Consuming projects integrate these jobs via `include:component`, passing configuration parameters through `inputs` to eliminate the need for template customization or repository forking.

### Step A. Prerequisites in the Consuming Project

- Ensure that the runner is registered and project-level variables (e.g., `GEMINI_MR_REVIEWER`, `CLAUDE_MR_REVIEWER`, `GEMINI_API_KEY`, `CLAUDE_API_KEY`) are fully configured for dynamic evaluation at runtime.
- Authorize `CI_JOB_TOKEN` repository write permissions within **Settings > CI/CD > Token Access** should job execution necessitate automated commits and pushes back to the source branch.

### Step B. Reference Components in `.gitlab-ci.yml`

Components are referenced using the path syntax `gitlab.com/csning1998/gitlab-ci-with-code-reviewer/<component>@<version>`. The `<version>` value must be explicitly pinned to a designated release tag. Inclusion of the `core` component is mandatory and necessitates explicit definition of the `reviewer_image` input.

```yaml
include:
    - component: gitlab.com/csning1998/gitlab-ci-with-code-reviewer/core@1.0.0
      inputs:
          reviewer_image: registry.gitlab.com/csning1998/gitlab-ci-with-code-reviewer/reviewer:1.0.0
    - component: gitlab.com/csning1998/gitlab-ci-with-code-reviewer/lang-go@1.0.0
```

### Step C. Inject Inputs for Project-Specific Differences

Specify inputs to override default configurations. Operators should verify and specify the latest published version number for both the component and the container image. The `claude_model` and `gemini_model` variables default to empty strings; providing a model identifier activates the corresponding review pipeline. Representative implementation example:

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

Supported components comprise `core`, `lang-go`, `lang-typescript`, `iac-terraform`, `iac-packer`, and `iac-ansible`. Full input schemas are defined within the respective files under `templates/`.

The `core` component unconditionally executes a `gitleaks` secret-detection job on every Merge Request, alongside a `sonarqube-scan` job gated by the `enable_sonarqube` input (`type: boolean`, default `false`). Activation of `enable_sonarqube` requires a self-hosted SonarQube instance and the presence of `SONAR_HOST_URL` and `SONAR_TOKEN` CI/CD variables within the consuming project or an inherited group.

## Section 6. Replicating on a Self-Hosted GitLab Instance

The CI/CD Catalog operates on an instance-scoped architecture. Since self-hosted instances cannot resolve or include components directly from the gitlab.com catalog, template components must be replicated within the local instance environment.

1. **Mirror Repository**: Import the `gitlab-ci-with-code-reviewer` repository into the self-hosted instance and designate it as a CI/CD Catalog project within **Settings > General > Visibility, project features, permissions > CI/CD Catalog project**.
2. **Mirror Container Image**: Retrieve the container image `registry.gitlab.com/csning1998/gitlab-ci-with-code-reviewer/reviewer:<tag>` and publish it to the local instance registry or Harbor. Provide this internal image path to the `reviewer_image` input of the `core` component.
3. **Update Component References**: Configure consuming projects on the self-hosted instance to reference the local component path `<instance-namespace>/gitlab-ci-with-code-reviewer/<component>@<version>` rather than the gitlab.com path.
4. **Publish Catalog Release**: Apply a version tag to the mirrored project to publish its components to the instance catalog, thereby replicating the release workflow documented in Section 7.

## Section 7. Versioning and Release

A unified Semantic Versioning (SemVer) tag aligns container image releases with catalog components, guaranteeing that a consumer configuring `reviewer_image` to `reviewer:X.Y.Z` corresponds exactly to `core@X.Y.Z`. The `core` component deliberately omits a default value for `reviewer_image` to enforce explicit version pinning by consumers, thereby preventing configuration drift. Tags omit the `v` prefix, as neither the Container Registry nor the CI/CD Catalog mandates its inclusion; a `v` prefix pertains exclusively to projects whose internal tags are resolved through Go module versioning, a mechanism not utilized within this architecture.

1. **Automatic Version Tag**: The `auto-tag` job defined within `.gitlab-ci.yml` executes upon every push to `main`, deriving the subsequent version from the squash-merge commit subject via `mr-semver-resolver` and pushing the generated tag.
    - A commit subject bearing the `feat` type produces a minor version bump.
    - `fix` and `perf` types yield patch bumps.
    - An exclamation mark (`!`) appended to the type or scope triggers a major bump.
    - All remaining types yield no tag creation.

    Pushing this tag necessitates the `TAG_PUSH_TOKEN` CI/CD variable, configured as a Project Access Token assigned the `Developer` role and the `write_repository` scope. The default `CI_JOB_TOKEN` cannot initiate the downstream tag pipeline responsible for publishing the release, as GitLab excludes `CI_JOB_TOKEN`-authenticated pushes from triggering secondary pipelines to prevent infinite execution loops.

2. **Tag Pipeline**: Publication of the tag initiates an independent pipeline. The `release` stage within `.gitlab-ci.yml` builds and pushes `reviewer:<tag>` (releasing exclusively the pinned version tag without a `:latest` tag), subsequently generating a GitLab release that publishes the components located within `templates/` to the catalog.
3. **Verify Deployment**: Verify that the generated image is publicly accessible within the project Container Registry and that all components render correctly on the project CI/CD Catalog page.

### Planned Language Path Space (Not Yet Implemented)

Two additional language components remain reserved for future implementation, adhering to the input conventions established by the `core` component:

- `lang-python`: Code formatting and linting (executed via `ruff` or a combination of `black` and `flake8`), alongside static type checking (executed via `mypy`).
- `lang-java`: Build execution (executed via `gradle` or `maven`), together with formatting and linting validation (executed via `spotless` and `checkstyle`). The selection of the build tool dictates the structural design and caching strategy of the job.
