# GitLab Merge Request Reviewer

## Section 1. Obtain API Key from AI Provider(s)

### Option 1. Google AI Studio Setup Process

The primary objective on the Google platform is to acquire model access credentials and verify model access permissions.

1. **Create an API Key**:
    - Log in to the Google AI Studio platform.
    - Click **Get API key** in the upper-left corner or navigation bar.
    - Select **Create API key** and choose the Google Cloud project associated with the account.
    - Once the system generates the key, copy the string completely and store it securely.

2. **Verify Model Quota and Availability**:
    - In the model selection list on the right side of the platform, verify that the account is currently able to call the designated model under the free tier (e.g., `gemini-3.5-flash`).
    - Ensure that the model quota is not restricted within the region and project (the limit must be greater than `0`).

### Option 2. Anthropic Console Setup Process

The primary objective on the Anthropic platform is to acquire API credentials for Claude model access.

1. **Create an API Key**:
    - Log in to the Anthropic Console.
    - Navigate to **API Keys** and click **Create Key**.
    - Copy the generated key string immediately and store it securely (it will not be displayed again).

2. **Verify Model Access**:
    - Confirm that the account has access to the designated model (e.g., `claude-sonnet-4-6`) and that the usage tier permits API calls.

## Section 2. GitLab Setup Process

The workflow on the GitLab platform consists of two parts:

1. Generating one or more access tokens that permit the pipeline to read MR changes and post review comments.
2. Injecting all credentials into the project CI/CD environment variables.

### Step A. Generate a GitLab Access Token

This token enables the Go binary in the pipeline to read Merge Request (MR) diffs and post inline discussion threads.

- Navigate to the GitLab project page and select **Settings > Access Tokens** (alternatively, click the user avatar to access **User Settings > Access Tokens** for a user-scoped token).
- Click **Add new token**, configure the token name, and select an expiration date.
- Under **Select scopes**, check **`api`** (Classic PAT with full API access is required; fine-grained PATs do not expose the Merge Request Create permission needed to post inline discussion threads).
- After clicking create, **the generated token string must be copied immediately**, it will not be displayed again after leaving the page.

### Step B. Add Reviewer Variables at Settings > Access Tokens

Navigate to **Settings > Access Tokens** and create the following project access tokens. Set the role to **Reporter** and check the **`api`** and **`read_api`** scopes.

| Variable             | Role            |
| -------------------- | --------------- |
| `GEMINI_MR_REVIEWER` | GitLab Reviewer |
| `CLAUDE_MR_REVIEWER` | GitLab Reviewer |

Copy each generated token string immediately. It will not be displayed again after leaving the page.

After creation, verify that the token's active scopes show only **`api`** and **`read_api`**. Tokens auto-generated through GitLab AI features or MCP integrations carry additional scopes (`mcp`, `ai_workflows`) that restrict the privilege level and will cause a `403 insufficient_scope` error when the pipeline calls the GitLab API.

### Step C. Configure CI/CD Environment Variables at Settings > CI/CD

Complete **Section 3** first to obtain `GEMINI_API_KEY` and `CLAUDE_API_KEY` before proceeding with this step.

Navigate to **Settings > CI/CD**, expand the **Variables** section, and click **Add variable** for each entry below.

All variables require the same flag configuration:

- **[Required] Mask variable** and **Hidden variable**: Prevents values from being printed in plaintext in pipeline logs.
- **[Must Not Be Checked] Protect variable**: If checked, the variable is restricted to protected branches only, blocking pipelines triggered from MR source branches.

| Variable             | Value                                                |
| -------------------- | ---------------------------------------------------- |
| `GEMINI_MR_REVIEWER` | Token string generated at Step B                     |
| `CLAUDE_MR_REVIEWER` | Token string generated at Step B                     |
| `GEMINI_API_KEY`     | API key from Google AI Studio (Section 3, Option 1)  |
| `CLAUDE_API_KEY`     | API key from Anthropic Console (Section 3, Option 2) |

## Section 3. Runner Setup

The project runner is provisioned via Terraform. It registers a project-scoped runner and writes the runner daemon configuration to the host machine.

### Step A. Generate Terraform Management Token

Navigate to the GitLab user avatar and select **User Settings > Access Tokens**.

- Click **Add new token**, set a token name and expiration date.
- Under **Select scopes**, check **`api`**.
- Click **Create personal access token** and copy the generated string immediately. It will not be displayed again.

The token owner must hold the **Owner** role on the target GitLab project. For projects in the user's personal namespace, this requirement is satisfied by default.

This token is referenced in both `backend.hcl` (`password`) and `terraform.tfvars` (`gitlab_token`).

### Step B. Prerequisites

- Terraform >= 1.8.0
- Podman and `podman-compose` installed on the host
- Rootless Podman socket active at `/run/user/<HOST_UID>/podman/podman.sock`

### Step C. Configure Terraform Files

Both files are gitignored and must be created manually. Use `terraform/terraform.tfvars.example` as a reference.

- For **`terraform/backend.hcl`**: Stores the Terraform HTTP backend credentials for remote state.
    - **`address`**: `https://gitlab.com/api/v4/projects/<PROJECT_ID>/terraform/state/default`
    - **`lock_address`**: `https://gitlab.com/api/v4/projects/<PROJECT_ID>/terraform/state/default/lock`
    - **`unlock_address`**: `https://gitlab.com/api/v4/projects/<PROJECT_ID>/terraform/state/default/lock`
    - **`username`**: `oauth2`
    - **`password`**: The User PAT generated in Step A.
    - **`lock_method`**: `POST`
    - **`unlock_method`**: `DELETE`
    - **`retry_wait_min`**: `5`

    The `<PROJECT_ID>` is found in **Settings > General** on the GitLab project page.

- For **`terraform/terraform.tfvars`**:
    - **`gitlab_token`**: The User PAT generated in Step A.
    - **`runner_description`**: Display name shown in **Settings > CI/CD > Runners** (default: `local-podman-runner`).
    - **`runner_tag_list`**: Tag list for explicit job targeting (default: `["podman", "local"]`).

### Step D. Provision with Terraform

1. Initialize the backend. The `-backend-config` flag loads the gitignored `backend.hcl` which holds the state address and credentials.

    ```bash
    terraform init -backend-config=backend.hcl
    ```

2. Import the existing GitLab project into Terraform state. This step is required on the first run because the project already exists; without it, `apply` would attempt to create a duplicate.

    ```bash
    terraform import gitlab_project.this <PROJECT_ID>
    ```

3. Apply the configuration. This registers the project runner on GitLab and writes the runner token to `~/.config/gitlab-runner/config.toml` on the host.

    ```bash
    terraform apply -auto-approve
    ```

### Step E. Start the Runner Service

1. Copy `.env.example` to `.env` and fill in the two variables:
    - **`UHOME`**: Absolute path to the user home directory. Obtain with `echo $HOME`.
    - **`HOST_UID`**: Numeric UID of the host user. Required to resolve the rootless Podman socket at `/run/user/<HOST_UID>/podman/podman.sock`. Obtain with `id -u`.

    ```bash
    cp .env.example .env
    ```

2. Start the runner container:

    ```bash
    podman compose up -d
    ```

## Section 4. Triggering Workflow

Once configuration is complete, reviews integrate into the standard development workflow.

1. **Create a Merge Request**:
    - Push a feature branch to the remote repository and open a Merge Request targeting `main` from the GitLab web interface.

2. **Trigger a Review Manually**:
    - After the MR is created, a pipeline will appear under the **Pipelines** tab.
    - Two review jobs are available: `gemini-code-review` and `claude-code-review`. Both are paused by default and display a **Play** button.
    - Clicking the **Play** button on either job triggers the GitLab Runner to spin up a Go container, compile and execute the reviewer binary against the MR diff, and post inline discussion comments to the MR timeline.
    - Both jobs may be triggered independently within the same pipeline.
