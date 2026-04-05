package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	extadapters "github.com/aiNrve/adapters"
	"github.com/aiNrve/proxy/internal/models"
)

// ToExternalRequest converts the gateway request model to the external adapters format.
func ToExternalRequest(req *models.ChatRequest) *extadapters.Request {
	if req == nil {
		return &extadapters.Request{}
	}

	messages := make([]extadapters.Message, 0, len(req.Messages))
	for _, msg := range req.Messages {
		messages = append(messages, extadapters.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return &extadapters.Request{
		Model:       req.Model,
		Messages:    messages,
		Stream:      req.Stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TaskType:    req.XAiNrveTask,
	}
}

// FromExternalResponse converts external adapter responses to gateway response model.
func FromExternalResponse(resp *extadapters.Response) *models.ChatResponse {
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

// ConvertExternalError normalizes external provider errors to ProviderError.
func ConvertExternalError(err error) error {
	var adapterErr *extadapters.AdapterError
	if errors.As(err, &adapterErr) {
		return &ProviderError{
			Provider:   adapterErr.Provider,
			StatusCode: adapterErr.StatusCode,
			Body:       adapterErr.Message,
		}
	}
	return err
}

// StreamDeltaLine converts streamed text into OpenAI-compatible SSE payload lines.
func StreamDeltaLine(delta string) (string, error) {
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

// StreamErrorLine formats a streamed error as an SSE data line.
func StreamErrorLine(err error) string {
	payload := map[string]any{
		"error": map[string]string{"message": err.Error()},
	}
	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return "data: {\"error\":{\"message\":\"stream failed\"}}"
	}
	return "data: " + string(body)
}
