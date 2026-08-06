# Component: `auto-tag` in [`templates/auto-tag.yml`](../../../templates/auto-tag.yml)

**Binary:** `auto-tag` in `reviewer_image`

## Section 1. Scope

### Item A. Purpose

On pushes to the default branch, derive the next Semantic Version from Conventional Commit metadata (typically the squash-merge subject), optionally scoped per module via `.gitlab/versioning.yml`, and push the resulting git tag.

## Section 2. Contract

### Item A. Inputs (Summary)

| Input            | Default                  | Role                        |
| ---------------- | ------------------------ | --------------------------- |
| `reviewer_image` | _(required)_             | image providing `auto-tag`  |
| `stage`          | `deploy`                 | pipeline stage name         |
| `config_path`    | `.gitlab/versioning.yml` | module list and directories |

### Item B. Job

| Job        | Stage         | Rules                                   |
| ---------- | ------------- | --------------------------------------- |
| `auto-tag` | `stage` input | `push` and branch equals default branch |

Script form:

```bash
auto-tag --sha "$CI_COMMIT_SHA" --config "<config_path>" \
    --username "gitlab-ci-token" \
    --remote-url "https://${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git"
```

Authentication for the push uses `TAG_PUSH_TOKEN` (refer to [service-interface](../service-interface.md) and [versioning](../mechanisms/versioning.md)).

### Item C. Versioning File Shape

Default whole-repository tagging (this repository):

```yaml
modules:
  - name: ''
    dir: '.'
```

Non-empty `name` values select tag prefixes and directory-scoped change detection. Algorithm detail: [versioning](../mechanisms/versioning.md).

### Item D. Side Effects

Creates and pushes one or more git tags. Pushed tags SHOULD start the release pipeline that publishes images and Catalog entries for this product.

## Section 3. Verification

1. Merge a commit whose subject begins with `fix:` and confirm a patch bump tag.
2. Merge `feat:` and confirm a minor bump.
3. Merge with `!` breaking marker and confirm a major bump.
4. Confirm types outside the bump policy create no tag.
5. Confirm `CI_JOB_TOKEN` alone is insufficient if the consumer expects a tag pipeline (platform constraint).
