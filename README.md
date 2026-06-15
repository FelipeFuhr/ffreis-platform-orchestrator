# ffreis-platform-orchestrator

<!-- ffreis-badges:start -->
[![CI](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/FelipeFuhr/ffreis-badges/main/badges/ffreis-platform-orchestrator/ci.json)](https://github.com/FelipeFuhr/ffreis-platform-orchestrator/actions) [![License](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/FelipeFuhr/ffreis-badges/main/badges/ffreis-platform-orchestrator/license.json)](https://github.com/FelipeFuhr/ffreis-platform-orchestrator/blob/main/LICENSE)
<!-- ffreis-badges:end -->

A Go CLI that coordinates multi-step platform provisioning flows — AWS Organization
bootstrap, org-level Terraform, and GitHub OIDC setup — as a single resumable
pipeline. Each step's progress is checkpointed to DynamoDB, so a run that fails
partway through can be continued from the last successful step instead of being
re-run from scratch. Steps declare their dependencies and are executed in
topological order; values produced by earlier steps are threaded to later ones
through a context value store rather than global state.

## What it does

The orchestrator builds a DAG of provisioning steps and runs them in dependency
order under the appropriate AWS privilege level for each step. The canonical
`platform-setup` pipeline registers:

- **Bootstrap** (`root` credential class) — create the AWS Organization, enable
  service control policies, create the admin role.
- **Org Terraform** (`admin` class) — `terraform init`, `plan`, `apply` against
  the org-level configuration.
- **OIDC** (`admin` / `operator` class) — create the GitHub OIDC provider, create
  the GitHub deploy role, write resulting outputs.

Key behaviors:

- **Stateful and resumable.** Per-step state (`pending`, `running`, `succeeded`,
  `failed`, `skipped`, `rolled_back`, `rollback_failed`), attempt counts, and
  outputs are persisted to DynamoDB. `resume` skips already-succeeded steps.
- **Credential classes via STS.** Each step declares whether it needs `root`
  (default-chain credentials — never stored), `admin`, or `operator`. Admin and
  operator roles are assumed via `sts:AssumeRole`; their ARNs are read lazily from
  the config store (so they can be produced by an earlier bootstrap step).
- **Stream separation.** Logs, prompts, and progress go to stderr; machine-readable
  output goes to stdout.
- **Input collection.** Required inputs are gathered interactively, or in batch from
  the config store when `--non-interactive` is set, then persisted before execution.
- **Dry-run.** `--dry-run` describes the actions a run would take without executing
  them.
- **Optional post-run handoff.** `init --run` invokes an external `platform-runner`
  binary as a subprocess (`validate` → `sync-template` → `plan-all`) after the
  pipeline completes. There is no package-level coupling to that tool.

## Usage

Build the binary:

```bash
make build          # → bin/platform-orchestrator
```

Commands:

```bash
platform-orchestrator init    --project platform --env dev   # start a new run
platform-orchestrator resume  --project platform --env dev   # continue the last (or --run-id) failed run
platform-orchestrator status  [--run-id <id>]                # show step states for a run
platform-orchestrator steps                                  # list the pipeline steps and their deps
platform-orchestrator version                                # print build info
```

`init` and `resume` require `--project` and `--env`. `--env` has no default, by
design, to prevent accidentally targeting prod.

Global flags (persistent across subcommands):

| Flag | Default | Purpose |
|---|---|---|
| `--table` | _(from env)_ | DynamoDB table name; overrides `ORCHESTRATOR_TABLE` |
| `--region` | _(AWS default)_ | AWS region |
| `--log-level` | `info` | Log level |
| `--non-interactive` | `false` | Disable prompts; fail if any required value is missing |
| `--confirm-all` | `false` | Auto-confirm all confirmation gates (for CI) |
| `--dry-run` | `false` | Describe actions without executing them |
| `--project` | `platform` | Target platform project |
| `--env` | _(required)_ | Target environment (`dev`, `staging`, `prod`) |
| `--ui` | `auto` | UI mode: `auto`, `plain`, `rich` |

`init` additionally accepts `--run` and `--runner-*` flags to drive the external
`platform-runner` handoff.

Configuration (environment variables; CLI flags override):

| Variable | Required | Purpose |
|---|---|---|
| `ORCHESTRATOR_TABLE` | yes (unless `--table`) | DynamoDB state/config table |
| `AWS_DEFAULT_REGION` / `AWS_REGION` | no | AWS region |
| `ORCHESTRATOR_LOG_LEVEL` | no (default `info`) | Log level |
| `GITHUB_TOKEN` | no | Fallback token for the `platform-runner` handoff |

AWS credentials are resolved from the standard default chain for the `root` class;
`admin` and `operator` classes assume IAM roles via STS.

### Container

A multi-stage `Containerfile` (works with podman or docker) produces a distroless
image with only the binary:

```bash
make container-build           # build ghcr.io/ffreis/platform-orchestrator:dev
make container-run ARGS="steps"
```

## Development

Go 1.25 module (`github.com/ffreis/platform-orchestrator`). Layout:

```
cmd/platform-orchestrator/   Cobra CLI main entry point
cmd/                         init, resume, status, steps, version commands
internal/pipeline/           DAG, engine, step interface, persisted state
internal/steps/              bootstrap, oidc, terraform step implementations + registry
internal/credential/         credential classes + STS-based AWS resolver
internal/configctl/          DynamoDB-backed config/state store
internal/appconfig/          environment-variable config
internal/prompt/             interactive + batch input collection
internal/runner/             subprocess exec runner
internal/postrun/            external platform-runner invoker
pipelines/                   platform_setup.go — the canonical pipeline definition
```

Common targets (`make help` lists all):

```bash
make build         # compile to bin/ with version ldflags
make test          # go test ./... -race -shuffle=on -count=1
make test-short    # unit tests, no live AWS
make fmt-check     # gofmt check (mirrors CI)
make vet lint      # go vet + golangci-lint
make sec           # govulncheck
make ci            # fmt-check + vet + lint + test + sec (local CI gate)
make validate      # go vet + compile check
make fuzz          # run Fuzz* targets (default 30s each)
make mutation-test # gremlins mutation testing (slow; CI/weekly)
```

To add a step, implement the step interface, register it in
`pipelines/platform_setup.go`, and declare its dependencies within the step itself.
Avoid non-idempotent side effects without checkpointing — steps must be safe to
skip on resume.

## License

MIT. See [LICENSE](LICENSE).
