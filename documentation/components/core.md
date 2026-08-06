# Component: `core` in [`templates/core.yml`](../../../templates/core.yml)

**Required input:** `reviewer_image` (no default)

## Section 1. Scope

### Item A. Purpose

`core` installs the shared stage list, workflow auto-cancellation policy, cross-language quality jobs, deterministic MR policy jobs, and optional manual LLM review jobs. Language components assume this stage list exists.

## Section 2. Contract

### Item A. Inputs (Contract Summary)

| Input                    | Type    | Default                                  | Notes                                           |
| ------------------------ | ------- | ---------------------------------------- | ----------------------------------------------- |
| `reviewer_image`         | string  | _(none)_                                 | MUST pin `reviewer:X.Y.Z` for production        |
| `claude_model`           | string  | `''`                                     | Non-empty enables `review:claude-code`          |
| `gemini_model`           | string  | `''`                                     | Non-empty enables `review:gemini-code`          |
| `markdownlint_image`     | string  | `markdownlint/markdownlint:0.17.0`       |                                                 |
| `yamllint_image`         | string  | `pipelinecomponents/yamllint:0.35.12`    |                                                 |
| `max_description_chars`  | number  | `5000`                                   | Shared by gate and review                       |
| `claude_max_tokens`      | number  | `16384`                                  | Claude Messages API max output tokens           |
| `enable_commitlint`      | boolean | `true`                                   |                                                 |
| `gitleaks_image`         | string  | `docker.io/zricethezav/gitleaks:v8.30.1` |                                                 |
| `enable_sonarqube`       | boolean | `false`                                  | Requires `SONAR_HOST_URL` and `SONAR_TOKEN`     |
| `sonar_scanner_image`    | string  | Sonar scanner CLI pin                    |                                                 |
| `sonar_scanner_tags`     | array   | `[]`                                     | Runner tags for Sonar network reachability      |
| `sonar_go_coverage_path` | string  | `''`                                     | Relative path to `go test -coverprofile` output |

### Item B. Jobs Emitted

| Job                  | Stage    | Rules summary                 | Side effects                              |
| -------------------- | -------- | ----------------------------- | ----------------------------------------- |
| `fmt:prepare-pusher` | format   | MR only                       | Artifact: `fmt-commit-pusher` binary      |
| `misc:mr-gate`       | lint     | MR only                       | Fails if description length exceeds limit |
| `misc:mr-labeler`    | lint     | MR only                       | Writes GitLab labels                      |
| `lint:commit`        | lint     | MR and `enable_commitlint`    | None                                      |
| `security:gitleaks`  | security | MR only                       | None (fails on findings)                  |
| `security:sonarqube` | security | MR and `enable_sonarqube`     | `allow_failure: true`                     |
| `lint:markdown`      | lint     | MR and `**/*.md` changes      | `allow_failure: true`                     |
| `lint:yaml`          | lint     | MR and YAML changes           | None                                      |
| `review:claude-code` | review   | MR and non-empty Claude model | Manual; discussions                       |
| `review:gemini-code` | review   | MR and non-empty Gemini model | Manual; discussions                       |

### Item C. Invariants

1. Review jobs MUST NOT become required merge gates without a major-version interface change and an explicit consumer opt-in design.
2. `fmt:prepare-pusher` MUST remain free of language-specific tools, which allows every formatter image to fetch a single binary artifact.
3. `security:gitleaks` runs on every MR regardless of path filters.

## Section 3. Verification

1. Include `core` alone with pinned image and empty models: labeler, gate, gitleaks present; review jobs absent.
2. Set `claude_model`; confirm manual `review:claude-code`.
3. Set description over `max_description_chars`; confirm `misc:mr-gate` fails.
