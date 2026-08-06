# Mechanism: Deterministic MR Labeling

## Section 1. Scope

### Item A. Purpose

Apply classification labels without an LLM. Every MR pipeline receives consistent `type::*`, optional `breaking-change`, and `area::*` labels from title, description, and path heuristics.

### Item B. Boundary with LLM Review

`mr-labeler` owns `type::*`, `breaking-change`, and `area::*`. The `security` label depends on findings from the current LLM review run and is owned by [review](review.md).

## Section 2. Behavior

### Item A. Control Flow

```mermaid
flowchart TB
    fetch[FetchMR]
    type[resolveCommitTypeLabel]
    break[detectBreakingChange]
    area[resolveAreaLabels]
    apply[AddLabels]
    fetch --> type --> break --> area --> apply
```

`labeler.ExecuteLabeling` is the binary entrypoint, where `ExecuteLabeling` constructs a `Labeler` and calls `Execute`.

`AddLabels` uses the GitLab `add_labels` parameter as a comma-separated string. The API appends the supplied labels. Existing labels on the merge request remain in place.

## Section 3. Policy

### Item A. Type Mapping

Subject pattern: `^([a-z]+)(\([^)]*\))?(!)?:\s` (refer to `resolveCommitTypeLabel` in `internal/labeler`).

| Commit type                               | Label                          |
| ----------------------------------------- | ------------------------------ |
| `feat`                                    | `type::feature`                |
| `fix`                                     | `type::fix`                    |
| `docs`                                    | `type::documentation`          |
| `refactor`                                | `type::refactor`               |
| `test`                                    | `type::test`                   |
| `perf`                                    | `type::enhancement`            |
| `build`, `chore`, `ci`, `revert`, `style` | `type::ad-hoc`                 |
| Unmatched subject                         | no type label from this mapper |

### Item B. Breaking Change Detection (`detectBreakingChange`)

1. `!` immediately before `:` in the Conventional Commit header, or
2. A body footer matching `BREAKING CHANGE` / `BREAKING-CHANGE` line prefixes.

Either condition adds `breaking-change`.

Tag major bumps under [versioning](versioning.md) use only the subject-line `!`. Body footers leave SemVer major increments unchanged.

### Item C. Area Rules (`resolveAreaLabels`)

First-match accumulation over changed paths with deduplication:

| Label                  | Path heuristic (summary)                                     |
| ---------------------- | ------------------------------------------------------------ |
| `area::infrastructure` | Terraform/OpenTofu/HCL paths                                 |
| `area::CI`             | `.gitlab-ci.yml`, `templates/*.yml`, `tools/ci/`             |
| `area::frontend`       | `frontend/`                                                  |
| `area::backend`        | `backend/`                                                   |
| `area::observability`  | grafana, prometheus, monitoring, observability path segments |

Rules are repository-opinionated defaults for this repository layout. Forks that require different areas MUST change `internal/labeler` and cut a release. Inputs currently omit parameterization of the rule table.

## Section 4. Verification

1. `go test` under `tools/ci/internal/labeler`.
2. Open an MR titled `fix(ci): ...` touching `templates/core.yml`. Expect `type::fix` and `area::CI`.
3. Confirm a prior human label remains after the job runs under `add_labels` append semantics.
