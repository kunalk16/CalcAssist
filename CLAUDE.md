# CLAUDE.md

Repository guide for Claude / AI agents. For the full engineering reference see
[AGENTS.md](AGENTS.md); this file is the quick version.

## Project

**CalcAssist** — a cross-platform Go terminal AI agent. A Copilot-style scrolling REPL
drives a multi-step agentic loop: an LLM (OpenAI/Azure via the Responses API, or
Anthropic via the Messages API) picks and runs local **tools** with native tool calling.
Single `CGO_ENABLED=0` binary. Module path: `calcassist`.

## Commands

```bash
# build / run
make build           # -> bin/calcassist[.exe]   (or ./build.ps1 / ./build.sh)
go run ./cmd/calcassist
./bin/calcassist -p "what is sqrt(144)?"   # one-shot mode

# checks (run all three before finishing)
gofmt -w .           # or: make fmt
go vet ./...
go test ./...        # or: make test
```

Keep the build **cgo-free**. Run `gofmt`, `go vet`, and `go test` after every change.

## Layout

```
cmd/calcassist     Cobra CLI (flags, config load, REPL vs one-shot -p)
internal/config    YAML config (~/.calcassist/config.yaml), env overrides, validation
internal/llm       neutral types + Provider interface + OpenAI/Azure + Anthropic + factory
internal/agent     the agentic loop (agent.Run + Hooks)
internal/tools     Tool interface, Registry, 9 built-in tools (defaults.go)
internal/tui       Bubble Tea inline scrolling REPL
internal/version   ldflags-stamped version
```

## Architecture rules (don't break these)

- `config`, `llm`, and `tools` are **standalone** — no cross-imports between them.
  `main` maps `config.Config` → `llm.Config`. `agent` imports `llm`+`tools`; `tui`
  imports `agent`+`tools`.
- The agent advertises each tool's JSON schema, runs requested tools (asking the
  `Confirm` hook for **mutating** ones — currently only `create_file`), feeds results
  back, and loops until a final answer or `max_tool_iterations`.
- Tool/LLM HTTP errors are returned to the model as results so it can recover; only
  context-cancel / confirm failures abort the loop.

## Adding things

- **New tool:** implement `Tool` in `internal/tools/<name>.go` (`New<Name>Tool()`),
  register in `defaults.go`, add a hermetic test (`t.TempDir()` / generated fixtures).
- **New provider:** implement `llm.Provider`, wire into `llm.New()` in `factory.go`,
  add `httptest` tests.

## Conventions

- `gofmt` clean; **LF** endings (`.gitattributes`). Comment only where it clarifies.
- Tests are table-driven and hermetic (`httptest` for `llm`, temp dirs for `tools`,
  a mock provider for `agent`). No network in tests.
- Never commit secrets; `config.yaml` is git-ignored. Use env vars
  (`CALCASSIST_API_KEY`, `OPENAI_API_KEY`, `AZURE_OPENAI_API_KEY`, `ANTHROPIC_API_KEY`).

## Notes

- PDF = text only (no OCR); Word = `.docx` only.
- The TUI is inline Bubble Tea (no alt-screen); finalized output uses `tea.Println`,
  agent work runs in a goroutine that streams events through a channel.
