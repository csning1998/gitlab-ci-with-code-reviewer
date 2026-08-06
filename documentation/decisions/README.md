# Decisions

This dimension records accepted operating principles and deferred alternatives. Decisions constrain product behavior as a cross-cutting index wherever those choices apply.

Long-form memoranda currently reside under `documentation/MOU-*.md`. This directory provides the navigation surface and status for the architecture reading path under `documentation/`. A later commit on this branch MAY move full text here as `ADR-NNN-*.md` without changing decisions.

## Section 1. Topology

### Item A. Decision Index

| ID   | Title                                                              | Status                                                    | Source                                         |
| ---- | ------------------------------------------------------------------ | --------------------------------------------------------- | ---------------------------------------------- |
| D001 | Diff-bounded review context (not full repository)                  | Accepted (operating) / full-repo alternative **deferred** | `documentation/MOU-review-context-strategy.md` |
| D002 | Static `CLAUDE_API_KEY` per consumer (WIF alternative deferred)    | Accepted for present scale / WIF **deferred**             | `documentation/MOU-claude-api-key-strategy.md` |
| D003 | Pin `reviewer_image` with no component default                     | Accepted                                                  | `templates/core.yml`, release prose            |
| D004 | Manual LLM review jobs with `allow_failure: true`                  | Accepted                                                  | `templates/core.yml`                           |
| D005 | SemVer tag without `v` prefix; couple image tag to Catalog version | Accepted                                                  | release design, `.gitlab-ci.yml`               |
| D006 | `TAG_PUSH_TOKEN` required for release-triggering tags              | Accepted                                                  | platform constraint + `auto-tag`               |
| D007 | Deterministic labeling independent of LLM                          | Accepted                                                  | `misc:mr-labeler`                              |
| D008 | Subject-only SemVer bump (ignore squash body)                      | Accepted                                                  | `internal/semver`                              |

## Section 2. Policy

### Item A. Interpretation of Deferred Items

Deferred means:

1. Analysis exists and recommends not implementing now.
2. Implementation MUST NOT begin without satisfying the memorandum prerequisites (measurements, API feasibility, or scale trigger).
3. Reopening the decision requires new evidence recorded beside the original memo.

### Item B. D001 Summary (Review Context)

**Operating decision:** prompts carry annotated MR diffs capped at 300000 characters, plus optional MR intent text.

**Deferred alternative:** serialize full repository content with prompt caching anchored on the target branch.

**Rationale (summary):** full-tree context is technically feasible on large windows but requires token measurement, cache economics, and a serialization subsystem; streaming for the diff design is the independent improvement with clearer payoff.

### Item C. D002 Summary (Claude Credentials)

**Operating decision:** each consumer supplies `CLAUDE_API_KEY` as a CI variable (Gemini keys may be provisioned elsewhere via platform automation).

**Deferred alternative:** GitLab OIDC to Anthropic Workload Identity Federation, eliminating static Anthropic secrets in CI.

**Rationale (summary):** Admin API cannot create Claude keys; WIF is feasible and preferred when consumer count grows, but spans template, Go client, and identity operations beyond current repository count.

## Section 3. Verification

### Item A. Verification of Decision Hygiene

1. Any MR that implements a deferred alternative MUST update this index status and the source memorandum in the same change set.
2. Any MR that breaks D003 through D008 MUST bump major Catalog version and document consumer migration in the MR template Changes section.
