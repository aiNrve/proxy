# aiNrve/proxy

[![CI](https://github.com/aiNrve/proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/aiNrve/proxy/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/go-1.22-blue.svg)](https://golang.org)

The nervous system for your AI stack: route every LLM inference call to the fastest, cheapest available provider automatically.

## Why aiNrve exists

Teams shipping AI products are forced to hand-roll provider routing logic.
That means:

- Costs spike because traffic is pinned to one provider.
- Latency varies wildly by region/model/time of day.
- Failover is brittle and usually manual.

aiNrve/proxy solves this by acting like nginx for AI inference: one endpoint in, smart provider routing out.

## 30-second quickstart

```bash
cp .env.example .env
# Fill at least one provider API key in .env

docker compose up --build
```

Send an OpenAI-compatible request:

```bash
curl -sS http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer local-dev-key" \
  -H "X-AiNrve-Task: code" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Write a quicksort in Go"}],
    "stream": false
  }'
```

## Configuration example (ainrve.yaml)

```yaml
port: 8080
log_level: info
database_url: ""
ainrve_api_key: local-dev-key
health_check_interval: 30

providers:
  openai:
    enabled: true
    api_key: ""
    base_url: https://api.openai.com
    weight: 1.0
  anthropic:
    enabled: true
    api_key: ""
    base_url: https://api.anthropic.com
    weight: 1.0
  groq:
    enabled: true
    api_key: ""
    base_url: https://api.groq.com
    weight: 1.0
  together:
    enabled: true
    api_key: ""
    base_url: https://api.together.ai
    weight: 1.0
  gemini:
    enabled: true
    api_key: ""
    base_url: https://generativelanguage.googleapis.com
    weight: 1.0

routing:
  default_strategy: balanced
  weight_cost: 0.5
  weight_latency: 0.5
  task_overrides:
    code: groq
    reasoning: anthropic
  fallback_provider: openai
```

## Provider support

| Provider | Status | Streaming |
|---|---|---|
| OpenAI | Implemented | Yes |
| Anthropic | Implemented | Yes |
| Groq | Implemented | Yes |
| Together AI | Implemented | Yes |
| Gemini | Implemented | Yes |

## How routing works

- aiNrve scores healthy providers by cost and latency based on your strategy.
- Task hints (for example, `X-AiNrve-Task: code`) can override provider choice.
- If a selected provider returns 5xx, aiNrve retries once with the next best provider.

## API

- `POST /v1/chat/completions` OpenAI-compatible chat completions.
- `GET /health` process and provider health snapshot.
- `GET /metrics` Prometheus-compatible metrics.

## Python SDK

or use our SDK — `pip install ainrve`

## Contributing

Contributions are welcome.

1. Fork and clone this repository.
2. Create a feature branch.
3. Add tests for your changes.
4. Run `go test ./...`.
5. Open a pull request with clear context.

## License

MIT. See [LICENSE](LICENSE).
