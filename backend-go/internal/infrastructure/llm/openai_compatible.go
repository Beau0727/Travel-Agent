package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"travel-agent-go/internal/config"
	corellm "travel-agent-go/internal/llm"
	"travel-agent-go/internal/logging"
)

type OpenAICompatibleClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewOpenAICompatibleClient(cfg config.Config) *OpenAICompatibleClient {
	baseURL := strings.TrimRight(cfg.LLMBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	timeout := cfg.LLMTimeoutSeconds
	if timeout <= 0 {
		timeout = 60
	}
	return &OpenAICompatibleClient{
		apiKey:  strings.TrimSpace(cfg.LLMAPIKey),
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (c *OpenAICompatibleClient) Chat(ctx context.Context, request corellm.ChatRequest) (corellm.ChatMessage, error) {
	start := time.Now()
	if c.apiKey == "" {
		return corellm.ChatMessage{}, errors.New("LLM_API_KEY is empty")
	}

	body := chatRequestDTO{
		Model:       request.Model,
		Messages:    request.Messages,
		Tools:       request.Tools,
		ToolChoice:  request.ToolChoice,
		Temperature: request.Temperature,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return corellm.ChatMessage{}, err
	}

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return corellm.ChatMessage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	logging.Info(ctx, "llm chat request started",
		"model", request.Model,
		"messages", len(request.Messages),
		"tools", len(request.Tools),
		"url", endpoint,
	)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logging.Warn(ctx, "llm chat request failed",
			"model", request.Model,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return corellm.ChatMessage{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return corellm.ChatMessage{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.Warn(ctx, "llm chat response failed",
			"model", request.Model,
			"status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", trimBody(respBody),
		)
		return corellm.ChatMessage{}, fmt.Errorf("llm chat failed: %s", trimBody(respBody))
	}

	var parsed chatResponseDTO
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return corellm.ChatMessage{}, err
	}
	if len(parsed.Choices) == 0 {
		return corellm.ChatMessage{}, errors.New("LLM response has no choices")
	}

	logging.Info(ctx, "llm chat request completed",
		"model", request.Model,
		"status", resp.StatusCode,
		"choices", len(parsed.Choices),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return parsed.Choices[0].Message, nil
}

type chatRequestDTO struct {
	Model       string                   `json:"model"`
	Messages    []corellm.ChatMessage    `json:"messages"`
	Tools       []corellm.ToolDefinition `json:"tools,omitempty"`
	ToolChoice  string                   `json:"tool_choice,omitempty"`
	Temperature float64                  `json:"temperature"`
}

type chatResponseDTO struct {
	Choices []struct {
		Message corellm.ChatMessage `json:"message"`
	} `json:"choices"`
}

func trimBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 800 {
		return text[:800] + "..."
	}
	if text == "" {
		return "empty response body"
	}
	return text
}
