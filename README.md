# CalcAssist

A cross-platform (Windows + Linux + macOS) terminal AI assistant written in Go. It
presents a GitHub-Copilot–style scrolling conversational REPL and uses an LLM with
**native tool calling** to pick and run local tools in a multi-step agentic loop —
creating files, navigating the filesystem, doing math & statistics, converting Excel
to JSON, and reading PDF and Word documents.

The whole app ships as a single, dependency-free binary (`calcassist.exe` / `calcassist`).

---

## Features

- **Conversational REPL** (built on the Charm Bubble Tea stack) with streamed,
  Markdown-rendered answers.
- **Multi-step agentic loop**: the model calls a tool, sees the result, and decides the
  next step until your request is satisfied.
- **Native tool/function calling** across three providers:
  - **OpenAI** and **Azure OpenAI** via the **Responses API** (`/v1/responses`)
  - **Anthropic** via the Messages API
- **Confirmation before writes**: any tool that modifies the filesystem asks first
  (with a preview). Use `--yes` to auto-approve in non-interactive runs.
- **One-shot mode** for scripting: `calcassist -p "…"` prints the answer to stdout.
- **Pure Go, `CGO_ENABLED=0`** — trivial cross-compilation, no system libraries.

## Built-in tools

| Tool | Writes? | What it does |
|------|:------:|--------------|
| `create_file` | ✅ | Create a file with given content (creates parent dirs). |
| `list_directory` | — | List a directory (dirs first, then files). |
| `read_file` | — | Read a text file up to a byte limit. |
| `search_files` | — | Find files by name glob and/or content regex. |
| `calculate` | — | Evaluate a math expression (sqrt, pow, trig, log, …). |
| `statistics` | — | count/sum/mean/median/min/max/variance/stddev over values or an Excel/JSON column. |
| `excel_to_json` | — | Convert an Excel worksheet to a JSON array of objects. |
| `read_pdf` | — | Extract plain text from a PDF (no OCR). |
| `read_docx` | — | Extract text from a `.docx` Word document. |

> Word support is `.docx` only (not legacy `.doc`). PDF support is text extraction only
> (scanned/encrypted PDFs and OCR are not supported).

---

## Install / build

Requires Go 1.24+ (developed against Go 1.26).

**Windows (PowerShell):**
```powershell
./build.ps1            # -> bin\calcassist.exe
./build.ps1 -All       # cross-compile everything -> dist\
```

**Linux / macOS:**
```bash
./build.sh             # -> bin/calcassist
./build.sh all         # cross-compile everything -> dist/
# or:
make build
make all
```

**Plain Go:**
```bash
CGO_ENABLED=0 go build -o bin/calcassist ./cmd/calcassist
```

---

## Configure

CalcAssist reads `~/.calcassist/config.yaml`. Copy [`config.example.yaml`](config.example.yaml)
there and fill it in. See [docs/configuration.md](docs/configuration.md) for full,
per-provider instructions.

Minimal OpenAI example (`~/.calcassist/config.yaml`):
```yaml
provider: openai
model: gpt-4o
api_key: "sk-…"          # or leave blank and set OPENAI_API_KEY
```

Azure OpenAI example:
```yaml
provider: azure
model: my-deployment-name                       # the Azure *deployment* name
api_key: "…"                                     # or set AZURE_OPENAI_API_KEY
base_url: https://my-resource.openai.azure.com/openai/v1
```
> Azure uses the `/openai/v1/responses` path, so **no `api-version` is required**.

Anthropic example:
```yaml
provider: anthropic
model: claude-sonnet-4-5
api_key: "…"             # or set ANTHROPIC_API_KEY
```

Any field can be overridden by environment variables: `CALCASSIST_PROVIDER`,
`CALCASSIST_MODEL`, `CALCASSIST_API_KEY`, `CALCASSIST_BASE_URL`,
`CALCASSIST_MAX_TOKENS`, `CALCASSIST_TEMPERATURE`.

---

## Usage

Interactive REPL:
```text
$ calcassist
you ❯ convert sales.xlsx to JSON and save it as sales.json
⚙ excel_to_json {"path":"sales.xlsx"}
  ✓ excel_to_json
Allow tool "create_file" to run?
  path: sales.json
  content: 1843 bytes, 42 line(s)
[y]es / [N]o ❯ y
  ✓ create_file
Saved 42 records to **sales.json**.
```

Slash commands inside the REPL: `/help`, `/tools`, `/config`, `/clear`, `/exit`.

One-shot (scriptable):
```bash
calcassist -p "what is the standard deviation of 4, 8, 15, 16, 23, 42?"
calcassist -p "convert data.xlsx to JSON and save data.json" --yes
```

Flags:
```text
-c, --config string   path to config file (default ~/.calcassist/config.yaml)
-p, --prompt string   run a single prompt non-interactively and exit
    --yes             auto-approve tool actions that modify files
-v, --version         print version
```

---

## How it works

```
your prompt ─▶ Agent ─▶ Provider (Responses/Messages API) ─▶ tool call?
                 ▲                                              │
                 └────────── tool result ◀── run tool ◀────────┘
                              (confirm if it writes)
```

The agent advertises every tool's JSON schema to the model. When the model requests a
tool, the agent runs it (asking for confirmation on mutating tools), feeds the result
back, and loops until the model returns a final answer or `max_tool_iterations` is hit.

## Project layout

```
cmd/calcassist      entry point (Cobra CLI: REPL + one-shot)
internal/config     YAML config load, env overrides, validation
internal/llm        provider-neutral types + OpenAI/Azure (Responses API) + Anthropic
internal/agent      the multi-step agentic loop
internal/tools      Tool interface, registry, and all built-in tools
internal/tui        Bubble Tea scrolling REPL
internal/version    build version stamp
```

## Development

```bash
make test     # go test ./...
make vet      # go vet ./...
make fmt      # gofmt -w .
```

See **[AGENTS.md](AGENTS.md)** for the full engineering guide (architecture, package
boundaries, how to add tools/providers, conventions). AI agents: see
[CLAUDE.md](CLAUDE.md) and [.github/copilot-instructions.md](.github/copilot-instructions.md).

Continuous integration ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs
`go build`, `go vet`, the full unit-test suite (Ubuntu + Windows), and a `gofmt` check
on every pull request to `main`.

## License

Released under the [MIT License](LICENSE).
