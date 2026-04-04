# Environment Variables

## Required (at least one provider key)
OPENAI_API_KEY        — OpenAI API key
ANTHROPIC_API_KEY     — Anthropic API key
GROQ_API_KEY          — Groq API key
TOGETHER_API_KEY      — Together AI API key
GEMINI_API_KEY        — Google Gemini API key

## Optional
DATABASE_URL          — Postgres connection string for logging
                        (if not set, logs to stdout only)
PORT                  — HTTP port (default: 8080)
CONFIG_PATH           — path to ainrve.yaml (default: ./ainrve.yaml)
AINRVE_API_KEY        — master key clients must send (if not set, 
                        no auth — fine for local/dev use)
LOG_LEVEL             — debug|info|warn|error (default: info)
HEALTH_CHECK_INTERVAL — seconds between provider pings (default: 30)
RATE_LIMIT_RPS        — requests per second per API key/IP (default: 10)
RATE_LIMIT_BURST      — token bucket burst size (default: 20)
