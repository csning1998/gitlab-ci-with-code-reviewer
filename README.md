# GitLab Merge Request Reviewer

## I. Google AI Studio Setup Process

The primary objective on the Google platform is to acquire model access credentials and verify model access permissions.

1. **Create an API Key**:
    - Log in to the Google AI Studio platform.
    - Click **Get API key** in the upper-left corner or navigation bar.
    - Select **Create API key** and choose the Google Cloud project associated with the account.
    - Once the system generates the key, copy the string completely and store it securely.

2. **Verify Model Quota and Availability**:
    - In the model selection list on the right side of the platform, verify that the account is currently able to call the designated model under the free tier (e.g., `gemini-3.5-flash`).
    - Ensure that the model quota is not restricted within the region and project (the limit must be greater than `0`).

## II. Anthropic Console Setup Process

The primary objective on the Anthropic platform is to acquire API credentials for Claude model access.

1. **Create an API Key**:
    - Log in to the Anthropic Console.
    - Navigate to **API Keys** and click **Create Key**.
    - Copy the generated key string immediately and store it securely (it will not be displayed again).

2. **Verify Model Access**:
    - Confirm that the account has access to the designated model (e.g., `claude-sonnet-4-6`) and that the usage tier permits API calls.

## III. GitLab Setup Process

The workflow on the GitLab platform consists of two parts:

1. Generating one or more access tokens that permit the pipeline to read MR changes and post review comments, and
2. Injecting all credentials into the project CI/CD environment variables.

### A. Generate a GitLab Access Token

This token enables the Go binary in the pipeline to read Merge Request (MR) diffs and post inline discussion threads.

- Navigate to GitLab and click the user avatar to access **User Settings > Access Tokens** (alternatively, use **Project Settings > Access Tokens** within the project).
- Click **Add new token**, configure the token name, and select an expiration date.
- Under **Select scopes**, check **`api`** (Classic PAT with full API access is required — fine-grained PATs do not expose the Merge Request Create permission needed to post inline discussion threads).
- After clicking create, **the generated token string must be copied immediately** (it will not be displayed again after leaving the page).

A single PAT may be reused for both `GEMINI_MR_REVIEWER` and `CLAUDE_MR_REVIEWER` below, or separate tokens may be issued per reviewer.

### B. Configure CI/CD Environment Variables

All credentials must be stored in the project settings to be referenced by `.gitlab-ci.yml`.

- Navigate to the GitLab project page and select **Settings > CI/CD** on the left sidebar.
- Locate the **Variables** section and click **Expand** on the right side.
- Click **Add variable** and create the following variables:

#### Variable A: Gemini API Key

- **Key**: `GEMINI_API_KEY`
- **Value**: Paste the API key copied from Google AI Studio.
- **Flags**:
    - **[Required] Mask variable** and **Hidden variable** (ensures the key is not printed in plaintext in pipeline logs).
    - **[Must Not Be Checked] Protect variable** (if checked, this variable will only be available on protected branches, blocking MRs from development branches).

#### Variable B: GitLab Token for Gemini Reviewer

- **Key**: `GEMINI_MR_REVIEWER`
- **Value**: Paste the GitLab access token generated in Step A.
- **Flags**:
    - **[Required] Mask variable** and **Hidden variable**.
    - **[Must Not Be Checked] Protect variable**.

#### Variable C: Anthropic API Key

- **Key**: `CLAUDE_API_KEY`
- **Value**: Paste the API key copied from the Anthropic Console.
- **Flags**:
    - **[Required] Mask variable** and **Hidden variable**.
    - **[Must Not Be Checked] Protect variable**.

#### Variable D: GitLab Token for Claude Reviewer

- **Key**: `CLAUDE_MR_REVIEWER`
- **Value**: Paste the GitLab access token generated in Step A (may be the same token as `GEMINI_MR_REVIEWER`).
- **Flags**:
    - **[Required] Mask variable** and **Hidden variable**.
    - **[Must Not Be Checked] Protect variable**.

## IV. Triggering Workflow

Once configuration is complete, reviews integrate into the standard development workflow.

1. **Create a Merge Request**:
    - Push a feature branch to the remote repository and open a Merge Request targeting `main` from the GitLab web interface.

2. **Trigger a Review Manually**:
    - After the MR is created, a pipeline will appear under the **Pipelines** tab.
    - Two review jobs are available: `gemini-code-review` and `claude-code-review`. Both are paused by default and display a **Play** button.
    - Clicking the **Play** button on either job triggers the GitLab Runner to spin up a Go container, compile and execute the reviewer binary against the MR diff, and post inline discussion comments to the MR timeline.
    - Both jobs may be triggered independently within the same pipeline.
