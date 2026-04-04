# Architecture Decisions

## ADR-001: OpenAI format as canonical internal format
All adapters translate TO OpenAI format on the way out.
Reason: most developers already know this format. Lowest friction.

## ADR-002: Async logging
Request logging must NOT block the response path.
Use a buffered channel + goroutine to write to Postgres.
If the channel is full, drop the log (availability > completeness).

## ADR-003: ainrve.yaml for routing config
Routing rules are in a YAML file, not a database.
Reason: version-controllable, no external dependency to start.
Hot-reload via fsnotify — no restart needed when config changes.

## ADR-004: No global state
All state passed via context or explicit struct fields.
No package-level variables except for the config singleton.

## ADR-005: Provider errors are retried once
On 5xx from a provider, score them to 0 and re-route.
On 429 (rate limit), mark provider as cooling for 60s.

## ADR-006: Retry uses exclusion context
Retry selection is driven by context-level provider exclusions.
Reason: keeps Route() deterministic and lets gateway retry once without
introducing hidden side effects in the scoring path.
