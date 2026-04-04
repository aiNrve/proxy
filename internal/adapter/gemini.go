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
	"net/url"
	"strings"
	"time"

	"github.com/aiNrve/proxy/internal/models"
)

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata geminiUsage       `json:"usageMetadata"`
}

// GeminiAdapter implements Gemini request/response translation.
type GeminiAdapter struct {
	apiKey             string
	baseURL            string
	httpClient         *http.Client
	promptCostPer1KUSD float64
	outputCostPer1KUSD float64
}

// NewGeminiAdapter builds a Gemini adapter with sensible defaults.
func NewGeminiAdapter(apiKey, baseURL string) *GeminiAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	return &GeminiAdapter{
		apiKey:             apiKey,
		baseURL:            strings.TrimRight(baseURL, "/"),
		httpClient:         &http.Client{Timeout: defaultHTTPTimeout},
		promptCostPer1KUSD: 0.00125,
		outputCostPer1KUSD: 0.00500,
	}
}

func (a *GeminiAdapter) Name() string {
	return "gemini"
}

func (a *GeminiAdapter) Complete(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "gemini-1.5-flash"
	}

	payloadReq := geminiRequest{
		Contents: toGeminiContents(req.Messages),
		GenerationConfig: geminiGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		},
	}

	payload, err := json.Marshal(payloadReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", a.baseURL, url.PathEscape(model), url.QueryEscape(a.apiKey))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send gemini request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return nil, fmt.Errorf("provider gemini returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var providerResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&providerResp); err != nil {
		return nil, fmt.Errorf("decode gemini response: %w", err)
	}

	return toOpenAIResponseFromGemini(model, providerResp), nil
}

func (a *GeminiAdapter) CompleteStream(ctx context.Context, req *models.ChatRequest) (<-chan string, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "gemini-1.5-flash"
	}

	payloadReq := geminiRequest{
		Contents: toGeminiContents(req.Messages),
		GenerationConfig: geminiGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		},
	}

	payload, err := json.Marshal(payloadReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini stream request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", a.baseURL, url.PathEscape(model), url.QueryEscape(a.apiKey))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create gemini stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send gemini stream request: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return nil, fmt.Errorf("provider gemini stream returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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

func (a *GeminiAdapter) EstimateCost(req *models.ChatRequest) float64 {
	if req == nil {
		return 0
	}
	promptTokens := estimatePromptTokens(req)
	completionTokens := req.MaxTokens
	if completionTokens <= 0 {
		completionTokens = 512
	}
	cost := (float64(promptTokens)/1000.0)*a.promptCostPer1KUSD + (float64(completionTokens)/1000.0)*a.outputCostPer1KUSD
	return math.Max(cost, 0)
}

func (a *GeminiAdapter) IsHealthy(ctx context.Context) bool {
	endpoint := fmt.Sprintf("%s/v1beta/models?key=%s", a.baseURL, url.QueryEscape(a.apiKey))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func toGeminiContents(messages []models.Message) []geminiContent {
	out := make([]geminiContent, 0, len(messages))
	for _, message := range messages {
		role := "user"
		if message.Role == "assistant" {
			role = "model"
		}
		out = append(out, geminiContent{
			Role: role,
			Parts: []geminiPart{{
				Text: message.Content,
			}},
		})
	}
	return out
}

func toOpenAIResponseFromGemini(model string, resp geminiResponse) *models.ChatResponse {
	choice := models.Choice{
		Index: 0,
		Message: models.Message{
			Role:    "assistant",
			Content: "",
		},
		FinishReason: "stop",
	}

	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		texts := make([]string, 0, len(candidate.Content.Parts))
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				texts = append(texts, part.Text)
			}
		}
		choice.Message.Content = strings.Join(texts, "\n")
		choice.FinishReason = mapGeminiFinishReason(candidate.FinishReason)
	}

	promptTokens := resp.UsageMetadata.PromptTokenCount
	completionTokens := resp.UsageMetadata.CandidatesTokenCount
	totalTokens := resp.UsageMetadata.TotalTokenCount
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}

	return &models.ChatResponse{
		ID:      fmt.Sprintf("gemini-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []models.Choice{choice},
		Usage: models.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
		},
	}
}

func mapGeminiFinishReason(reason string) string {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "STOP", "FINISH_REASON_UNSPECIFIED":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	default:
		if strings.TrimSpace(reason) == "" {
			return "stop"
		}
		return strings.ToLower(reason)
	}
}
