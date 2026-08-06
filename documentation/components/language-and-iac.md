# Language and IaC Components

This page is the contract sheet for optional Catalog packs that extend `core` on Merge Request pipelines (and, for some jobs, additional pipeline sources). Each item names one published template under `templates/`. Authoritative input schemas remain `templates/<name>.yml` `spec.inputs`.

Shared stage semantics: [pipeline-protocol.md](../pipeline-protocol.md). Shared format-push control pattern: [components README](README.md) Behavior section and [formatting-and-push.md](../mechanisms/formatting-and-push.md).

`iac-terraform-module` and `auto-tag` are out of scope for this page. Module registry upload and SemVer tagging use separate lifecycle documents.

## Section 1. Scope

### Item A. Role Relative to `core`

1. Each pack assumes `core` stages exist.
2. Format jobs that rewrite the worktree `need` the `fmt:prepare-pusher` artifact from `core`.
3. Path `changes:` globs keep unrelated MRs from pulling language or IaC tool images.
4. Defaults under `templates/` often match this product monorepo; consumers MUST reset directory and glob inputs to the consuming layout.

## Section 2. Contract

### Item A. `lang-go` in [`templates/lang-go.yml`](../../../templates/lang-go.yml)

Go format, build, test (with coverage artifact), and `golangci-lint` gated by path globs.

| Input             | Default                                      | Role                                   |
| ----------------- | -------------------------------------------- | -------------------------------------- |
| `go_image`        | `golang:1.26-alpine`                         | fmt, build, test                       |
| `golangci_image`  | `golangci/golangci-lint:v2.12.2-alpine`      | lint                                   |
| `go_dir`          | `tools/ci`                                   | module directory for build, test, lint |
| `go_fmt_path`     | `./tools`                                    | path passed to `gofmt -w`              |
| `go_globs`        | `tools/**/*.go`, module files under `tools/` | `changes:` gate                        |
| `golangci_config` | `../../.gitlab/golangci.yml`                 | path relative to `go_dir`              |

Consumers outside this monorepo MUST reset `go_dir`, `go_fmt_path`, `go_globs`, and `golangci_config` to the module layout.

| Job        | Stage  | Notes                                                         |
| ---------- | ------ | ------------------------------------------------------------- |
| `fmt:go`   | format | `gofmt`; push via `fmt-commit-pusher`; `interruptible: false` |
| `build:go` | build  | `go build ./...` in `go_dir`                                  |
| `test:go`  | build  | `go test -coverprofile=coverage.out ./...`; artifact coverage |
| `lint:go`  | lint   | `golangci-lint run`                                           |

**Side effects:** May push a commit with subject `style: gofmt` to the MR source branch.

### Item B. `lang-python` in [`templates/lang-python.yml`](../../../templates/lang-python.yml)

Ruff format and autofix commit, Ruff lint, and pytest. Package manager input selects `pip` or `uv` installation paths.

| Input             | Default              | Role                             |
| ----------------- | -------------------- | -------------------------------- |
| `package_manager` | `pip`                | `pip` or `uv`                    |
| `python_image`    | `python:3.11-alpine` | Override for uv images as needed |
| `python_globs`    | `**/*.py`            | `changes:` gate                  |
| `python_test_cmd` | `pytest`             | test command                     |
| `test_stage`      | `test`               | stage name for `test:python`     |

| Job           | Stage        | Notes                                                                              |
| ------------- | ------------ | ---------------------------------------------------------------------------------- |
| `fmt:python`  | format       | ruff format + `ruff check --fix`; push; `interruptible: false`                     |
| `lint:python` | lint         | `ruff check` (no write)                                                            |
| `test:python` | `test_stage` | installs project deps then `python_test_cmd`; also runs on default branch and tags |

**Side effects:** May push `style: ruff format and autofix`.

### Item C. `lang-typescript` in [`templates/lang-typescript.yml`](../../../templates/lang-typescript.yml)

Prettier format commit, parallel frontend (`vue-tsc`) and backend (`tsc --noEmit`) typechecks, and Bun workspace tests.

| Input          | Default                                           | Role               |
| -------------- | ------------------------------------------------- | ------------------ |
| `bun_image`    | `oven/bun:1-alpine`                               | all jobs           |
| `ts_globs`     | `frontend/**/*`, `backend/**/*`, `contracts/**/*` | `changes:`         |
| `frontend_dir` | `frontend`                                        | `vue-tsc` cwd      |
| `backend_dir`  | `backend`                                         | `tsc --noEmit` cwd |

| Job                        | Stage  | Notes                                                         |
| -------------------------- | ------ | ------------------------------------------------------------- |
| `fmt:typescript`           | format | `prettier --write .`; push; `interruptible: false`            |
| `lint:typescript-frontend` | lint   | `vue-tsc --noEmit`                                            |
| `lint:typescript-backend`  | lint   | `tsc --noEmit`                                                |
| `test:typescript`          | test   | `bun run --filter '*' test`; optional needs on both lint jobs |

**Side effects:** May push `style: prettier fmt`.

### Item D. `iac-terraform` in [`templates/iac-terraform.yml`](../../../templates/iac-terraform.yml)

Recursive `terraform fmt` with commit push, and Checkov scan for the Terraform framework.

| Input             | Default                                           | Role                      |
| ----------------- | ------------------------------------------------- | ------------------------- |
| `terraform_image` | `hashicorp/terraform:1.15`                        | fmt                       |
| `checkov_image`   | `bridgecrew/checkov:3.3.0`                        | security                  |
| `checkov_dir`     | `terraform/`                                      | scan root                 |
| `checkov_skip`    | selected CKV ids                                  | comma-separated skip list |
| `terraform_globs` | `**/*.tf`, `**/*.hcl`, `**/*.tfvars`, `**/*.tofu` | `changes:`                |

| Job                          | Stage    | Notes                                            |
| ---------------------------- | -------- | ------------------------------------------------ |
| `fmt:terraform`              | format   | `terraform fmt -recursive`; push; gated by globs |
| `security:checkov-terraform` | security | `allow_failure: true`                            |

**Side effects:** May push `style: terraform fmt`.

### Item E. `iac-packer` in [`templates/iac-packer.yml`](../../../templates/iac-packer.yml)

Recursive Packer format with commit push.

| Input          | Default                 | Role       |
| -------------- | ----------------------- | ---------- |
| `packer_image` | `hashicorp/packer:1.15` | fmt        |
| `packer_dir`   | `packer/`               | fmt root   |
| `packer_globs` | `packer/**/*.hcl`       | `changes:` |

| Job          | Stage  | Notes                         |
| ------------ | ------ | ----------------------------- |
| `fmt:packer` | format | `packer fmt -recursive`; push |

**Side effects:** May push `style: packer fmt`.

### Item F. `iac-ansible` in [`templates/iac-ansible.yml`](../../../templates/iac-ansible.yml)

`ansible-lint` and Checkov Ansible framework scan. No format-and-push job.

| Input                | Default                                   | Role              |
| -------------------- | ----------------------------------------- | ----------------- |
| `ansible_lint_image` | `pipelinecomponents/ansible-lint:0.79.33` | lint              |
| `checkov_image`      | `bridgecrew/checkov:3.3.0`                | security          |
| `ansible_dir`        | `ansible`                                 | working directory |
| `ansible_globs`      | `ansible/**`, `ansible.cfg`               | `changes:`        |

| Job                        | Stage    | Notes                                |
| -------------------------- | -------- | ------------------------------------ |
| `lint:ansible`             | lint     | `allow_failure: true`                |
| `security:checkov-ansible` | security | `--soft-fail`; `allow_failure: true` |

**Side effects:** None on the git remote.

## Section 3. Verification

| Component         | Observation                                                                                                      |
| ----------------- | ---------------------------------------------------------------------------------------------------------------- |
| `lang-go`         | Touch a `.go` file under `go_globs`; expect `fmt:go` then build/test/lint. Touch only Markdown; expect skip.     |
| `lang-python`     | Change a `.py` file; confirm format then lint/test. On default branch, confirm `test:python` when rules match.   |
| `lang-typescript` | Change a file under `ts_globs`; confirm format and both typecheck jobs; confirm test when lockfile install works |
| `iac-terraform`   | Edit a `.tf` file; confirm fmt. Confirm Checkov skip list matches documented consumer policy.                    |
| `iac-packer`      | Touch HCL under `packer_globs`; confirm format job and optional style commit.                                    |
| `iac-ansible`     | Change playbooks under `ansible_globs`; confirm both jobs schedule and soft-fail matches debt policy.            |

Related: [core.md](core.md), [iac-terraform-module.md](iac-terraform-module.md), [auto-tag.md](auto-tag.md), [service-interface.md](../service-interface.md).
