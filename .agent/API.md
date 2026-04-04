# API Reference

## Endpoints

### POST /v1/chat/completions
Drop-in replacement for OpenAI's chat completions endpoint.
Request body: OpenAI ChatCompletion format
Response: OpenAI ChatCompletion format (or SSE stream)

Headers required:
  Authorization: Bearer <ainrve-api-key>
  Content-Type: application/json

Extra optional headers:
  X-AiNrve-Task: classify|summarize|code|rag|reasoning
    (hints the router about task type for better routing)
  X-AiNrve-Provider: openai|anthropic|groq|together|gemini
    (force a specific provider, bypass routing)

### GET /health
Returns 200 if the proxy is running.
Body: {"status":"ok","providers":{"openai":true,"groq":true,...}}

### GET /metrics
Prometheus-compatible metrics endpoint.
Exposes: request_count, routing_latency_ms, provider_error_rate
