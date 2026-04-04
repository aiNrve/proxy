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

const (
	defaultHTTPTimeout = 30 * time.Second
)

type openAICompatAdapter struct {
	providerName       string
	apiKey             string
	baseURL            string
	httpClient         *http.Client
	promptCostPer1KUSD float64
	outputCostPer1KUSD float64
}

// OpenAIAdapter implements the OpenAI provider.
type OpenAIAdapter struct {
	*openAICompatAdapter
}

// NewOpenAIAdapter builds an OpenAI adapter with sensible defaults.
func NewOpenAIAdapter(apiKey, baseURL string) *OpenAIAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.openai.com"
	}
	core := newOpenAICompatAdapter("openai", apiKey, baseURL, 0.0100, 0.0300)
	return &OpenAIAdapter{openAICompatAdapter: core}
}

func newOpenAICompatAdapter(providerName, apiKey, baseURL string, promptCostPer1KUSD, outputCostPer1KUSD float64) *openAICompatAdapter {
	return &openAICompatAdapter{
		providerName:       providerName,
		apiKey:             apiKey,
		baseURL:            strings.TrimRight(baseURL, "/"),
		httpClient:         &http.Client{Timeout: defaultHTTPTimeout},
		promptCostPer1KUSD: promptCostPer1KUSD,
		outputCostPer1KUSD: outputCostPer1KUSD,
	}
}

func (a *openAICompatAdapter) Name() string {
	return a.providerName
}

func (a *openAICompatAdapter) Complete(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return nil, &ProviderError{
			Provider:   a.providerName,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(body)),
		}
	}

	var out models.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

func (a *openAICompatAdapter) CompleteStream(ctx context.Context, req *models.ChatRequest) (<-chan string, error) {
	streamReq := *req
	streamReq.Stream = true

	payload, err := json.Marshal(&streamReq)
	if err != nil {
		return nil, fmt.Errorf("marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create stream request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send stream request: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return nil, &ProviderError{
			Provider:   a.providerName,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(body)),
		}
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

func (a *openAICompatAdapter) EstimateCost(req *models.ChatRequest) float64 {
	if req == nil {
		return 0
	}
	promptTokens := estimatePromptTokens(req)
	completionTokens := req.MaxTokens
	if completionTokens <= 0 {
		completionTokens = 256
	}

	cost := (float64(promptTokens)/1000.0)*a.promptCostPer1KUSD + (float64(completionTokens)/1000.0)*a.outputCostPer1KUSD
	return math.Max(cost, 0)
}

func (a *openAICompatAdapter) IsHealthy(ctx context.Context) bool {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func estimatePromptTokens(req *models.ChatRequest) int {
	if req == nil {
		return 0
	}
	totalChars := 0
	for _, msg := range req.Messages {
		totalChars += len(msg.Content)
	}
	if totalChars == 0 {
		return 0
	}
	return (totalChars / 4) + (len(req.Messages) * 4)
}
