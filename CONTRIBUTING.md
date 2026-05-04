# Contributing

## CLI Output Rule

- Use structured logging for diagnostics and lifecycle events.
- Use the command/output or prompt layer for operator-facing terminal UX.
- Keep `stdout` for command results and `stderr` for logs, prompts, and progress.

## `fmt` Usage

- Prefer `io.WriteString`, `strconv`, `strings.Builder`, and `path/filepath` for simple output and composition.
- Use `fmt.Errorf` for wrapped errors.
- Use `fmt` for local formatting only when it is clearly the simplest readable option.

## Logging

- Log structured key/value fields rather than preformatted strings.
- Do not route human status lines, confirmations, or next-step hints through the logger.

## Repo Layout

- This repo follows the Go CLI archetype.
- Keep the executable entrypoint in `cmd/platform-orchestrator/main.go`.
- Keep Cobra wiring outside `main.go`; `main.go` should only call the top-level execute path.
- Keep automation in `scripts/`.
- Do not move the CLI entrypoint back to the repo root.
