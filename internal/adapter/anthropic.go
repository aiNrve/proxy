package adapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/aiNrve/proxy/internal/models"
)

type anthropicContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicMessage struct {
	Role    string                 `json:"role"`
	Content []anthropicContentPart `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicResponse struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Model      string                 `json:"model"`
	StopReason string                 `json:"stop_reason"`
	Content    []anthropicContentPart `json:"content"`
	Usage      anthropicUsage         `json:"usage"`
}

// AnthropicAdapter implements Anthropic's messages API and translation logic.
type AnthropicAdapter struct {
	apiKey             string
	baseURL            string
	httpClient         *http.Client
	promptCostPer1KUSD float64
	outputCostPer1KUSD float64
}

// NewAnthropicAdapter builds an Anthropic adapter with sensible defaults.
func NewAnthropicAdapter(apiKey, baseURL string) *AnthropicAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &AnthropicAdapter{
		apiKey:             apiKey,
		baseURL:            strings.TrimRight(baseURL, "/"),
		httpClient:         &http.Client{Timeout: defaultHTTPTimeout},
		promptCostPer1KUSD: 0.0080,
		outputCostPer1KUSD: 0.0240,
	}
}

func (a *AnthropicAdapter) Name() string {
	return "anthropic"
}

func (a *AnthropicAdapter) Complete(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	payloadReq := anthropicRequest{
		Model:       req.Model,
		Messages:    toAnthropicMessages(req.Messages),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	if payloadReq.MaxTokens <= 0 {
		payloadReq.MaxTokens = 1024
	}

	payload, err := json.Marshal(payloadReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create anthropic request: %w", err)
	}
	a.setHeaders(httpReq)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send anthropic request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return nil, fmt.Errorf("provider anthropic returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var providerResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&providerResp); err != nil {
		return nil, fmt.Errorf("decode anthropic response: %w", err)
	}

	return toOpenAIResponseFromAnthropic(providerResp), nil
}

func (a *AnthropicAdapter) CompleteStream(ctx context.Context, req *models.ChatRequest) (<-chan string, error) {
	payloadReq := anthropicRequest{
		Model:       req.Model,
		Messages:    toAnthropicMessages(req.Messages),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}
	if payloadReq.MaxTokens <= 0 {
		payloadReq.MaxTokens = 1024
	}

	payload, err := json.Marshal(payloadReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create anthropic stream request: %w", err)
	}
	a.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send anthropic stream request: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return nil, fmt.Errorf("provider anthropic stream returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	out := make(chan string, 32)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case out <- line:
			}
		}
	}()

	return out, nil
}

func (a *AnthropicAdapter) EstimateCost(req *models.ChatRequest) float64 {
	if req == nil {
		return 0
	}
	promptTokens := estimatePromptTokens(req)
	completionTokens := req.MaxTokens
	if completionTokens <= 0 {
		completionTokens = 1024
	}
	cost := (float64(promptTokens)/1000.0)*a.promptCostPer1KUSD + (float64(completionTokens)/1000.0)*a.outputCostPer1KUSD
	return math.Max(cost, 0)
}

func (a *AnthropicAdapter) IsHealthy(ctx context.Context) bool {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	a.setHeaders(httpReq)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (a *AnthropicAdapter) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
}

func toAnthropicMessages(messages []models.Message) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(messages))
	for _, msg := range messages {
		role := msg.Role
		switch role {
		case "assistant", "user":
		default:
			role = "user"
		}
		out = append(out, anthropicMessage{
			Role: role,
			Content: []anthropicContentPart{{
				Type: "text",
				Text: msg.Content,
			}},
		})
	}
	return out
}

func toOpenAIResponseFromAnthropic(resp anthropicResponse) *models.ChatResponse {
	texts := make([]string, 0, len(resp.Content))
	for _, part := range resp.Content {
		if strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}

	finishReason := mapAnthropicFinishReason(resp.StopReason)
	promptTokens := resp.Usage.InputTokens
	completionTokens := resp.Usage.OutputTokens

	return &models.ChatResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []models.Choice{{
			Index: 0,
			Message: models.Message{
				Role:    "assistant",
				Content: strings.Join(texts, "\n"),
			},
			FinishReason: finishReason,
		}},
		Usage: models.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}
}

func mapAnthropicFinishReason(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	default:
		if reason == "" {
			return "stop"
		}
		return reason
	}
}
