# GitHub Copilot instructions — CalcAssist

These are repository-wide custom instructions for GitHub Copilot. For the full
engineering reference, see [`AGENTS.md`](../AGENTS.md).

## Project context

CalcAssist is a cross-platform Go terminal AI agent (module `calcassist`). It runs a
Copilot-style scrolling REPL and uses an LLM with native tool calling to drive a
multi-step agentic loop over local tools. It builds to a single `CGO_ENABLED=0` binary.

Key packages: `cmd/calcassist` (Cobra CLI), `internal/config` (YAML config),
`internal/llm` (OpenAI/Azure Responses API + Anthropic Messages API + factory),
`internal/agent` (the loop), `internal/tools` (Tool interface + 9 tools),
`internal/tui` (Bubble Tea REPL), `internal/version`.

## When generating or changing code

- Target **Go** with the toolchain in `go.mod`; keep everything **pure Go / `CGO_ENABLED=0`**.
  Do not introduce cgo or heavyweight dependencies.
- Respect package boundaries: `config`, `llm`, and `tools` are **standalone** and must
  **not import each other** or `agent`/`tui`. Wiring happens in `cmd/calcassist`.
  `llm` has its own `llm.Config`; `main` maps `config.Config` into it.
- Format with `gofmt`; use **LF** line endings (enforced by `.gitattributes`).
- Wrap errors with `%w` and useful context. Tool errors should be user-actionable and
  are returned to the model as results (the loop self-corrects); only context
  cancellation or confirmation failure aborts a run.
- Add or update **hermetic** tests for any change: `httptest` for `llm`, `t.TempDir()`
  and generated fixtures for `tools`, a mock `llm.Provider` for `agent`. No network in tests.

## Adding a tool

Implement the `Tool` interface in `internal/tools/<name>.go` with a `New<Name>Tool()`
constructor (JSON-Schema `Schema()`, typed-struct unmarshal in `Execute`, `Mutating()`
true only if it writes files), register it in `internal/tools/defaults.go`, and add a test.

## Adding an LLM provider

Implement `llm.Provider` (`Name()` + `Chat(ctx, messages, tools, onText)`; stream when
`onText != nil`) and wire it into `llm.New()` in `internal/llm/factory.go`, with
`httptest` tests for request mapping, auth, streaming/non-streaming, and tool calls.

## Secrets & config

Never hardcode or commit API keys. Configuration lives in `~/.calcassist/config.yaml`
(git-ignored); prefer env vars (`CALCASSIST_API_KEY`, `OPENAI_API_KEY`,
`AZURE_OPENAI_API_KEY`, `ANTHROPIC_API_KEY`). Azure uses the `/openai/v1/responses`
path — no `api-version`.

## Before finishing a change

Run `gofmt -w .`, `go vet ./...`, and `go test ./...` (all must pass). PR CI runs these
on an Ubuntu/Windows matrix.
