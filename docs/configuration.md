# Configuration

CalcAssist is configured with a YAML file at `~/.calcassist/config.yaml`
(on Windows: `C:\Users\<you>\.calcassist\config.yaml`). You can point at a different
file with `--config <path>`.

A starter file lives at [`config.example.yaml`](../config.example.yaml) in the repo root.

## Fields

| Field | Required | Description |
|-------|:--------:|-------------|
| `provider` | yes | `openai`, `azure`, or `anthropic`. |
| `model` | yes | OpenAI model id, Azure **deployment name**, or Anthropic model id. |
| `api_key` | yes | The API key. May be left blank if supplied via an environment variable (see below). |
| `base_url` | azure only | Endpoint base. Required for Azure; optional for OpenAI/Anthropic. |
| `max_tokens` | no | Max output tokens (default `4096`). |
| `temperature` | no | Sampling temperature (default `0.2`). |
| `max_tool_iterations` | no | Cap on tool-call rounds per request (default `12`). |
| `web_search` | no | Enable the provider-hosted web search tool (default `false`). |

## Environment variables

Non-empty environment variables override the file:

- `CALCASSIST_PROVIDER`
- `CALCASSIST_MODEL`
- `CALCASSIST_API_KEY`
- `CALCASSIST_BASE_URL`
- `CALCASSIST_MAX_TOKENS`
- `CALCASSIST_TEMPERATURE`
- `CALCASSIST_WEB_SEARCH`

If `api_key` is still empty, CalcAssist falls back to a provider-specific variable:

- OpenAI → `OPENAI_API_KEY`
- Azure → `AZURE_OPENAI_API_KEY`
- Anthropic → `ANTHROPIC_API_KEY`

Storing the key in an environment variable rather than the file is recommended.

---

## OpenAI

```yaml
provider: openai
model: gpt-4o
api_key: ""              # or set OPENAI_API_KEY
# base_url: ""           # defaults to https://api.openai.com/v1
```

CalcAssist calls the **Responses API** at `<base_url>/responses`
(`https://api.openai.com/v1/responses` by default) with `Authorization: Bearer <key>`.

You can also point `base_url` at any OpenAI-compatible gateway that implements the
Responses API.

---

## Azure OpenAI

Azure uses the next-generation **v1 API**, so the endpoint path is the same as OpenAI
and **no `api-version` query parameter is needed**.

```yaml
provider: azure
model: my-deployment-name                          # the *deployment* name, not the base model
api_key: ""                                         # or set AZURE_OPENAI_API_KEY
base_url: https://my-resource.openai.azure.com/openai/v1
```

Requirements:

1. An Azure OpenAI resource with a deployed model that supports the Responses API.
2. `base_url` must be your resource endpoint **with the `/openai/v1` suffix**:
   `https://<resource-name>.openai.azure.com/openai/v1`.
3. `model` is the **deployment name** you chose in Azure.

CalcAssist sends the key in the `api-key` header and POSTs to `<base_url>/responses`.

---

## Anthropic

```yaml
provider: anthropic
model: claude-sonnet-4-5
api_key: ""              # or set ANTHROPIC_API_KEY
# base_url: ""           # defaults to https://api.anthropic.com
```

CalcAssist calls the Messages API at `<base_url>/v1/messages` with the
`x-api-key` and `anthropic-version` headers, and maps tools to Anthropic's
`tool_use` / `tool_result` blocks.

---

## Web search

Set `web_search: true` (or `CALCASSIST_WEB_SEARCH=true`) to let the model use the
provider's **hosted** web search tool. It is **off by default**.

- **OpenAI / Azure** advertise the Responses API `web_search` tool.
- **Anthropic** advertises the Messages API `web_search_20250305` tool.

When the model searches, CalcAssist appends a **Sources** list of the cited URLs to the
answer.

> The hosted tool must be supported by your configured model/deployment, otherwise the
> provider may reject the request. In particular, not all **Azure** deployments support
> `web_search`, and Anthropic requires web search to be enabled for your organization in
> the Claude Console. Leave `web_search` off if your model does not support it.

---

## Verifying your setup

Run a one-shot prompt that forces a tool call:

```bash
calcassist -p "what is sqrt(144) + 7?"
```

You should see the `calculate` tool run on stderr and the answer on stdout. If you get
an auth error, double-check the key, the `provider`, and (for Azure) that `base_url`
ends in `/openai/v1` and `model` is the deployment name.
