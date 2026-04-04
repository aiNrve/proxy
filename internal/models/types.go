package models

import "time"

// Message represents a single chat message in OpenAI-compatible format.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the canonical inbound request format for completion calls.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	XAiNrveTask string    `json:"-"`
}

// ChatResponse is the canonical outbound response format.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice is a single completion candidate.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage captures token accounting information.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Provider describes provider configuration and runtime health metadata.
type Provider struct {
	Name      string    `json:"name"`
	BaseURL   string    `json:"base_url"`
	APIKey    string    `json:"-"`
	Enabled   bool      `json:"enabled"`
	HealthyAt time.Time `json:"healthy_at"`
}

// RouteDecision records the winning provider and scoring rationale.
type RouteDecision struct {
	Provider     string  `json:"provider"`
	Reason       string  `json:"reason"`
	ScoreCost    float64 `json:"score_cost"`
	ScoreLatency float64 `json:"score_latency"`
}

// RequestLog tracks request lifecycle metrics and outcomes.
type RequestLog struct {
	ID               string    `json:"id" db:"id"`
	RequestID        string    `json:"request_id" db:"request_id"`
	Provider         string    `json:"provider" db:"provider"`
	Model            string    `json:"model" db:"model"`
	TaskType         string    `json:"task_type" db:"task_type"`
	PromptTokens     int       `json:"prompt_tokens" db:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens" db:"completion_tokens"`
	CostUSD          float64   `json:"cost_usd" db:"cost_usd"`
	LatencyMs        int       `json:"latency_ms" db:"latency_ms"`
	Error            string    `json:"error,omitempty" db:"error"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}
