# aiNrve/proxy — Architecture

## What this is
HTTP proxy in Go. Receives LLM requests in OpenAI-compatible format.
Routes to best provider. Returns response. Logs every call.

## Request lifecycle
1. Client sends POST /v1/chat/completions (OpenAI format)
2. Gateway: auth check → rate limit → request parse
3. Router: score all providers → pick winner
4. Adapter: translate to provider's native format
5. Provider: make HTTP call, stream or buffer response
6. Logger: write cost/latency/tokens to Postgres async
7. Client receives response in OpenAI format regardless of provider

## Package structure
/cmd/proxy          → main.go, binary entrypoint
/internal/gateway   → HTTP server, middleware, auth
/internal/router    → routing brain, scoring engine
/internal/adapter   → provider adapters (one file per provider)
/internal/logger    → async request logger → Postgres
/internal/config    → env var + ainrve.yaml parsing
/internal/models    → shared types (Request, Response, Provider, Route)
/pkg/ainrve         → public Go SDK surface (used by sdk-go later)

## Provider adapters to build (in order)
1. openai.go    — baseline, everything maps 1:1
2. anthropic.go — messages API, different stop reasons
3. groq.go      — OpenAI-compatible, just different base URL + key
4. together.go  — OpenAI-compatible with model name differences
5. gemini.go    — completely different request/response schema

## Routing scoring (v1 — rules-based)
Score = (cost_weight × cost_score) + (latency_weight × latency_score)
Weights come from ainrve.yaml. Provider with highest score wins.
Fallback: if top provider returns 5xx, retry with next highest score.

## Key design decisions
- All responses normalized to OpenAI format (providers adapt TO us)
- Streaming supported via SSE passthrough
- Config hot-reload: watch ainrve.yaml, no restart needed
- Provider health tracked in-memory, checked every 30s
