package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	external "github.com/aiNrve/adapters"
	"github.com/aiNrve/proxy/internal/models"
)

// WrapExternal adapts an external adapter to the internal adapter contract.
func WrapExternal(item external.Adapter) Adapter {
	if item == nil {
		return nil
	}
	return &externalAdapter{item: item}
}

type externalAdapter struct {
	item external.Adapter
}

func (a *externalAdapter) Name() string {
	if a == nil || a.item == nil {
		return ""
	}
	return a.item.Name()
}

func (a *externalAdapter) Complete(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	resp, err := a.item.Complete(ctx, toExternalRequest(req))
	if err != nil {
		return nil, convertExternalError(err)
	}
	return fromExternalResponse(resp), nil
}

func (a *externalAdapter) CompleteStream(ctx context.Context, req *models.ChatRequest) (<-chan string, error) {
	stream, err := a.item.CompleteStream(ctx, toExternalRequest(req))
	if err != nil {
		return nil, convertExternalError(err)
	}

	out := make(chan string)
	go func() {
		defer close(out)

		for chunk := range stream {
			if chunk.Error != nil {
				if !emit(ctx, out, streamErrorLine(chunk.Error)) {
					return
				}
				_ = emit(ctx, out, "data: [DONE]")
				return
			}

			if chunk.Delta != "" {
				line, marshalErr := streamDeltaLine(chunk.Delta)
				if marshalErr != nil {
					if !emit(ctx, out, streamErrorLine(marshalErr)) {
						return
					}
					_ = emit(ctx, out, "data: [DONE]")
					return
				}
				if !emit(ctx, out, line) {
					return
				}
			}

			if chunk.Done {
				_ = emit(ctx, out, "data: [DONE]")
				return
			}
		}
	}()

	return out, nil
}

func (a *externalAdapter) EstimateCost(req *models.ChatRequest) float64 {
	return a.item.EstimateCost(toExternalRequest(req))
}

func (a *externalAdapter) IsHealthy(ctx context.Context) bool {
	return a.item.IsHealthy(ctx)
}

func toExternalRequest(req *models.ChatRequest) *external.Request {
	if req == nil {
		return &external.Request{}
	}

	messages := make([]external.Message, 0, len(req.Messages))
	for _, msg := range req.Messages {
		messages = append(messages, external.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return &external.Request{
		Model:       req.Model,
		Messages:    messages,
		Stream:      req.Stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TaskType:    req.XAiNrveTask,
	}
}

func fromExternalResponse(resp *external.Response) *models.ChatResponse {
	if resp == nil {
		return &models.ChatResponse{}
	}

	choices := make([]models.Choice, 0, len(resp.Choices))
	for _, choice := range resp.Choices {
		choices = append(choices, models.Choice{
			Index: choice.Index,
			Message: models.Message{
				Role:    choice.Message.Role,
				Content: choice.Message.Content,
			},
			FinishReason: choice.FinishReason,
		})
	}

	return &models.ChatResponse{
		ID:      resp.ID,
		Object:  resp.Object,
		Created: resp.Created,
		Model:   resp.Model,
		Choices: choices,
		Usage: models.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
}

func convertExternalError(err error) error {
	var adapterErr *external.AdapterError
	if errors.As(err, &adapterErr) {
		return &ProviderError{
			Provider:   adapterErr.Provider,
			StatusCode: adapterErr.StatusCode,
			Body:       adapterErr.Message,
		}
	}
	return err
}

func streamDeltaLine(delta string) (string, error) {
	payload := map[string]any{
		"id":      "chatcmpl-ainrve",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]string{"content": delta},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal stream delta: %w", err)
	}
	return "data: " + string(body), nil
}

func streamErrorLine(err error) string {
	payload := map[string]any{
		"error": map[string]string{"message": err.Error()},
	}
	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return "data: {\"error\":{\"message\":\"stream failed\"}}"
	}
	return "data: " + string(body)
}

func emit(ctx context.Context, out chan<- string, line string) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- line:
		return true
	}
}
