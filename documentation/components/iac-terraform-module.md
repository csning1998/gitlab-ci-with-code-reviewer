# Component: `iac-terraform-module` in [`templates/iac-terraform-module.yml`](../../templates/iac-terraform-module.yml)


## Section 1. Scope

### Item A. Purpose

On a matching SemVer (or prefixed) tag, package `module_dir` and upload it to the GitLab Terraform Module Registry.

## Section 2. Contract

### Item A. Inputs (Summary)

| Input           | Default                      | Role                                     |
| --------------- | ---------------------------- | ---------------------------------------- |
| `module_dir`    | `$CI_PROJECT_DIR`            | archive root                             |
| `module_name`   | `$CI_PROJECT_NAME`           | registry module name                     |
| `module_system` | `local`                      | system/provider segment in registry path |
| `curl_image`    | `curlimages/curl:8.10.1`     | upload job image                         |
| `tag_prefix`    | `$[[ inputs.module_name ]]-` | `rules:` match against `CI_COMMIT_TAG`   |

### Item B. Jobs

| Job                                    | Stage  | Notes                                                 |
| -------------------------------------- | ------ | ----------------------------------------------------- |
| `deploy:terraform-module-$module_name` | deploy | tag rule uses `tag_prefix`; upload via `CI_JOB_TOKEN` |

### Item C. Side Effects

Creates a module package visible in the project Terraform Module Registry.

### Item D. Invariants

1. In a multi-module monorepo, each include MUST set a distinct `tag_prefix`, which prevents a tag for module A from publishing module B.
2. Archives exclude `.git`, `.terraform`, state files, and `*.tfvars`.

## Section 3. Verification

Push a tag that matches `tag_prefix`. Confirm a single deploy job for the tagged module runs and the registry lists the new version.
