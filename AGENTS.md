# AGENTS.md

Guidance for AI coding agents and human contributors working in this repository.
This is the canonical engineering reference — `CLAUDE.md` and
`.github/copilot-instructions.md` point here.

## What this project is

**CalcAssist** is a cross-platform (Windows/Linux/macOS) terminal AI agent written in
Go. It runs a Copilot-style scrolling REPL and uses an LLM with **native tool calling**
to drive a **multi-step agentic loop** over local tools (file ops, math/statistics,
Excel→JSON, PDF/Word reading). It ships as a single, `CGO_ENABLED=0` binary.

- Module path: `calcassist`
- Go: see the `go` directive in [`go.mod`](go.mod) (developed on Go 1.26).
- Entry point: [`cmd/calcassist/main.go`](cmd/calcassist/main.go)

## Quick commands

| Task | Command |
|------|---------|
| Build (host) | `./build.ps1` · `./build.sh` · `make build` → `bin/calcassist[.exe]` |
| Cross-compile all | `./build.ps1 -All` · `./build.sh all` · `make all` → `dist/` |
| Run REPL | `go run ./cmd/calcassist` or `./bin/calcassist` |
| One-shot | `./bin/calcassist -p "what is sqrt(144)?"` |
| Test (all) | `go test ./...` · `make test` |
| Vet | `go vet ./...` · `make vet` |
| Format | `gofmt -w .` · `make fmt` |
| Format check | `make fmtcheck` (CI fails on unformatted files) |
| Tidy modules | `go mod tidy` |

Always run `gofmt -w .`, `go vet ./...`, and `go test ./...` before finishing a change.
Builds must remain **CGO-free** (`CGO_ENABLED=0`); do not add cgo dependencies.

## Architecture

```
cmd/calcassist        Cobra CLI: flag parsing, config load, REPL vs one-shot (-p)
internal/config       YAML load (~/.calcassist/config.yaml), env overrides, validation
internal/llm          provider-neutral types + Provider interface + adapters + factory
internal/agent        the multi-step agentic loop
internal/tools        Tool interface, Registry, and the 9 built-in tools
internal/tui          Bubble Tea inline scrolling REPL
internal/version      build version stamped via -ldflags
```

The agentic loop:

```
user prompt ─▶ agent.Run ─▶ llm.Provider.Chat (Responses/Messages API)
                  ▲                                   │
                  │                          tool calls requested?
          tool result(s)                             │ yes
                  │                                   ▼
                  └──── tools.Tool.Execute ◀── (confirm if Mutating)
```

`agent` advertises every tool's JSON schema to the model, executes requested tools
(asking the `Confirm` hook for mutating ones), feeds results back, and loops until the
model returns a final answer or `max_tool_iterations` is reached.

### Package dependency rules (important)

To keep packages independently testable and avoid import cycles, the parallel "leaf"
packages **do not import each other**:

- `config`, `llm`, and `tools` are **standalone** (no cross-imports between them, and
  they do not import `agent`/`tui`).
- `llm` defines its own neutral `llm.Config`; `main` maps `config.Config` → `llm.Config`.
  Do **not** make `config` import `llm`.
- `agent` imports `llm` + `tools` only.
- `tui` imports `agent` + `tools`.
- `cmd/calcassist` wires everything together.

## Tools

All nine live in `internal/tools` and implement the `Tool` interface
(`Name/Description/Schema/Mutating/Execute`). They are registered in
[`internal/tools/defaults.go`](internal/tools/defaults.go).

| Tool | Mutating | Args (JSON) |
|------|:--------:|-------------|
| `create_file` | yes | `path`, `content`, `overwrite?` |
| `list_directory` | no | `path?` |
| `read_file` | no | `path`, `max_bytes?` |
| `search_files` | no | `root?`, `name_glob?`, `content_regex?`, `max_results?` |
| `calculate` | no | `expression` |
| `statistics` | no | `values?` or `source?{type,path,sheet,column}`, `operations?` |
| `excel_to_json` | no | `path`, `sheet?`, `header_row?` |
| `read_pdf` | no | `path`, `max_chars?` |
| `read_docx` | no | `path` |

Only **mutating** tools trigger the confirmation prompt.

### How to add a tool

1. Create `internal/tools/<name>.go` with a `New<Name>Tool() Tool` constructor.
   Implement `Schema()` as a JSON-Schema object; unmarshal args into a typed struct in
   `Execute`; return `Mutating() == true` only if it writes to disk.
2. Register it in `internal/tools/defaults.go`.
3. Add a hermetic test (`t.TempDir()`, generated fixtures — no network).
4. Run `go test ./internal/tools`.

### How to add an LLM provider

1. Implement the `llm.Provider` interface in `internal/llm/<provider>.go`
   (`Name()` and `Chat(ctx, messages, tools, onText)`; stream when `onText != nil`).
2. Wire it into `New(cfg)` in [`internal/llm/factory.go`](internal/llm/factory.go).
3. Add `httptest`-based tests covering request mapping, auth, non-streaming and
   streaming parsing, and tool-call extraction.

OpenAI and Azure share one adapter using the **Responses API** at `<base_url>/responses`
(no `api-version`; Azure uses the `api-key` header and `/openai/v1` base). Anthropic uses
the Messages API at `<base_url>/v1/messages`.

## Configuration & secrets

- Config file: `~/.calcassist/config.yaml` (see [`config.example.yaml`](config.example.yaml)
  and [`docs/configuration.md`](docs/configuration.md)).
- Never commit secrets. `config.yaml` is git-ignored. Prefer environment variables:
  `CALCASSIST_API_KEY` (or `OPENAI_API_KEY` / `AZURE_OPENAI_API_KEY` / `ANTHROPIC_API_KEY`).
- Env overrides: `CALCASSIST_PROVIDER|MODEL|API_KEY|BASE_URL|MAX_TOKENS|TEMPERATURE|WEB_SEARCH`.
- Web search: set `web_search: true` (or `CALCASSIST_WEB_SEARCH=true`, off by default) to
  advertise the provider-hosted web search tool (OpenAI/Azure `web_search`, Anthropic
  `web_search_20250305`); cited sources are appended to the answer. See
  [`docs/configuration.md`](docs/configuration.md).

## Conventions

- **Formatting:** `gofmt` clean; **LF** line endings enforced via `.gitattributes`.
- **Comments:** only where they add clarity (see existing files).
- **Errors:** wrap with `%w` and context; in tools, return user-actionable messages.
  Tool execution errors are surfaced back to the model as results so it can recover —
  only context cancellation / confirmation failures abort the loop.
- **Tests:** table-driven and hermetic — `httptest` for `llm`, temp dirs + generated
  xlsx/docx/pdf fixtures for `tools`, a mock `llm.Provider` for `agent`.
- **No new heavy deps** without good reason; keep the binary cgo-free.

## CI

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs on PRs to `main`
(and pushes to `main`): build + vet + all unit tests on an Ubuntu/Windows matrix, plus a
`gofmt` check. CI uses `go-version-file: go.mod` and `CGO_ENABLED=0`.

## Gotchas

- Keep `config`/`llm`/`tools` free of cross-imports (see dependency rules above).
- `go test -race` needs CGO; this project is CGO-free, so CI runs plain `go test`.
- The TUI is **inline** Bubble Tea (no alt-screen): finalized content is emitted with
  `tea.Println` and scrolls above the live input/spinner. Agent work runs in a goroutine
  that pushes events through a channel consumed by `Update`; the confirmation prompt
  blocks that goroutine on a reply channel.
- PDF is text-extraction only (no OCR); Word is `.docx` only (no legacy `.doc`).
