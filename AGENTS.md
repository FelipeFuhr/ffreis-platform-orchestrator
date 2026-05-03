# Agent Context

**This repo:** `ffreis-platform-orchestrator` — coordinates multi-step platform
automation flows (layer provisioning, account setup, deployments) with checkpointing
and resume capability.

## Non-obvious facts

- **Stateful — steps are resumable.** Progress is checkpointed; `resume` continues
  from the last successful step. Do not write step implementations that have
  non-idempotent side effects without checkpointing.

- **Logs to stderr, results to stdout.** Diagnostic text, prompts, and progress go to
  stderr. Machine-readable output goes to stdout. Never mix them.

- **Context-based value passing between steps.** Don't use global state; use the
  context value store that threads through the step chain.

## Structure

```
cmd/platform-orchestrator/   ← Cobra CLI entry point
cmd/                         ← init, resume, status, steps, version commands
```

## Build/run

```bash
make build
./bin/platform-orchestrator init
./bin/platform-orchestrator status
./bin/platform-orchestrator resume
```
